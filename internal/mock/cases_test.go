package mock

import (
	"encoding/json"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/gookit/rux/v2"

	"github.com/inhere/fakeserver/internal/config"
	"github.com/inhere/fakeserver/internal/recorder"
	"github.com/inhere/fakeserver/internal/scenario"
	"github.com/inhere/fakeserver/internal/tpl"
)

// helper：用 cases handler 启动一个 httptest server，返回 baseURL。
// 模式与 router_test.go 现有用例一致：经过真实 rux 路由匹配。
func startCasesServer(t *testing.T, route *config.Route, renderer tpl.Renderer) *httptest.Server {
	t.Helper()
	// 预编译 matchers + selector，模拟 Task 5 router.Mount 的产物
	matchers := make([]*Matcher, len(route.Cases))
	for i, c := range route.Cases {
		m, err := CompileMatcher(c.When)
		if err != nil {
			t.Fatalf("compile when[%d]: %v", i, err)
		}
		matchers[i] = m
	}
	sel := NewSelector(route.Strategy)
	r := rux.New()
	h := func(c *rux.Context) {
		RespondCases(c, route, 0, matchers, sel, renderer, nil)
	}
	for _, m := range route.Method {
		if m == "*" {
			r.Any(route.Path, h)
		} else {
			r.Add(route.Path, h, strings.ToUpper(m))
		}
	}
	return httptest.NewServer(r)
}

func mustMatcher(t *testing.T, expr string) *Matcher {
	t.Helper()
	m, err := CompileMatcher(expr)
	if err != nil {
		t.Fatalf("CompileMatcher(%q): %v", expr, err)
	}
	return m
}

func TestRespondCases_BodyTemplateSeesRequestBody(t *testing.T) {
	route := &config.Route{Method: []string{"POST"}, Path: "/case", Cases: []config.RouteCase{{When: `request.body.id == 7`, Body: map[string]any{"echo": `{{ .request.body.id }}`}}}}
	ts := startCasesServer(t, route, tpl.NewRenderer(nil, nil, 0))
	defer ts.Close()
	resp, err := http.Post(ts.URL+"/case", "application/json", strings.NewReader(`{"id":7}`))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	if !strings.Contains(string(b), `"echo":"7"`) {
		t.Fatalf("body=%s", b)
	}
}

func TestRespondCases_UsesScenarioCase(t *testing.T) {
	route := &config.Route{
		Method:   []string{"GET"},
		Path:     "/api/users",
		Strategy: "first-match",
		Cases: []config.RouteCase{
			{Name: "success", Status: 200, Body: map[string]any{"state": "success"}},
			{Name: "empty", Status: 200, Body: map[string]any{"state": "empty"}},
		},
	}
	cfg := &config.Config{
		Server: config.ServerOpts{Scenario: "emptyUsers"},
		Scenarios: map[string]config.ScenarioConfig{
			"emptyUsers": {Routes: map[string]string{"GET /api/users": "empty"}},
		},
	}
	rec := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/api/users", nil)
	ctx, trace := recorder.WithRequestTrace(req.Context())
	req = req.WithContext(ctx)
	c := &rux.Context{Req: req, Resp: rec}

	RespondCasesWithScenario(c, cfg, route, 0, []*Matcher{mustMatcher(t, ""), mustMatcher(t, "")}, NewSelector("first-match"), tpl.NewRenderer(nil, nil, 0), nil, nil, "")

	if !strings.Contains(rec.Body.String(), `"state":"empty"`) {
		t.Fatalf("scenario case response = %s", rec.Body.String())
	}
	if trace.CaseName != "empty" || trace.Scenario != "emptyUsers" || trace.OverrideSource != "config" {
		t.Fatalf("trace = %#v", trace)
	}
}

