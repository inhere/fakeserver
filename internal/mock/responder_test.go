package mock

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gookit/rux/v2"

	"github.com/inhere/fakeserver/internal/config"
	"github.com/inhere/fakeserver/internal/recorder"
	"github.com/inhere/fakeserver/internal/tpl"
)

// newRespondServer wires a rux router that calls mock.Respond for the
// configured route and returns a running httptest.Server. Using a real
// dispatch path is the cleanest way to exercise rux v2's inline
// [16]Param container — c.Params() is populated by the router from the
// matched URL, which is the same code path production traffic uses.
//
// rux v2 path syntax: "{name}" for path params (verified against
// internal/echo/probe.md and server.MountEchoRoutes' "/status/{code}"
// route). The handler receives a *rux.Context with c.Req == real
// *http.Request and c.Resp wrapping the underlying httptest writer.
func newRespondServer(t *testing.T, method, registerPath string, route *config.Route, renderer tpl.Renderer) *httptest.Server {
	t.Helper()
	r := rux.New()
	r.Add(registerPath, func(c *rux.Context) {
		Respond(c, route, 0, renderer, nil)
	}, method)
	return httptest.NewServer(r)
}

func TestRespond_SetsRecorderTrace(t *testing.T) {
	rdr := tpl.NewRenderer(nil, nil, 0)
	route := &config.Route{
		Method:     []string{"GET"},
		Path:       "/trace",
		SourceFile: "routes/trace.json5",
		Body:       "ok",
	}
	var trace *recorder.RequestTrace
	r := rux.New()
	r.GET("/trace", func(c *rux.Context) {
		ctx, tr := recorder.WithRequestTrace(c.Req.Context())
		trace = tr
		c.Req = c.Req.WithContext(ctx)
		Respond(c, route, 4, rdr, nil)
	})

	req := httptest.NewRequest("GET", "/trace", nil)
	resp := httptest.NewRecorder()
	r.ServeHTTP(resp, req)

	if trace == nil {
		t.Fatal("trace was not attached")
	}
	if trace.RouteIndex == nil || *trace.RouteIndex != 4 {
		t.Fatalf("RouteIndex=%v, want 4", trace.RouteIndex)
	}
	if trace.RouteMode != "mock" {
		t.Fatalf("RouteMode=%q, want mock", trace.RouteMode)
	}
	if trace.RouteSource != "routes/trace.json5" {
		t.Fatalf("RouteSource=%q", trace.RouteSource)
	}
}

func TestRespond_PlainStringBodyInfersTextPlain(t *testing.T) {
	r := tpl.NewRenderer(nil, nil, 0)
	route := &config.Route{Method: []string{"GET"}, Path: "/ping", Body: "pong"}
	ts := newRespondServer(t, "GET", "/ping", route, r)
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/ping")
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("status: got %d", resp.StatusCode)
	}
	if !strings.HasPrefix(resp.Header.Get("Content-Type"), "text/plain") {
		t.Errorf("Content-Type: got %q", resp.Header.Get("Content-Type"))
	}
	body, _ := io.ReadAll(resp.Body)
	if string(body) != "pong" {
		t.Errorf("body: got %q", string(body))
	}
}

func TestRespond_TemplateErrorIncludesSourceFieldHint(t *testing.T) {
	r := tpl.NewRenderer(nil, nil, 0)
	route := &config.Route{
		Method:     []string{"GET"},
		Path:       "/bad",
		SourceFile: "routes/health.json5",
		Body:       `{{ .request.headers.User-Agent }}`,
	}
	ts := newRespondServer(t, "GET", "/bad", route, r)
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/bad")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	var got map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&got); err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"error", "detail", "route", "source", "field", "hint"} {
		if got[key] == nil || got[key] == "" {
			t.Fatalf("error response missing %s: %+v", key, got)
		}
	}
	if got["source"] != "routes/health.json5" {
		t.Fatalf("source=%v", got["source"])
	}
	if got["field"] != "body" {
		t.Fatalf("field=%v", got["field"])
	}
	if !strings.Contains(got["hint"].(string), `index .request.headers "User-Agent"`) {
		t.Fatalf("hint=%v", got["hint"])
	}
}

