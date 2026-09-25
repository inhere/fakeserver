package mock

import (
	"encoding/json"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/inhere/fakeserver/internal/config"
)

func pageList(n int) []any {
	out := make([]any, 0, n)
	for i := 1; i <= n; i++ {
		out = append(out, "item-"+string(rune('0'+i)))
	}
	return out
}

// pagedRoute is the shape from the ZY internal-service sample: an envelope with
// data.{current,size,total,list}.
func pagedRoute(method string) *config.Route {
	return &config.Route{
		Method:  []string{method},
		Path:    "/page",
		Headers: map[string]string{"Content-Type": "application/json; charset=utf-8"},
		Body: map[string]any{
			"data": map[string]any{
				"current": 1,
				"size":    50,
				"total":   0,
				"list":    pageList(5),
			},
			"status": 200,
			"code":   0,
		},
		Paginate: &config.PaginateConfig{
			PageField: "current",
			SizeField: "size",
			ListPath:  "data.list",
			TotalPath: "data.total",
		},
	}
}

func postJSON(t *testing.T, url, body string) (*http.Response, map[string]any) {
	t.Helper()
	resp, err := http.Post(url, "application/json", strings.NewReader(body))
	if err != nil {
		t.Fatalf("POST %s: %v", url, err)
	}
	defer resp.Body.Close()
	return resp, decodePage(t, resp)
}

func decodePage(t *testing.T, resp *http.Response) map[string]any {
	t.Helper()
	raw, _ := io.ReadAll(resp.Body)
	var out map[string]any
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatalf("response is not JSON (%q): %v", raw, err)
	}
	return out
}

func pageData(t *testing.T, body map[string]any) map[string]any {
	t.Helper()
	data, ok := body["data"].(map[string]any)
	if !ok {
		t.Fatalf("response data missing: %v", body)
	}
	return data
}

func listOf(t *testing.T, data map[string]any) []any {
	t.Helper()
	list, ok := data["list"].([]any)
	if !ok {
		t.Fatalf("data.list is not a list: %v (%T)", data["list"], data["list"])
	}
	return list
}

// TestPaginate_SlicesListByRequestBodyPage 请求体里的页码/页大小生效，
// totalPath 写入切片前的总数。
func TestPaginate_SlicesListByRequestBodyPage(t *testing.T) {
	ts := mountedServer(t, &config.Config{Routes: []config.Route{*pagedRoute("POST")}})
	defer ts.Close()

	resp, body := postJSON(t, ts.URL+"/page", `{"current":2,"size":2}`)
	if resp.StatusCode != 200 {
		t.Fatalf("status=%d body=%v", resp.StatusCode, body)
	}
	data := pageData(t, body)
	list := listOf(t, data)
	if len(list) != 2 || list[0] != "item-3" || list[1] != "item-4" {
		t.Fatalf("page 2/2 => %v, want [item-3 item-4]", list)
	}
	if total, _ := data["total"].(float64); total != 5 {
		t.Fatalf("total=%v, want 5 (pre-slice length)", data["total"])
	}
	// 响应里保留 body 自身声明的 current/size（不覆盖）
	if cur, _ := data["current"].(float64); cur != 1 {
		t.Fatalf("current=%v, want the body's own value 1", data["current"])
	}
}

// TestPaginate_PageBeyondEndReturnsEmptyList 超出页返回空列表。
func TestPaginate_PageBeyondEndReturnsEmptyList(t *testing.T) {
	ts := mountedServer(t, &config.Config{Routes: []config.Route{*pagedRoute("POST")}})
	defer ts.Close()

	resp, body := postJSON(t, ts.URL+"/page", `{"current":99,"size":2}`)
	if resp.StatusCode != 200 {
		t.Fatalf("status=%d", resp.StatusCode)
	}
	data := pageData(t, body)
	if list := listOf(t, data); len(list) != 0 {
		t.Fatalf("page 99 => %v, want empty list", list)
	}
	if total, _ := data["total"].(float64); total != 5 {
		t.Fatalf("total=%v, want 5", data["total"])
	}
}

// TestPaginate_PageFromQueryParam 请求体没带页码时回退到查询参数。
func TestPaginate_PageFromQueryParam(t *testing.T) {
	ts := mountedServer(t, &config.Config{Routes: []config.Route{*pagedRoute("GET")}})
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/page?current=3&size=2")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	data := pageData(t, decodePage(t, resp))
	list := listOf(t, data)
	if len(list) != 1 || list[0] != "item-5" {
		t.Fatalf("query page 3/2 => %v, want [item-5]", list)
	}
}

// TestPaginate_NoSizeReturnsWholeList size 缺省时整表返回（单页）。
func TestPaginate_NoSizeReturnsWholeList(t *testing.T) {
	ts := mountedServer(t, &config.Config{Routes: []config.Route{*pagedRoute("POST")}})
	defer ts.Close()

	_, body := postJSON(t, ts.URL+"/page", `{"current":1}`)
	if list := listOf(t, pageData(t, body)); len(list) != 5 {
		t.Fatalf("no size => %v, want all 5 items", list)
	}
}

