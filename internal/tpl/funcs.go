package tpl

import (
	"text/template"

	"github.com/gookit/easytpl/tplfunc"
)

// BaseFuncMap returns the merged FuncMap shared by text and html renderers.
//
// Order of precedence (later wins on conflict):
//  1. tplfunc.StdFuncMap()              ← gookit 提供的基础（join/trim/upper/lower/...）
//  2. delete env/expandenv             ← tplfunc 的 env 取 os.Getenv，与我们的命名空间冲突
//  3. fakeserverFuncs (funcs_extra.go) ← 我们的 22+ 自有函数
//  4. fakerFuncs (faker.go, Task 4)    ← gofakeit 桥接的 fakeXxx + 通用 fake
//
// osenvWhitelist controls which OS environment keys the osenv() function
// may return. Empty slice / nil means "no restriction" (development-friendly).
func BaseFuncMap(osenvWhitelist []string) template.FuncMap {
	out := template.FuncMap{}
	for k, v := range tplfunc.StdFuncMap() {
		out[k] = v
	}
	delete(out, "env")
	delete(out, "expandenv")

	for k, v := range fakeserverFuncs(osenvWhitelist) {
		out[k] = v
	}
	for k, v := range fakerFuncs() {
		out[k] = v
	}
	return out
}