func TestRespond_HeaderTemplateErrorIncludesField(t *testing.T) {
	r := tpl.NewRenderer(nil, nil, 0)
	route := &config.Route{
		Method:     []string{"GET"},
		Path:       "/bad-header",
		SourceFile: "routes/headers.json5",
		Headers:    map[string]string{"X-Trace-Id": "{{ .unclosed"},
		Body:       "ok",
	}
	ts := newRespondServer(t, "GET", "/bad-header", route, r)
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/bad-header")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	var got map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&got); err != nil {
		t.Fatal(err)
	}
	if got["field"] != "headers.X-Trace-Id" {
		t.Fatalf("field=%v, response=%+v", got["field"], got)
	}
	if got["source"] != "routes/headers.json5" {
		t.Fatalf("source=%v", got["source"])
	}
}

func TestRespond_BodyFileErrorIncludesSourceField(t *testing.T) {
	r := tpl.NewRenderer(nil, nil, 0)
	route := &config.Route{
		Method:     []string{"GET"},
		Path:       "/missing",
		SourceFile: "routes/files.json5",
		BodyFile:   "missing.json",
	}
	ts := newRespondServer(t, "GET", "/missing", route, r)
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/missing")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	var got map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&got); err != nil {
		t.Fatal(err)
	}
	if got["field"] != "bodyFile" {
		t.Fatalf("field=%v, response=%+v", got["field"], got)
	}
	if got["source"] != "routes/files.json5" {
		t.Fatalf("source=%v", got["source"])
	}
}

func TestRespond_MapBodyInfersJSON(t *testing.T) {
	r := tpl.NewRenderer(nil, nil, 0)
	route := &config.Route{Method: []string{"GET"}, Path: "/x", Body: map[string]any{"k": "v"}}
	ts := newRespondServer(t, "GET", "/x", route, r)
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/x")
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()

	if !strings.HasPrefix(resp.Header.Get("Content-Type"), "application/json") {
		t.Errorf("Content-Type: got %q", resp.Header.Get("Content-Type"))
	}
	body, _ := io.ReadAll(resp.Body)
	var got map[string]any
	if err := json.Unmarshal(body, &got); err != nil {
		t.Fatalf("body not JSON: %v\nbody: %s", err, body)
	}
	if got["k"] != "v" {
		t.Errorf("body: got %v", got)
	}
}

func TestRespond_TemplateRendersParams(t *testing.T) {
	r := tpl.NewRenderer(nil, nil, 0)
	route := &config.Route{
		Method: []string{"GET"},
		Path:   "/u/{id}",
		Body: map[string]any{
			"id":   "{{ .request.params.id }}",
			"echo": "{{ .request.method }}",
		},
	}
	ts := newRespondServer(t, "GET", "/u/{id}", route, r)
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/u/42")
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	var got map[string]any
	if err := json.Unmarshal(body, &got); err != nil {
		t.Fatalf("body not JSON: %v\nbody: %s", err, body)
	}
	if got["id"] != "42" {
		t.Errorf("id: got %v", got["id"])
	}
	if got["echo"] != "GET" {
		t.Errorf("echo: got %v", got["echo"])
	}
}

func TestRespond_BodyFileServesBytesAndInfersType(t *testing.T) {
	r := tpl.NewRenderer(nil, nil, 0)
	// BodyFile path is relative; SourceFile empty means it's resolved
	// against the test's working directory (the package dir), which is
	// exactly where testdata/fixtures/avatar.png lives.
	route := &config.Route{Method: []string{"GET"}, Path: "/avatar", BodyFile: "testdata/fixtures/avatar.png"}
	ts := newRespondServer(t, "GET", "/avatar", route, r)
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/avatar")
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("status: got %d", resp.StatusCode)
	}
	if !strings.HasPrefix(resp.Header.Get("Content-Type"), "image/png") {
		t.Errorf("Content-Type: got %q", resp.Header.Get("Content-Type"))
	}
	body, _ := io.ReadAll(resp.Body)
	if len(body) == 0 {
		t.Error("expected non-empty body for bodyFile")
	}
}

