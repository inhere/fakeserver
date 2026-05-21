package webui

import (
	"embed"
	"io/fs"
	"mime"
	"net/http"
	"path"
	"strings"

	"github.com/gookit/rux/v2"
)

//go:embed assets/*
var embeddedAssets embed.FS

func uiAssetsHandler() rux.HandlerFunc {
	sub, err := fs.Sub(embeddedAssets, "assets")
	if err != nil {
		panic(err)
	}
	return func(c *rux.Context) {
		name := strings.TrimPrefix(c.Req.URL.Path, "/__fakeserver/ui/")
		if name == "" || c.Req.URL.Path == "/__fakeserver/ui" {
			name = "index.html"
		}
		name = path.Clean("/" + name)[1:]
		if name == "." || strings.HasPrefix(name, "../") {
			http.NotFound(c.Resp, c.Req)
			return
		}
		data, rerr := fs.ReadFile(sub, name)
		if rerr != nil {
			http.NotFound(c.Resp, c.Req)
			return
		}
		if ct := mime.TypeByExtension(path.Ext(name)); ct != "" {
			c.Resp.Header().Set("Content-Type", ct)
		}
		c.Resp.WriteHeader(http.StatusOK)
		_, _ = c.Resp.Write(data)
	}
}
