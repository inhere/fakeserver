package buildinfo

import (
	"runtime/debug"
	"testing"
)

func TestFromBuildInfoInjected(t *testing.T) {
	ver, bt, hash := FromBuildInfo("0.7.0-22-gabc1234", "time", "abc1234", &debug.BuildInfo{Main: debug.Module{Version: "v0.0.0"}})
	if ver != "0.7.0-22-gabc1234" || bt != "time" || hash != "abc1234" {
		t.Fatalf("got %q %q %q", ver, bt, hash)
	}
}
func TestFromBuildInfoFallback(t *testing.T) {
	info := &debug.BuildInfo{Main: debug.Module{Version: "v1.2.3"}, Settings: []debug.BuildSetting{{Key: "vcs.revision", Value: "1234567890"}, {Key: "vcs.time", Value: "2026-09-12T00:00:00Z"}}}
	ver, bt, hash := FromBuildInfo("dev", "", "", info)
	if ver != "v1.2.3" || bt != "2026-09-12T00:00:00Z" || hash != "1234567" {
		t.Fatalf("got %q %q %q", ver, bt, hash)
	}
}
func TestFromBuildInfoFallbackNoVCS(t *testing.T) {
	ver, bt, hash := FromBuildInfo("dev", "", "", &debug.BuildInfo{Main: debug.Module{Version: "(devel)"}})
	if ver != "dev" || bt != "" || hash != "" {
		t.Fatalf("got %q %q %q", ver, bt, hash)
	}
}
func TestFromBuildInfoModified(t *testing.T) {
	_, _, hash := FromBuildInfo("dev", "", "", &debug.BuildInfo{Main: debug.Module{Version: "v1"}, Settings: []debug.BuildSetting{{Key: "vcs.revision", Value: "abcdefghi"}, {Key: "vcs.modified", Value: "true"}}})
	if hash != "abcdefg-dirty" {
		t.Fatalf("hash=%q", hash)
	}
}
