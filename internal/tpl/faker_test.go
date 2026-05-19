package tpl

import (
	"strings"
	"testing"
	"text/template"

	"github.com/brianvoe/gofakeit/v7"
)

// TestGofakeitSeedReproducible 锁定 Seed 行为：相同 seed → 相同结果。
// design §12.1 依赖此特性提供"测试场景固定 seed，结果可复现"。
func TestGofakeitSeedReproducible(t *testing.T) {
	gofakeit.Seed(int64(12345))
	first := gofakeit.Name()

	gofakeit.Seed(int64(12345))
	second := gofakeit.Name()

	if first != second {
		t.Errorf("same seed should yield same Name; got %q vs %q", first, second)
	}
}

func TestGofakeitBasicFunctions(t *testing.T) {
	gofakeit.Seed(int64(1))

	if name := gofakeit.Name(); name == "" {
		t.Error("Name() returned empty")
	}
	if email := gofakeit.Email(); email == "" {
		t.Error("Email() returned empty")
	}
	if ip := gofakeit.IPv4Address(); ip == "" {
		t.Error("IPv4Address() returned empty")
	}
}

// renderInlineFaker 是测试辅助，类似 funcs_test.go 中的 renderInline 但
// 专门用于 faker 函数（固定 seed 确保测试可重现）。
func renderInlineFaker(t *testing.T, src string) string {
	t.Helper()
	gofakeit.Seed(int64(42))
	tpl, err := template.New("t").Funcs(BaseFuncMap(nil)).Parse(src)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	var sb strings.Builder
	if err := tpl.Execute(&sb, nil); err != nil {
		t.Fatalf("exec: %v", err)
	}
	return sb.String()
}

func TestFakerCommonFunctions(t *testing.T) {
	for _, fn := range []string{
		"fakeName", "fakeFirstName", "fakeLastName",
		"fakeEmail", "fakeUsername", "fakePhone",
		"fakeCity", "fakeCountry", "fakeAddress",
		"fakeIPv4", "fakeURL", "fakeUserAgent",
		"fakeCompany", "fakeJob",
		"fakeWord", "fakeSentence",
		"fakePastDate", "fakeFutureDate",
	} {
		out := renderInlineFaker(t, "{{ "+fn+" }}")
		if out == "" {
			t.Errorf("%s returned empty", fn)
		}
	}
}

func TestFakerIntRange(t *testing.T) {
	for i := 0; i < 20; i++ {
		out := renderInlineFaker(t, `{{ fakeIntRange 10 20 }}`)
		if len(out) < 2 {
			t.Errorf("fakeIntRange: got %q", out)
		}
	}
}

// TestFakerGenericInput verifies the catch-all `fake "<name>"` entry. The
// chosen name must be a real gofakeit identifier that produces a string.
func TestFakerGenericInput(t *testing.T) {
	// "color" 是 gofakeit.Color() 的别名；广泛存在于 v7
	out := renderInlineFaker(t, `{{ fake "color" }}`)
	if out == "" {
		t.Errorf("fake \"color\" returned empty")
	}
}

// TestFakerGenericUnknownName 未识别 name 应回退到空串。
func TestFakerGenericUnknownName(t *testing.T) {
	out := renderInlineFaker(t, `{{ fake "definitely_not_a_gofakeit_name_xyz" }}`)
	if out != "" {
		t.Errorf("fake unknown name should be empty; got %q", out)
	}
}

func TestFakerIPv6(t *testing.T) {
	out := renderInlineFaker(t, `{{ fakeIPv6 }}`)
	if out == "" {
		t.Error("fakeIPv6 empty")
	}
}

func TestFakerZip(t *testing.T) {
	out := renderInlineFaker(t, `{{ fakeZip }}`)
	if out == "" {
		t.Error("fakeZip empty")
	}
}

func TestFakerParagraph(t *testing.T) {
	out := renderInlineFaker(t, `{{ fakeParagraph }}`)
	if out == "" {
		t.Error("fakeParagraph empty")
	}
}

func TestFakerDate(t *testing.T) {
	out := renderInlineFaker(t, `{{ (fakeDate).Format "2006" }}`)
	if len(out) != 4 {
		t.Errorf("fakeDate.Format year: got %q", out)
	}
}

func TestFakerFloatRange(t *testing.T) {
	out := renderInlineFaker(t, `{{ fakeFloatRange 1.0 10.0 }}`)
	if out == "" {
		t.Error("fakeFloatRange empty")
	}
	// max<min 边界 → 返回 min
	out = renderInlineFaker(t, `{{ fakeFloatRange 10.0 1.0 }}`)
	if out != "10" {
		t.Errorf("fakeFloatRange max<min should return min; got %q", out)
	}
}

func TestFakerIntRangeMaxLessThanMin(t *testing.T) {
	out := renderInlineFaker(t, `{{ fakeIntRange 50 10 }}`)
	if out != "50" {
		t.Errorf("fakeIntRange max<min should return min; got %q", out)
	}
}
