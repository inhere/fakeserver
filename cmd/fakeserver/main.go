// Command fakeserver is the entry point for the fakeserver binary.
//
// All CLI logic lives in internal/cli; this file is intentionally minimal
// so that build/embedding code stays trivial and isolated from command
// definitions.
package main

import (
	"github.com/inhere/fakeserver/internal/buildinfo"
	"github.com/inhere/fakeserver/internal/cli"
	"runtime/debug"
)

// 在 build 时通过 ldflags 注入：-X main.Version=v0.8.0
var (
	Version   = "dev"
	BuildTime = ""
	GitHash   = "unknown"
)

func main() {
	Version, BuildTime, GitHash = buildinfo.FromBuildInfo(Version, BuildTime, GitHash, func() *debug.BuildInfo { i, _ := debug.ReadBuildInfo(); return i }())
	buildinfo.Set(Version, BuildTime, GitHash)
	cli.Run(Version)
}