func TestRespondCases_HeaderScenarioBeatsConfigScenario(t *testing.T) {
	route := &config.Route{
		Method:   []string{"GET"},
		Path:     "/api/users",
		Strategy: "first-match",
		Cases: []config.RouteCase{
			{Name: "success", Status: 200, Body: map[string]any{"state": "success"}},
			{Name: "empty", Status: 200, Body: map[string]any{"state": "empty"}},
			{Name: "error", Status: 500, Body: map[string]any{"state": "error"}},
		},
	}
	cfg := &config.Config{
		Server: config.ServerOpts{Scenario: "emptyUsers"},
		Scenarios: map[string]config.ScenarioConfig{
			"emptyUsers": {Routes: map[string]string{"GET /api/users": "empty"}},
			"errorUsers": {Routes: map[string]string{"GET /api/users": "error"}},
		},
	}
	rec := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/api/users", nil)
	req.Header.Set(scenario.HeaderName, "errorUsers")
	ctx, trace := recorder.WithRequestTrace(req.Context())
	req = req.WithContext(ctx)
	c := &rux.Context{Req: req, Resp: rec}

	RespondCasesWithScenario(c, cfg, route, 0, []*Matcher{mustMatcher(t, ""), mustMatcher(t, ""), mustMatcher(t, "")}, NewSelector("first-match"), tpl.NewRenderer(nil, nil, 0), nil, nil, "")

	if rec.Code != 500 || !strings.Contains(rec.Body.String(), `"state":"error"`) {
		t.Fatalf("header scenario response code=%d body=%s", rec.Code, rec.Body.String())
	}
	if trace.CaseName != "error" || trace.Scenario != "errorUsers" || trace.OverrideSource != "header" {
		t.Fatalf("trace = %#v", trace)
	}
}

func TestRespondCases_OverrideNextBeatsScenarioAndConsumes(t *testing.T) {
	route := &config.Route{
		Method:   []string{"GET"},
		Path:     "/api/users",
		Strategy: "first-match",
		Cases: []config.RouteCase{
			{Name: "success", Status: 200, Body: map[string]any{"state": "success"}},
			{Name: "empty", Status: 200, Body: map[string]any{"state": "empty"}},
			{Name: "error", Status: 500, Body: map[string]any{"state": "error"}},
		},
	}
	cfg := &config.Config{
		Server: config.ServerOpts{Scenario: "emptyUsers"},
		Scenarios: map[string]config.ScenarioConfig{
			"emptyUsers": {Routes: map[string]string{"GET /api/users": "empty"}},
		},
	}
	store := scenario.NewStore()
	store.SetOverride(scenario.NewRouteKey("GET", "/api/users"), scenario.Override{CaseName: "error", Mode: "next"})
	matchers := []*Matcher{mustMatcher(t, ""), mustMatcher(t, ""), mustMatcher(t, "")}
	renderer := tpl.NewRenderer(nil, nil, 0)

	firstRec := httptest.NewRecorder()
	firstReq := httptest.NewRequest("GET", "/api/users", nil)
	firstCtx, firstTrace := recorder.WithRequestTrace(firstReq.Context())
	firstReq = firstReq.WithContext(firstCtx)
	RespondCasesWithScenario(&rux.Context{Req: firstReq, Resp: firstRec}, cfg, route, 0, matchers, NewSelector("first-match"), renderer, nil, store, "")

	if firstRec.Code != 500 || !strings.Contains(firstRec.Body.String(), `"state":"error"`) {
		t.Fatalf("override response code=%d body=%s", firstRec.Code, firstRec.Body.String())
	}
	if firstTrace.CaseName != "error" || firstTrace.OverrideSource != "override:next" || firstTrace.Scenario != "emptyUsers" {
		t.Fatalf("first trace = %#v", firstTrace)
	}

	secondRec := httptest.NewRecorder()
	secondReq := httptest.NewRequest("GET", "/api/users", nil)
	secondCtx, secondTrace := recorder.WithRequestTrace(secondReq.Context())
	secondReq = secondReq.WithContext(secondCtx)
	RespondCasesWithScenario(&rux.Context{Req: secondReq, Resp: secondRec}, cfg, route, 0, matchers, NewSelector("first-match"), renderer, nil, store, "")

	if !strings.Contains(secondRec.Body.String(), `"state":"empty"`) {
		t.Fatalf("scenario response after consume = %s", secondRec.Body.String())
	}
	if secondTrace.CaseName != "empty" || secondTrace.OverrideSource != "config" || secondTrace.Scenario != "emptyUsers" {
		t.Fatalf("second trace = %#v", secondTrace)
	}
}

