package buildinfo

import (
	"runtime/debug"
	"strings"
)

var (
	Version   string
	BuildTime string
	GitHash   string
)

func Set(verStr, btime, gitHash string) {
	Version = verStr
	BuildTime = btime
	GitHash = gitHash
}

func FromBuildInfo(ver, bt, hash string, info *debug.BuildInfo) (string, string, string) {
	if info == nil || (ver != "" && ver != "dev") {
		return ver, bt, hash
	}
	if info.Main.Version != "" && info.Main.Version != "(devel)" {
		ver = info.Main.Version
	}
	for _, s := range info.Settings {
		switch s.Key {
		case "vcs.revision":
			if len(s.Value) >= 7 {
				hash = s.Value[:7]
			} else if s.Value != "" {
				hash = s.Value
			}
		case "vcs.modified":
			if s.Value == "true" && !strings.HasSuffix(hash, "-dirty") {
				hash += "-dirty"
			}
		case "vcs.time":
			bt = s.Value
		}
	}
	return ver, bt, hash
}
