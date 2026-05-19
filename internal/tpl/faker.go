package tpl

import (
	"text/template"

	_ "github.com/brianvoe/gofakeit/v7" // Task 4 fills in real bridging
)

// fakerFuncs is filled by Task 4 with ~20 fakeXxx functions plus the
// generic `fake "<name>"`. For Task 3 we return an empty map so
// BaseFuncMap compiles.
func fakerFuncs() template.FuncMap {
	return template.FuncMap{}
}