func TestRespondCases_MultiMethodUsesActualRequestMethodForScenario(t *testing.T) {
	route := &config.Route{
		Method:   []string{"GET", "POST"},
		Path:     "/api/users",
		Strategy: "first-match",
		Cases: []config.RouteCase{
			{Name: "success", Status: 200, Body: map[string]any{"state": "success"}},
			{Name: "created", Status: 201, Body: map[string]any{"state": "created"}},
		},
	}
	cfg := &config.Config{
		Server: config.ServerOpts{Scenario: "createUser"},
		Scenarios: map[string]config.ScenarioConfig{
			"createUser": {Routes: map[string]string{"POST /api/users": "created"}},
		},
	}
	rec := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/api/users", nil)
	ctx, trace := recorder.WithRequestTrace(req.Context())
	req = req.WithContext(ctx)

	RespondCasesWithScenario(&rux.Context{Req: req, Resp: rec}, cfg, route, 0, []*Matcher{mustMatcher(t, ""), mustMatcher(t, "")}, NewSelector("first-match"), tpl.NewRenderer(nil, nil, 0), nil, nil, "")

	if rec.Code != 201 || !strings.Contains(rec.Body.String(), `"state":"created"`) {
		t.Fatalf("POST scenario response code=%d body=%s", rec.Code, rec.Body.String())
	}
	if trace.CaseName != "created" || trace.OverrideSource != "config" {
		t.Fatalf("trace = %#v", trace)
	}
}

func TestRespondCases_MultiMethodUsesActualRequestMethodForOverride(t *testing.T) {
	route := &config.Route{
		Method:   []string{"GET", "POST"},
		Path:     "/api/users",
		Strategy: "first-match",
		Cases: []config.RouteCase{
			{Name: "success", Status: 200, Body: map[string]any{"state": "success"}},
			{Name: "created", Status: 201, Body: map[string]any{"state": "created"}},
		},
	}
	store := scenario.NewStore()
	store.SetOverride(scenario.NewRouteKey("POST", "/api/users"), scenario.Override{CaseName: "created", Mode: "always"})
	rec := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/api/users", nil)
	ctx, trace := recorder.WithRequestTrace(req.Context())
	req = req.WithContext(ctx)

	RespondCasesWithScenario(&rux.Context{Req: req, Resp: rec}, &config.Config{}, route, 0, []*Matcher{mustMatcher(t, ""), mustMatcher(t, "")}, NewSelector("first-match"), tpl.NewRenderer(nil, nil, 0), nil, store, "")

	if rec.Code != 201 || !strings.Contains(rec.Body.String(), `"state":"created"`) {
		t.Fatalf("POST override response code=%d body=%s", rec.Code, rec.Body.String())
	}
	if trace.CaseName != "created" || trace.OverrideSource != "override:always" {
		t.Fatalf("trace = %#v", trace)
	}
}

