// Command fakeserver is the entry point for the fakeserver binary.
//
// All CLI logic lives in internal/cli; this file is intentionally minimal
// so that build/embedding code stays trivial and isolated from command
// definitions.
package main

import (
	"github.com/inhere/fakeserver/internal/buildinfo"
	"github.com/inhere/fakeserver/internal/cli"
)

// 在 build 时通过 ldflags 注入：-X main.version=v0.1.0
var (
	Version   = "0.1.0"
	BuildTime = ""
	GitHash   = "unknown"
)

func main() {
	buildinfo.Set(Version, BuildTime, GitHash)
	cli.Run(Version)
}