func TestRespond_ExplicitContentTypePreserved(t *testing.T) {
	r := tpl.NewRenderer(nil, nil, 0)
	route := &config.Route{
		Method:  []string{"GET"},
		Path:    "/x",
		Headers: map[string]string{"Content-Type": "application/xml"},
		Body:    "<x/>",
	}
	ts := newRespondServer(t, "GET", "/x", route, r)
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/x")
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.Header.Get("Content-Type") != "application/xml" {
		t.Errorf("Content-Type: got %q", resp.Header.Get("Content-Type"))
	}
	body, _ := io.ReadAll(resp.Body)
	if string(body) != "<x/>" {
		t.Errorf("body: got %q", string(body))
	}
}

func TestRespond_DelayApplied(t *testing.T) {
	r := tpl.NewRenderer(nil, nil, 0)
	route := &config.Route{Method: []string{"GET"}, Path: "/x", Body: "x", Delay: "50ms"}
	ts := newRespondServer(t, "GET", "/x", route, r)
	defer ts.Close()

	start := time.Now()
	resp, err := http.Get(ts.URL + "/x")
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	resp.Body.Close()
	elapsed := time.Since(start)
	if elapsed < 40*time.Millisecond || elapsed > 500*time.Millisecond {
		t.Errorf("delay: %v (want ~50ms)", elapsed)
	}
}

// TestRespond_BodyTemplateError: body 含坏模板 → 500 + JSON error 体
func TestRespond_BodyTemplateError(t *testing.T) {
	r := tpl.NewRenderer(nil, nil, 0)
	route := &config.Route{
		Method: []string{"GET"},
		Path:   "/err",
		Body:   `{{ fail "intentional" }}`,
	}
	ts := newRespondServer(t, "GET", "/err", route, r)
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/err")
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusInternalServerError {
		t.Errorf("status: got %d, want 500", resp.StatusCode)
	}
	if !strings.HasPrefix(resp.Header.Get("Content-Type"), "application/json") {
		t.Errorf("Content-Type: got %q", resp.Header.Get("Content-Type"))
	}
	body, _ := io.ReadAll(resp.Body)
	var got map[string]any
	if err := json.Unmarshal(body, &got); err != nil {
		t.Fatalf("error body not JSON: %v\nbody: %s", err, body)
	}
	if got["error"] == nil {
		t.Errorf("error body missing 'error' field; got %v", got)
	}
}

// TestRespond_HeaderTemplateError: header 含坏模板 → 500
func TestRespond_HeaderTemplateError(t *testing.T) {
	r := tpl.NewRenderer(nil, nil, 0)
	route := &config.Route{
		Method:  []string{"GET"},
		Path:    "/herr",
		Headers: map[string]string{"X-Bad": `{{ fail "header-fail" }}`},
		Body:    "ok",
	}
	ts := newRespondServer(t, "GET", "/herr", route, r)
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/herr")
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusInternalServerError {
		t.Errorf("status: got %d, want 500", resp.StatusCode)
	}
}

// TestRespond_BodyFileMissing: bodyFile 指向不存在路径 → 500
func TestRespond_BodyFileMissing(t *testing.T) {
	r := tpl.NewRenderer(nil, nil, 0)
	route := &config.Route{
		Method:   []string{"GET"},
		Path:     "/missing",
		BodyFile: "testdata/fixtures/does-not-exist.bin",
	}
	ts := newRespondServer(t, "GET", "/missing", route, r)
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/missing")
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusInternalServerError {
		t.Errorf("status: got %d, want 500", resp.StatusCode)
	}
	body, _ := io.ReadAll(resp.Body)
	var got map[string]any
	if err := json.Unmarshal(body, &got); err != nil {
		t.Fatalf("error body not JSON: %v\nbody: %s", err, body)
	}
	if got["error"] == nil {
		t.Errorf("error body missing 'error' field; got %v", got)
	}
}