func TestRespondCases_SetsCaseTrace(t *testing.T) {
	route := &config.Route{
		Method:     []string{"GET"},
		Path:       "/cases",
		SourceFile: "routes/cases.json5",
		Strategy:   "first-match",
		Cases: []config.RouteCase{
			{When: `request.query.hit == "0"`, Status: 200, Body: "zero"},
			{When: `request.query.hit == "1"`, Status: 200, Body: "one"},
		},
	}
	matchers := make([]*Matcher, len(route.Cases))
	for i, cs := range route.Cases {
		m, err := CompileMatcher(cs.When)
		if err != nil {
			t.Fatal(err)
		}
		matchers[i] = m
	}
	selector := NewSelector(route.Strategy)
	rdr := tpl.NewRenderer(nil, nil, 0)
	var trace *recorder.RequestTrace
	r := rux.New()
	r.GET("/cases", func(c *rux.Context) {
		ctx, tr := recorder.WithRequestTrace(c.Req.Context())
		trace = tr
		c.Req = c.Req.WithContext(ctx)
		RespondCases(c, route, 6, matchers, selector, rdr, nil)
	})

	resp := httptest.NewRecorder()
	r.ServeHTTP(resp, httptest.NewRequest("GET", "/cases?hit=1", nil))

	if trace == nil {
		t.Fatal("trace was not attached")
	}
	if trace.RouteIndex == nil || *trace.RouteIndex != 6 {
		t.Fatalf("RouteIndex=%v, want 6", trace.RouteIndex)
	}
	if trace.CaseIndex == nil || *trace.CaseIndex != 1 {
		t.Fatalf("CaseIndex=%v, want 1", trace.CaseIndex)
	}
	if trace.RouteMode != "cases" {
		t.Fatalf("RouteMode=%q, want cases", trace.RouteMode)
	}
	if trace.RouteSource != "routes/cases.json5" {
		t.Fatalf("RouteSource=%q", trace.RouteSource)
	}
}

func TestRespondCases_FirstMatch_Hits(t *testing.T) {
	route := &config.Route{
		Method:   []string{"GET"},
		Path:     "/u/{id}",
		Strategy: "first-match",
		Cases: []config.RouteCase{
			{When: `request.query.fail == "1"`, Status: 500, Body: map[string]any{"error": "boom"}},
			{Status: 200, Body: map[string]any{"id": "{{ .request.params.id }}", "ok": true}},
		},
	}
	r := tpl.NewRenderer(nil, nil, 1)
	srv := startCasesServer(t, route, r)
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/u/42?fail=1")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 500 {
		t.Errorf("?fail=1 → status %d want 500", resp.StatusCode)
	}
	var got map[string]any
	_ = json.NewDecoder(resp.Body).Decode(&got)
	if got["error"] != "boom" {
		t.Errorf("body=%v", got)
	}
}

func TestRespondCases_FirstMatch_Fallback(t *testing.T) {
	route := &config.Route{
		Method:   []string{"GET"},
		Path:     "/u/{id}",
		Strategy: "first-match",
		Cases: []config.RouteCase{
			{When: `request.query.fail == "1"`, Status: 500, Body: map[string]any{"error": "boom"}},
			{Status: 200, Body: map[string]any{"id": "{{ .request.params.id }}"}},
		},
	}
	r := tpl.NewRenderer(nil, nil, 1)
	srv := startCasesServer(t, route, r)
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/u/42")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Errorf("default branch → status %d want 200", resp.StatusCode)
	}
	var got map[string]any
	_ = json.NewDecoder(resp.Body).Decode(&got)
	if got["id"] != "42" {
		t.Errorf("expected id=42 got %v", got)
	}
}

// TestRespondCases_AllFiltered_NoMatch 锁定 DoD #4 与 design §4.5 错误表：
// first-match 下所有 when 都为 false → 500 + {"error":"no case matched"}。
func TestRespondCases_AllFiltered_NoMatch(t *testing.T) {
	route := &config.Route{
		Method:   []string{"GET"},
		Path:     "/x",
		Strategy: "first-match",
		Cases: []config.RouteCase{
			{When: `request.query.a == "1"`, Status: 200, Body: "a"},
			{When: `request.query.b == "1"`, Status: 200, Body: "b"},
		},
	}
	r := tpl.NewRenderer(nil, nil, 1)
	srv := startCasesServer(t, route, r)
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/x")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 500 {
		t.Errorf("no-match → status %d want 500", resp.StatusCode)
	}
	var got map[string]any
	_ = json.NewDecoder(resp.Body).Decode(&got)
	if got["error"] != "no case matched" {
		t.Errorf("error msg=%v want 'no case matched'", got["error"])
	}
	if got["route"] == nil {
		t.Errorf("error body should include route field; got %v", got)
	}
}

