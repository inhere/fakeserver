package tpl

import (
	"testing"

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
