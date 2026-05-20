package mock

import (
	"errors"
	"math"
	"testing"
)

func makeCases(weights ...int) []SelectorCase {
	out := make([]SelectorCase, len(weights))
	for i, w := range weights {
		out[i] = SelectorCase{OrigIdx: i, Weight: w}
	}
	return out
}

func TestSelector_Empty_ReturnsErrNoMatch(t *testing.T) {
	strategies := []string{"", "random", "round-robin", "weighted", "first-match"}
	for _, strat := range strategies {
		t.Run(strat, func(t *testing.T) {
			sel := NewSelector(strat)
			_, err := sel.Pick(nil)
			if !errors.Is(err, ErrNoMatch) {
				t.Errorf("nil cases for %s: want ErrNoMatch, got %v", strat, err)
			}
		})
	}
}

func TestSelector_FirstMatch(t *testing.T) {
	sel := NewSelector("first-match")
	picked, err := sel.Pick([]SelectorCase{
		{OrigIdx: 1, Weight: 0},
		{OrigIdx: 3, Weight: 0},
	})
	if err != nil {
		t.Fatal(err)
	}
	if picked != 1 {
		t.Errorf("first-match should return first OrigIdx, got %d", picked)
	}
}

func TestSelector_RoundRobin_PerRouteCounter(t *testing.T) {
	sel := NewSelector("round-robin")
	cases := makeCases(0, 0, 0)
	got := []int{}
	for i := 0; i < 6; i++ {
		idx, err := sel.Pick(cases)
		if err != nil {
			t.Fatal(err)
		}
		got = append(got, idx)
	}
	want := []int{0, 1, 2, 0, 1, 2}
	for i := range got {
		if got[i] != want[i] {
			t.Errorf("round-robin step %d: got %d want %d (full=%v)", i, got[i], want[i], got)
		}
	}
}

func TestSelector_RoundRobin_Isolated(t *testing.T) {
	s1 := NewSelector("round-robin")
	s2 := NewSelector("round-robin")
	cases := makeCases(0, 0)
	idx1a, _ := s1.Pick(cases)
	idx2a, _ := s2.Pick(cases)
	if idx1a != idx2a {
		t.Errorf("first pick should be deterministic (0); got s1=%d s2=%d", idx1a, idx2a)
	}
	idx1b, _ := s1.Pick(cases)
	idx2b, _ := s2.Pick(cases)
	if idx1b != 1 || idx2b != 1 {
		t.Errorf("independent counters: s1=%d s2=%d want both=1", idx1b, idx2b)
	}
}

func TestSelector_Random_HitsAllCasesEventually(t *testing.T) {
	sel := NewSelector("random")
	cases := makeCases(0, 0, 0)
	hit := map[int]int{}
	for i := 0; i < 300; i++ {
		idx, err := sel.Pick(cases)
		if err != nil {
			t.Fatal(err)
		}
		hit[idx]++
	}
	for _, oi := range []int{0, 1, 2} {
		if hit[oi] == 0 {
			t.Errorf("random: OrigIdx %d never hit after 300 picks (dist=%v)", oi, hit)
		}
	}
}

// TestSelector_Weighted_Distribution 锁定 DoD #3：1000 次按权重分布 ±5%
// 容差。权重 1:9 → 期望 10% : 90%。
func TestSelector_Weighted_Distribution(t *testing.T) {
	sel := NewSelector("weighted")
	cases := []SelectorCase{
		{OrigIdx: 0, Weight: 1},
		{OrigIdx: 1, Weight: 9},
	}
	// N=3000 keeps 3σ comfortably inside the ±5% tolerance, halving CI flake
	// risk vs. the plan's nominal N=1000.
	const N = 3000
	hit := map[int]int{}
	for i := 0; i < N; i++ {
		idx, err := sel.Pick(cases)
		if err != nil {
			t.Fatal(err)
		}
		hit[idx]++
	}
	exp0, exp1 := 0.1, 0.9
	got0 := float64(hit[0]) / N
	got1 := float64(hit[1]) / N
	if math.Abs(got0-exp0) > 0.05 {
		t.Errorf("weighted dist OrigIdx 0: got %.3f want %.3f ±0.05", got0, exp0)
	}
	if math.Abs(got1-exp1) > 0.05 {
		t.Errorf("weighted dist OrigIdx 1: got %.3f want %.3f ±0.05", got1, exp1)
	}
}

// TestSelector_Weighted_ZeroDefaultsToOne 锁定：Weight<=0 等价于 1。
func TestSelector_Weighted_ZeroDefaultsToOne(t *testing.T) {
	sel := NewSelector("weighted")
	cases := makeCases(0, 0)
	// N=3000 keeps 3σ comfortably inside the ±5% tolerance, halving CI flake
	// risk vs. the plan's nominal N=1000.
	const N = 3000
	hit := map[int]int{}
	for i := 0; i < N; i++ {
		idx, _ := sel.Pick(cases)
		hit[idx]++
	}
	got0 := float64(hit[0]) / N
	if math.Abs(got0-0.5) > 0.05 {
		t.Errorf("weight=0 should default to 1; got dist %v", hit)
	}
}

func TestSelector_Unknown_FallsBackToRandom(t *testing.T) {
	sel := NewSelector("does-not-exist")
	cases := makeCases(0, 0, 0)
	for i := 0; i < 50; i++ {
		idx, err := sel.Pick(cases)
		if err != nil {
			t.Fatalf("fallback pick err=%v", err)
		}
		if idx < 0 || idx > 2 {
			t.Fatalf("fallback yielded out-of-range OrigIdx %d", idx)
		}
	}
}
