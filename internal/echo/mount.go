// Package echo provides the default httpbin-style echo handlers used when
// fakeserver runs without any user-defined route configuration.
//
// rux v2's server/ sub-package ships a comprehensive set of httpbin-style
// endpoints (/anything, /headers, /ip, /status/{code}, /delay/{seconds},
// /uuid, /redirect, /cookies, /basic-auth, /bytes, /download, /upload, …)
// plus an HTML index at /. We delegate to server.MountEchoRoutes() so
// fakeserver inherits all of them with one line.
//
// See internal/echo/probe.md for the discovery notes that led to this
// design.
package echo

import (
	"github.com/gookit/rux/v2"
	"github.com/gookit/rux/v2/server"
)

// Mount attaches rux v2's full httpbin-style echo endpoint set to the
// given router. The caller's router is otherwise untouched — additional
// admin or user-defined routes registered before/after Mount remain
// effective, with rux's static > param > wildcard priority deciding
// matches (the /*path catch-all that MountEchoRoutes registers only
// fires when nothing more specific wins).
func Mount(r *rux.Router) {
	server.MountEchoRoutes(r)
}