// TestRespond_DelayRange: delay "10ms~50ms" 区间 sleep
func TestRespond_DelayRange(t *testing.T) {
	r := tpl.NewRenderer(nil, nil, 0)
	route := &config.Route{Method: []string{"GET"}, Path: "/d", Body: "x", Delay: "10ms~50ms"}
	ts := newRespondServer(t, "GET", "/d", route, r)
	defer ts.Close()

	start := time.Now()
	resp, _ := http.Get(ts.URL + "/d")
	resp.Body.Close()
	elapsed := time.Since(start)
	// 区间是 10~50ms，给出 5ms 余量 / 上限 500ms 余量
	if elapsed < 5*time.Millisecond || elapsed > 500*time.Millisecond {
		t.Errorf("delay range: %v (want 10~50ms)", elapsed)
	}
}

// TestRespond_DelayBadDuration: 坏 duration 字符串应被忽略（返回 0），不影响响应。
func TestRespond_DelayBadDuration(t *testing.T) {
	r := tpl.NewRenderer(nil, nil, 0)
	route := &config.Route{Method: []string{"GET"}, Path: "/bd", Body: "x", Delay: "not-a-duration"}
	ts := newRespondServer(t, "GET", "/bd", route, r)
	defer ts.Close()

	start := time.Now()
	resp, _ := http.Get(ts.URL + "/bd")
	resp.Body.Close()
	if time.Since(start) > 500*time.Millisecond {
		t.Errorf("bad delay should not sleep; took %v", time.Since(start))
	}
}

// TestRespond_DelayRangeInvalid: 区间 lo>hi 或解析失败 → 不 sleep
func TestRespond_DelayRangeInvalid(t *testing.T) {
	r := tpl.NewRenderer(nil, nil, 0)
	route := &config.Route{Method: []string{"GET"}, Path: "/bd2", Body: "x", Delay: "50ms~10ms"}
	ts := newRespondServer(t, "GET", "/bd2", route, r)
	defer ts.Close()

	start := time.Now()
	resp, _ := http.Get(ts.URL + "/bd2")
	resp.Body.Close()
	if time.Since(start) > 500*time.Millisecond {
		t.Errorf("invalid range should not sleep; took %v", time.Since(start))
	}
}

// TestRespond_BodyAsSlice: body 是 []any 时应走 JSON marshal，并保持顺序
func TestRespond_BodyAsSlice(t *testing.T) {
	r := tpl.NewRenderer(nil, nil, 0)
	route := &config.Route{
		Method: []string{"GET"},
		Path:   "/list",
		Body:   []any{"a", "b", "c"},
	}
	ts := newRespondServer(t, "GET", "/list", route, r)
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/list")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if !strings.HasPrefix(resp.Header.Get("Content-Type"), "application/json") {
		t.Errorf("Content-Type: got %q", resp.Header.Get("Content-Type"))
	}
	body, _ := io.ReadAll(resp.Body)
	var got []any
	if err := json.Unmarshal(body, &got); err != nil {
		t.Fatalf("body not JSON array: %v\nbody: %s", err, body)
	}
	if len(got) != 3 || got[0] != "a" {
		t.Errorf("slice body: got %v", got)
	}
}

// TestRespond_NestedSliceTemplateRender: body 是嵌套 slice，模板字符串应被渲染
func TestRespond_NestedSliceTemplateRender(t *testing.T) {
	r := tpl.NewRenderer(nil, nil, 0)
	route := &config.Route{
		Method: []string{"GET"},
		Path:   "/u/{id}",
		Body: map[string]any{
			"items": []any{
				map[string]any{"id": "{{ .request.params.id }}"},
			},
		},
	}
	ts := newRespondServer(t, "GET", "/u/{id}", route, r)
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/u/99")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	var got map[string]any
	if err := json.Unmarshal(body, &got); err != nil {
		t.Fatalf("body not JSON: %v\nbody: %s", err, body)
	}
	items := got["items"].([]any)
	if items[0].(map[string]any)["id"] != "99" {
		t.Errorf("nested slice render: got %v", got)
	}
}

