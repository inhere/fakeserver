package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"runtime"

	"github.com/gookit/gcli/v3"

	"github.com/inhere/fakeserver/internal/buildinfo"
)

type versionOptions struct {
	JSON bool
	out  io.Writer
}

// versionLine is the single source of truth for the application version text,
// shared by the global `--version` flag (app.Version) and the `version`
// subcommand so both always print the same string.
func versionLine() string {
	return fmt.Sprintf("%s, %s, %s", buildinfo.Version, buildinfo.GitHash, buildinfo.BuildTime)
}

func newVersionCmd() *gcli.Command {
	opts := versionOptions{}
	return &gcli.Command{
		Name: "version",
		Desc: "Print version information (same text as --version; --json for machine output)",
		Config: func(cmd *gcli.Command) {
			cmd.BoolOpt2(&opts.JSON, "json", "Print version information as JSON")
		},
		Func: func(cmd *gcli.Command, _ []string) error {
			opts.out = os.Stdout
			return runVersion(opts)
		},
	}
}

func runVersion(opts versionOptions) error {
	if opts.out == nil {
		opts.out = os.Stdout
	}
	if !opts.JSON {
		_, err := fmt.Fprintf(opts.out, "Version: %s\n", versionLine())
		return err
	}
	info := struct {
		Version   string `json:"version"`
		Commit    string `json:"commit"`
		BuildTime string `json:"buildTime"`
		GoVersion string `json:"goVersion"`
	}{
		Version:   buildinfo.Version,
		Commit:    buildinfo.GitHash,
		BuildTime: buildinfo.BuildTime,
		GoVersion: runtime.Version(),
	}
	data, err := json.Marshal(info)
	if err != nil {
		return err
	}
	_, err = fmt.Fprintf(opts.out, "%s\n", data)
	return err
}
