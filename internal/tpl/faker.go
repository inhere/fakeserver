package tpl

import (
	"text/template"
	"time"

	"github.com/brianvoe/gofakeit/v7"
)

// fakerFuncs returns the FuncMap that bridges gofakeit into our renderer.
// design §12.2 splits this into two entry styles:
//   A. 20 commonly-needed fakeXxx functions (zero-arg or simple-arg)
//   B. a generic `fake "<name>"` lookup for the long tail
//
// Seed control happens externally (Task 6's NewRenderer calls gofakeit.Seed
// once with cfg.Server.FakerSeed before any render). All functions here
// are pure wrappers — they never call Seed themselves.
func fakerFuncs() template.FuncMap {
	return template.FuncMap{
		// --- person / contact ---
		"fakeName":      gofakeit.Name,
		"fakeFirstName": gofakeit.FirstName,
		"fakeLastName":  gofakeit.LastName,
		"fakeEmail":     gofakeit.Email,
		"fakeUsername":  gofakeit.Username,
		"fakePhone":     gofakeit.Phone,

		// --- geography ---
		"fakeCity":    gofakeit.City,
		"fakeCountry": gofakeit.Country,
		"fakeAddress": func() string { return gofakeit.Address().Address },
		"fakeZip":     gofakeit.Zip,

		// --- network ---
		"fakeIPv4":      gofakeit.IPv4Address,
		"fakeIPv6":      gofakeit.IPv6Address,
		"fakeURL":       gofakeit.URL,
		"fakeUserAgent": gofakeit.UserAgent,

		// --- business / content ---
		"fakeCompany":   gofakeit.Company,
		"fakeJob":       gofakeit.JobTitle,
		"fakeWord":      gofakeit.Word,
		"fakeSentence":  gofakeit.Sentence,
		"fakeParagraph": gofakeit.Paragraph,

		// --- numeric / time ---
		"fakeIntRange": func(min, max int) int {
			if max < min {
				return min
			}
			return gofakeit.Number(min, max)
		},
		"fakeFloatRange": func(min, max float64) float64 {
			if max < min {
				return min
			}
			return gofakeit.Float64Range(min, max)
		},
		"fakeDate":       gofakeit.Date,
		"fakePastDate":   func() time.Time { return gofakeit.DateRange(time.Now().AddDate(-1, 0, 0), time.Now()) },
		"fakeFutureDate": func() time.Time { return gofakeit.DateRange(time.Now(), time.Now().AddDate(1, 0, 0)) },

		// --- generic entry ---
		"fake": fakeGeneric,
	}
}

// fakeGeneric dispatches to gofakeit by string name. Names follow
// gofakeit's identifier convention (lowercase, no separator), e.g.
// "color", "carmaker", "creditcardnumber". Unknown names return "".
//
// gofakeit.Generate accepts the curly-brace template syntax "{name}" and
// returns (string, error). Unknown names cause it to return the original
// placeholder unchanged — we detect that and return "" instead.
func fakeGeneric(name string) string {
	out, _ := gofakeit.Generate("{" + name + "}")
	if out == "{"+name+"}" {
		return ""
	}
	return out
}
