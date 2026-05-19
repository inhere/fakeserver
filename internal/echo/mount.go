// Package echo provides a minimal httpbin-style echo handler set used when
// fakeserver runs without user-defined routes.
//
// rux v1.4.1 ships an /{all} echo handler in its server/ sub-package, but it
// does not expose discrete httpbin-style endpoints (/status, /delay, /headers,
// /ip). To keep our router intact, we do NOT use rux/server here; instead we
// reuse goutil/testutil.BuildEchoReply for the response payload structure
// (method/url/headers/query/form/json/body), and add our own status/delay/
// headers/ip endpoints. See internal/echo/probe.md for the discovery notes.
package echo

import (
	"net/http"
	"strconv"
	"time"

	"github.com/gookit/goutil/testutil"
	"github.com/gookit/rux"
)

// Mount 把固定的 echo 端点和 NotFound 兜底注册到 router。
//
// 端点列表：
//   - /anything           Any → JSON 回显 (method/url/headers/query/body 等)
//   - /anything/*rest     同上
//   - /headers            GET → JSON，仅含 headers
//   - /ip                 GET → JSON，仅含 ip
//   - /status/{code}      GET → 直接以 {code} 返回，无 body；非法 code 返回 400
//   - /delay/{seconds}    GET → sleep 后走 anything 回显；秒数被夹到 [0,10]
//   - NotFound 兜底       → anything 回显
func Mount(r *rux.Router) {
	r.Any("/anything", anythingHandler)
	r.Any("/anything/*rest", anythingHandler)
	r.GET("/headers", headersHandler)
	r.GET("/ip", ipHandler)
	r.GET("/status/{code}", statusHandler)
	r.GET("/delay/{seconds}", delayHandler)
	r.NotFound(anythingHandler)
}

func anythingHandler(c *rux.Context) {
	data := testutil.BuildEchoReply(c.Req)
	c.JSON(http.StatusOK, data)
}

func headersHandler(c *rux.Context) {
	hdrs := make(map[string]string, len(c.Req.Header))
	for k, vs := range c.Req.Header {
		if len(vs) > 0 {
			hdrs[k] = vs[0]
		}
	}
	c.JSON(http.StatusOK, map[string]any{"headers": hdrs})
}

func ipHandler(c *rux.Context) {
	ip := c.Req.Header.Get("X-Forwarded-For")
	if ip == "" {
		ip = c.Req.RemoteAddr
	}
	c.JSON(http.StatusOK, map[string]any{"ip": ip})
}

func statusHandler(c *rux.Context) {
	code, err := strconv.Atoi(c.Param("code"))
	if err != nil || code < 100 || code > 599 {
		c.JSON(http.StatusBadRequest, map[string]any{"error": "invalid status code"})
		return
	}
	c.Resp.WriteHeader(code)
}

func delayHandler(c *rux.Context) {
	sec, err := strconv.Atoi(c.Param("seconds"))
	if err != nil || sec < 0 {
		c.JSON(http.StatusBadRequest, map[string]any{"error": "invalid seconds"})
		return
	}
	if sec > 10 {
		sec = 10
	}
	time.Sleep(time.Duration(sec) * time.Second)
	anythingHandler(c)
}