// TestRespond_NilBodyEmptyResponse: 无 body 时仍能写头 + 状态
func TestRespond_NilBodyEmptyResponse(t *testing.T) {
	r := tpl.NewRenderer(nil, nil, 0)
	route := &config.Route{Method: []string{"GET"}, Path: "/empty", Status: 204}
	ts := newRespondServer(t, "GET", "/empty", route, r)
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/empty")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 204 {
		t.Errorf("status: got %d, want 204", resp.StatusCode)
	}
	body, _ := io.ReadAll(resp.Body)
	if len(body) != 0 {
		t.Errorf("body should be empty; got %q", body)
	}
}

// TestRespond_BodyFileAbsolutePath: 绝对路径 bodyFile 应直接被使用（不再 join SourceFile dir）
func TestRespond_BodyFileAbsolutePath(t *testing.T) {
	r := tpl.NewRenderer(nil, nil, 0)
	// 用 testdata/fixtures/avatar.png 的绝对路径
	abs, err := filepath.Abs("testdata/fixtures/avatar.png")
	if err != nil {
		t.Fatalf("abs: %v", err)
	}
	route := &config.Route{
		Method:   []string{"GET"},
		Path:     "/abs",
		BodyFile: abs,
		// SourceFile 故意设错以验证不被 join 进来
		SourceFile: "/nonexistent/dir/cfg.json5",
	}
	ts := newRespondServer(t, "GET", "/abs", route, r)
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/abs")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Errorf("status: %d", resp.StatusCode)
	}
	body, _ := io.ReadAll(resp.Body)
	if len(body) == 0 {
		t.Error("abs bodyFile empty body")
	}
}

// TestRespond_StatusZeroDefaultsTo200: route.Status == 0 应回落到 200
func TestRespond_StatusZeroDefaultsTo200(t *testing.T) {
	r := tpl.NewRenderer(nil, nil, 0)
	route := &config.Route{Method: []string{"GET"}, Path: "/d200", Body: "ok"}
	ts := newRespondServer(t, "GET", "/d200", route, r)
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/d200")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Errorf("status zero defaults to 200; got %d", resp.StatusCode)
	}
}

// TestRespond_EnvAccessibleInTemplate: cfg.Env 注入后模板可访问 .env.* 键
func TestRespond_EnvAccessibleInTemplate(t *testing.T) {
	route := &config.Route{
		Method: []string{"GET"}, Path: "/",
		Body: "{{ .env.token }}",
	}
	rdr := tpl.NewRenderer(nil, nil, 1)
	r := rux.New()
	envMap := map[string]any{"token": "T-FROM-ENV"}
	r.GET("/", func(c *rux.Context) {
		Respond(c, route, 0, rdr, envMap)
	})
	srv := httptest.NewServer(r)
	defer srv.Close()
	resp, err := http.Get(srv.URL + "/")
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	b, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if string(b) != "T-FROM-ENV" {
		t.Errorf("body=%q want T-FROM-ENV", string(b))
	}
}

// TestRespond_BodyFileUnknownExtMime: 未识别扩展名应回落到 application/octet-stream
func TestRespond_BodyFileUnknownExtMime(t *testing.T) {
	// 在 testdata/fixtures 下放一个临时 .xyz 文件
	dir := "testdata/fixtures"
	tmp := filepath.Join(dir, "tmp-unknown.xyz")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(tmp, []byte("payload"), 0o644); err != nil {
		t.Fatalf("write tmp: %v", err)
	}
	defer os.Remove(tmp)

	r := tpl.NewRenderer(nil, nil, 0)
	route := &config.Route{Method: []string{"GET"}, Path: "/u", BodyFile: tmp}
	ts := newRespondServer(t, "GET", "/u", route, r)
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/u")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	ct := resp.Header.Get("Content-Type")
	if !strings.HasPrefix(ct, "application/octet-stream") {
		t.Errorf("unknown ext CT: got %q, want application/octet-stream", ct)
	}
}
