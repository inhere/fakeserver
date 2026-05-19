// Package cli wires up the fakeserver command-line interface.
//
// All command definitions live here; cmd/fakeserver/main.go is intentionally
// thin and only invokes Run().
package cli

import (
	"github.com/gookit/gcli/v3"
)

// Run 构建 fakeserver CLI app，注册所有子命令并启动。
// version 通常由 cmd/fakeserver/main.go 透传，build 时通过 ldflags 注入。
func Run(version string) {
	app := gcli.NewApp(func(a *gcli.App) {
		a.Name = "fakeserver"
		a.Version = version
		a.Desc = "Configurable HTTP mock/fake server"
	})
	app.Add(newServeCmd())
	app.Add(newInitCmd())
	app.Add(newCheckCmd())
	// 未来子命令在此追加：
	//   app.Add(newInitCmd())   // Phase 2
	//   app.Add(newCheckCmd())  // Phase 2
	//   app.Add(newRoutesCmd()) // Phase 2
	//   app.Add(newListCmd())   // Phase 3
	//   app.Add(newUseCmd())    // Phase 3
	app.Run(nil)
}