func TestRespondCases_CaseInheritsOuterDefaults(t *testing.T) {
	route := &config.Route{
		Method:   []string{"POST"},
		Path:     "/p",
		Status:   201,
		Headers:  map[string]string{"X-Source": "outer"},
		Strategy: "first-match",
		Cases: []config.RouteCase{
			{Body: map[string]any{"ok": true}}, // 没写 status/headers → 应继承
		},
	}
	r := tpl.NewRenderer(nil, nil, 1)
	srv := startCasesServer(t, route, r)
	defer srv.Close()

	resp, err := http.Post(srv.URL+"/p", "application/json", strings.NewReader("{}"))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 201 {
		t.Errorf("inherit status: got %d want 201", resp.StatusCode)
	}
	if got := resp.Header.Get("X-Source"); got != "outer" {
		t.Errorf("inherit X-Source: got %q want 'outer'", got)
	}
}

func TestRespondCases_HeadersMergeCaseWinsOnConflict(t *testing.T) {
	// outer has 2 headers; case has 2 headers, one overlapping
	// expected after merge: 3 distinct keys; X-Both is case's value
	route := &config.Route{
		Method:   []string{"GET"},
		Path:     "/m",
		Headers:  map[string]string{"X-Outer-Only": "from-outer", "X-Both": "outer-value"},
		Strategy: "first-match",
		Cases: []config.RouteCase{
			{
				Status:  200,
				Headers: map[string]string{"X-Case-Only": "from-case", "X-Both": "case-value"},
				Body:    map[string]any{"ok": true},
			},
		},
	}
	r := tpl.NewRenderer(nil, nil, 1)
	srv := startCasesServer(t, route, r)
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/m")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if got := resp.Header.Get("X-Outer-Only"); got != "from-outer" {
		t.Errorf("X-Outer-Only=%q want from-outer", got)
	}
	if got := resp.Header.Get("X-Case-Only"); got != "from-case" {
		t.Errorf("X-Case-Only=%q want from-case", got)
	}
	if got := resp.Header.Get("X-Both"); got != "case-value" {
		t.Errorf("X-Both=%q want case-value (case must win)", got)
	}
}

// TestRespondCases_Random_HitsBothCases 单测 random 路径走通（分布在
// selector_test.go 验证）。
func TestRespondCases_Random_HitsBothCases(t *testing.T) {
	route := &config.Route{
		Method:   []string{"GET"},
		Path:     "/r",
		Strategy: "random",
		Cases: []config.RouteCase{
			{Status: 200, Body: "A"},
			{Status: 200, Body: "B"},
		},
	}
	r := tpl.NewRenderer(nil, nil, 1)
	srv := startCasesServer(t, route, r)
	defer srv.Close()

	seen := map[string]bool{}
	for i := 0; i < 50; i++ {
		resp, err := http.Get(srv.URL + "/r")
		if err != nil {
			t.Fatal(err)
		}
		b, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		seen[strings.TrimSpace(string(b))] = true
	}
	if !seen["A"] || !seen["B"] {
		t.Errorf("50 random picks should hit both cases; seen=%v", seen)
	}
}

