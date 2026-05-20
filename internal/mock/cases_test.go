package mock

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gookit/rux/v2"

	"github.com/inhere/fakeserver/internal/config"
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
		RespondCases(c, route, matchers, sel, renderer)
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
