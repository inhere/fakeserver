// Package cli wires up the fakeserver command-line interface.
//
// All command definitions live here; cmd/fakeserver/main.go is intentionally
// thin and only invokes Run().
package cli

import (
	"fmt"
	"os"

	"github.com/gookit/gcli/v3"
	"github.com/inhere/fakeserver/internal/buildinfo"
)

// Run 构建 fakeserver CLI app，注册所有子命令并启动。
// version 通常由 cmd/fakeserver/main.go 透传，build 时通过 ldflags 注入。
func Run(version string) {
	app := gcli.NewApp(func(a *gcli.App) {
		a.Name = "fakeserver"
		a.Desc = "Configurable HTTP mock/fake server tool"
		a.Version = fmt.Sprintf("%s, %s, %s", buildinfo.Version, buildinfo.GitHash, buildinfo.BuildTime)
	})
	app.Add(newServeCmd())
	app.Add(newInitCmd())
	app.Add(newCheckCmd())
	app.Add(newDoctorCmd())
	app.Add(newRoutesCmd())
	app.Add(newListCmd())
	app.Add(newUseCmd())
	code := app.Run(nil)
	os.Exit(code)
}
