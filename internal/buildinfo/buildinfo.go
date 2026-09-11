package buildinfo

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