// TestPaginate_CaseIsPickedBeforePaging cases 组合：先选 case，再对选中的 body
// 分页（route 级 paginate 被 case 继承）。
func TestPaginate_CaseIsPickedBeforePaging(t *testing.T) {
	route := pagedRoute("POST")
	route.Strategy = "first-match"
	route.Body = nil
	route.Cases = []config.RouteCase{
		{
			Name: "empty",
			When: `request.body.mode == "empty"`,
			Body: map[string]any{"data": map[string]any{"list": []any{}}, "code": 0},
		},
		{
			Name: "full",
			Body: map[string]any{"data": map[string]any{"list": pageList(4)}, "code": 0},
		},
	}
	ts := mountedServer(t, &config.Config{Routes: []config.Route{*route}})
	defer ts.Close()

	_, body := postJSON(t, ts.URL+"/page", `{"mode":"full","current":2,"size":3}`)
	data := pageData(t, body)
	list := listOf(t, data)
	if len(list) != 1 || list[0] != "item-4" {
		t.Fatalf("case full page 2/3 => %v, want [item-4]", list)
	}
	if total, _ := data["total"].(float64); total != 4 {
		t.Fatalf("total=%v, want 4", data["total"])
	}

	_, empty := postJSON(t, ts.URL+"/page", `{"mode":"empty","current":1,"size":3}`)
	if list := listOf(t, pageData(t, empty)); len(list) != 0 {
		t.Fatalf("empty case => %v, want empty list", list)
	}
}

// TestPaginate_CasePaginateOverridesRoute case 自己的 paginate 覆盖 route 级。
func TestPaginate_CasePaginateOverridesRoute(t *testing.T) {
	route := pagedRoute("POST")
	route.Strategy = "first-match"
	route.Body = nil
	route.Cases = []config.RouteCase{{
		Name: "items",
		Body: map[string]any{"data": map[string]any{"items": pageList(3), "list": pageList(5)}, "code": 0},
		Paginate: &config.PaginateConfig{
			PageField: "page",
			SizeField: "size",
			ListPath:  "data.items",
		},
	}}
	ts := mountedServer(t, &config.Config{Routes: []config.Route{*route}})
	defer ts.Close()

	_, body := postJSON(t, ts.URL+"/page", `{"page":2,"size":2}`)
	data := pageData(t, body)
	if items, ok := data["items"].([]any); !ok || len(items) != 1 || items[0] != "item-3" {
		t.Fatalf("case paginate should slice data.items by page/size: %v", data)
	}
	if list, _ := data["list"].([]any); len(list) != 5 {
		t.Fatalf("data.list must stay untouched when the case overrides listPath: %v", data["list"])
	}
}

// TestPaginate_BodyFileIsPaginated bodyFile 里的列表同样参与分页。
func TestPaginate_BodyFileIsPaginated(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "list.json")
	if err := os.WriteFile(path, []byte(`{"data":{"list":["a","b","c"],"total":0}}`), 0o644); err != nil {
		t.Fatal(err)
	}
	route := &config.Route{
		Method:   []string{"POST"},
		Path:     "/file-page",
		BodyFile: path,
		Paginate: &config.PaginateConfig{PageField: "current", SizeField: "size", ListPath: "data.list", TotalPath: "data.total"},
	}
	ts := mountedServer(t, &config.Config{Routes: []config.Route{*route}})
	defer ts.Close()

	_, body := postJSON(t, ts.URL+"/file-page", `{"current":2,"size":2}`)
	data := pageData(t, body)
	list := listOf(t, data)
	if len(list) != 1 || list[0] != "c" {
		t.Fatalf("bodyFile page 2/2 => %v, want [c]", list)
	}
	if total, _ := data["total"].(float64); total != 3 {
		t.Fatalf("total=%v, want 3", data["total"])
	}
}

// TestPaginate_UnresolvableListPathReturns500 配置的 listPath 在响应体里不存在时
// 明确报错，不静默返回未分页数据。
func TestPaginate_UnresolvableListPathReturns500(t *testing.T) {
	route := pagedRoute("POST")
	route.Paginate = &config.PaginateConfig{ListPath: "data.missing"}
	ts := mountedServer(t, &config.Config{Routes: []config.Route{*route}})
	defer ts.Close()

	resp, body := postJSON(t, ts.URL+"/page", `{"current":1,"size":2}`)
	if resp.StatusCode != 500 {
		t.Fatalf("status=%d, want 500", resp.StatusCode)
	}
	if !strings.Contains(strings.ToLower(toString(body["error"])), "paginate") {
		t.Fatalf("error should mention paginate: %v", body)
	}
}

func toString(v any) string {
	s, _ := v.(string)
	return s
}