// TestRespondCases_RuntimeWhenError_Skips 锁定 design §4.5 降级：when 运
// 行期出错 → 该 case 视为不匹配，后续 case 继续。
func TestRespondCases_RuntimeWhenError_Skips(t *testing.T) {
	route := &config.Route{
		Method:   []string{"GET"},
		Path:     "/e",
		Strategy: "first-match",
		Cases: []config.RouteCase{
			// when 期待 query.foo 是 string 但请求带的是 query.foo=1（其实也是
			// string），所以这里我们用 len() 触发类型错。
			{When: `len(request.query.foo) > 100`, Status: 500, Body: "should not hit"},
			{Status: 200, Body: "fallback"},
		},
	}
	r := tpl.NewRenderer(nil, nil, 1)
	// Silence the expected warn-log so test output stays clean
	log.SetOutput(io.Discard)
	defer log.SetOutput(os.Stderr)
	srv := startCasesServer(t, route, r)
	defer srv.Close()

	// 这条请求没带 foo → len(nil) 求值出错 → 跳过 → 命中 fallback
	resp, err := http.Get(srv.URL + "/e")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Errorf("when runtime err should skip case; got status %d", resp.StatusCode)
	}
}

// TestRespondCases_MissingFieldIsNoMatchNotError 锁定任务书语义：字段缺失
// （expr 求值为 nil）属于正常不匹配，不产生 when_error 标记。
func TestRespondCases_MissingFieldIsNoMatchNotError(t *testing.T) {
	route := &config.Route{
		Method:   []string{"GET"},
		Path:     "/e5",
		Strategy: "first-match",
		Cases: []config.RouteCase{
			{Name: "absent", When: `request.query.absent`, Status: 200, Body: "absent"},
		},
	}
	matchers := make([]*Matcher, len(route.Cases))
	for i, cs := range route.Cases {
		m, err := CompileMatcher(cs.When)
		if err != nil {
			t.Fatal(err)
		}
		matchers[i] = m
	}
	var trace *recorder.RequestTrace
	r := rux.New()
	r.GET("/e5", func(c *rux.Context) {
		ctx, tr := recorder.WithRequestTrace(c.Req.Context())
		trace = tr
		c.Req = c.Req.WithContext(ctx)
		RespondCases(c, route, 0, matchers, NewSelector(route.Strategy), tpl.NewRenderer(nil, nil, 0), nil)
	})

	resp := httptest.NewRecorder()
	r.ServeHTTP(resp, httptest.NewRequest("GET", "/e5", nil))
	if resp.Code != 500 {
		t.Fatalf("missing field case should not match → 500, got %d", resp.Code)
	}
	if trace == nil || trace.WhenError != "" {
		t.Fatalf("missing field must not be flagged as when error, got %+v", trace)
	}
	var body struct {
		Unmatched  []string `json:"unmatched"`
		WhenErrors []any    `json:"whenErrors"`
	}
	if err := json.Unmarshal(resp.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	if len(body.Unmatched) != 1 || body.Unmatched[0] != "absent" {
		t.Errorf("unmatched=%v want [absent]", body.Unmatched)
	}
	if len(body.WhenErrors) != 0 {
		t.Errorf("whenErrors=%v want empty for a missing field", body.WhenErrors)
	}
}

// TestRespondCases_NoMatch_ListsUnmatchedAndWhenErrors 锁定「when 错误不再静默」：
// 全部 case 不匹配时的 500 响应体里，正常不匹配只列 case 名，出错的那条带原因。
func TestRespondCases_NoMatch_ListsUnmatchedAndWhenErrors(t *testing.T) {
	route := &config.Route{
		Method:   []string{"GET"},
		Path:     "/e3",
		Strategy: "first-match",
		Cases: []config.RouteCase{
			{Name: "boom", When: `len(request.query.foo) > 100`, Status: 200, Body: "boom"},
			{Name: "plain", When: `request.query.hit == "yes"`, Status: 200, Body: "plain"},
		},
	}
	matchers := make([]*Matcher, len(route.Cases))
	for i, cs := range route.Cases {
		m, err := CompileMatcher(cs.When)
		if err != nil {
			t.Fatal(err)
		}
		matchers[i] = m
	}
	rdr := tpl.NewRenderer(nil, nil, 0)
	var trace *recorder.RequestTrace
	r := rux.New()
	r.GET("/e3", func(c *rux.Context) {
		ctx, tr := recorder.WithRequestTrace(c.Req.Context())
		trace = tr
		c.Req = c.Req.WithContext(ctx)
		RespondCases(c, route, 2, matchers, NewSelector(route.Strategy), rdr, nil)
	})

	resp := httptest.NewRecorder()
	r.ServeHTTP(resp, httptest.NewRequest("GET", "/e3", nil))
	if resp.Code != 500 {
		t.Fatalf("all cases filtered → status %d want 500", resp.Code)
	}
	var body struct {
		Error      string   `json:"error"`
		Route      string   `json:"route"`
		Unmatched  []string `json:"unmatched"`
		WhenErrors []struct {
			Case  string `json:"case"`
			Error string `json:"error"`
		} `json:"whenErrors"`
	}
	if err := json.Unmarshal(resp.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode 500 body: %v (%s)", err, resp.Body.String())
	}
	if body.Error != "no case matched" || body.Route != "GET /e3" {
		t.Fatalf("unexpected error body: %s", resp.Body.String())
	}
	if len(body.Unmatched) != 1 || body.Unmatched[0] != "plain" {
		t.Errorf("unmatched=%v want [plain]", body.Unmatched)
	}
	if len(body.WhenErrors) != 1 || body.WhenErrors[0].Case != "boom" {
		t.Fatalf("whenErrors=%+v want single boom entry", body.WhenErrors)
	}
	if !strings.Contains(body.WhenErrors[0].Error, "when") {
		t.Errorf("whenErrors reason should quote the when source: %q", body.WhenErrors[0].Error)
	}
	if trace == nil {
		t.Fatal("trace was not attached")
	}
	if !strings.HasPrefix(trace.WhenError, "boom:") {
		t.Fatalf("trace.WhenError=%q want boom:<err>", trace.WhenError)
	}
}

// TestRespondCases_WhenError_MarksTraceWhenAnotherCaseMatches 锁定兼容性：
// 某个 case 求值出错仍不改变「跳过并继续」语义，但 trace 上留下 when_error
// 供访问日志/历史记录标注。
func TestRespondCases_WhenError_MarksTraceWhenAnotherCaseMatches(t *testing.T) {
	route := &config.Route{
		Method:   []string{"GET"},
		Path:     "/e4",
		Strategy: "first-match",
		Cases: []config.RouteCase{
			{Name: "boom", When: `len(request.query.foo) > 100`, Status: 500, Body: "boom"},
			{Name: "fallback", Status: 200, Body: "fallback"},
		},
	}
	matchers := make([]*Matcher, len(route.Cases))
	for i, cs := range route.Cases {
		m, err := CompileMatcher(cs.When)
		if err != nil {
			t.Fatal(err)
		}
		matchers[i] = m
	}
	rdr := tpl.NewRenderer(nil, nil, 0)
	var trace *recorder.RequestTrace
	r := rux.New()
	r.GET("/e4", func(c *rux.Context) {
		ctx, tr := recorder.WithRequestTrace(c.Req.Context())
		trace = tr
		c.Req = c.Req.WithContext(ctx)
		RespondCases(c, route, 0, matchers, NewSelector(route.Strategy), rdr, nil)
	})

	resp := httptest.NewRecorder()
	r.ServeHTTP(resp, httptest.NewRequest("GET", "/e4", nil))
	if resp.Code != 200 {
		t.Fatalf("fallback case should still answer 200, got %d", resp.Code)
	}
	if trace == nil || !strings.HasPrefix(trace.WhenError, "boom:") {
		t.Fatalf("trace.WhenError=%v, want boom:<err>", trace)
	}
}
