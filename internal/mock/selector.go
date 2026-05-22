package mock

import (
	"errors"
	"math/rand"
	"sync/atomic"

	"github.com/inhere/fakeserver/internal/config"
)

// ErrNoMatch indicates Selector.Pick was called with an empty case set.
// In practice this happens when all when-conditions filtered out their
// cases (or the route had no cases at all). The responder maps this to
// the design §6 "no case matched" 500 response.
var ErrNoMatch = errors.New("no case matched")

// SelectorCase carries the minimal data a Selector needs to make a pick.
// Caller (cases.go) constructs the slice after running Matcher.Evaluate,
// so this slice already only contains cases whose when evaluated to true.
//
// OrigIdx is the original index in route.Cases, so Pick's return value
// addresses the *config* cases slice directly — no double indirection at
// render time.
type SelectorCase struct {
	OrigIdx int
	Weight  int
}

// Selector picks one SelectorCase index (OrigIdx) given the already-
// filtered candidate set. Implementations must be safe for concurrent
// Pick across goroutines (per-request handler invocations run in parallel).
type Selector interface {
	Pick(cases []SelectorCase) (int, error)
}

// NewSelector returns the strategy implementation by name. Empty string
// defaults to "random" (design §3.2). Unknown strategies also fall back
// to random — config.Validate is the source of truth for catching typos
// at startup; this fallback exists for runtime robustness.
func NewSelector(strategy string) Selector {
	switch strategy {
	case "first-match":
		return &firstMatchSelector{}
	case "round-robin":
		return &roundRobinSelector{}
	case "weighted":
		return &weightedSelector{}
	case "", "random":
		return &randomSelector{}
	default:
		return &randomSelector{}
	}
}

func pickCaseByName(cases []config.RouteCase, name string) (int, bool) {
	if name == "" {
		return 0, false
	}
	for i := range cases {
		if cases[i].Name == name {
			return i, true
		}
	}
	return 0, false
}

type randomSelector struct{}

func (s *randomSelector) Pick(cases []SelectorCase) (int, error) {
	if len(cases) == 0 {
		return 0, ErrNoMatch
	}
	return cases[rand.Intn(len(cases))].OrigIdx, nil
}

type firstMatchSelector struct{}

func (s *firstMatchSelector) Pick(cases []SelectorCase) (int, error) {
	if len(cases) == 0 {
		return 0, ErrNoMatch
	}
	return cases[0].OrigIdx, nil
}

// roundRobinSelector keeps a per-instance atomic counter. Two routes that
// both use round-robin get independent counters (the router creates one
// Selector per route in Task 5).
type roundRobinSelector struct {
	counter atomic.Uint64
}

func (s *roundRobinSelector) Pick(cases []SelectorCase) (int, error) {
	if len(cases) == 0 {
		return 0, ErrNoMatch
	}
	// Subtract 1 so first call lands on index 0 (Add returns the *new* value).
	n := s.counter.Add(1) - 1
	return cases[int(n%uint64(len(cases)))].OrigIdx, nil
}

// weightedSelector picks proportional to Weight, treating Weight<=0 as 1.
type weightedSelector struct{}

func (s *weightedSelector) Pick(cases []SelectorCase) (int, error) {
	if len(cases) == 0 {
		return 0, ErrNoMatch
	}
	total := 0
	for _, c := range cases {
		w := c.Weight
		if w <= 0 {
			w = 1
		}
		total += w
	}
	r := rand.Intn(total)
	acc := 0
	for _, c := range cases {
		w := c.Weight
		if w <= 0 {
			w = 1
		}
		acc += w
		if r < acc {
			return c.OrigIdx, nil
		}
	}
	// Unreachable given total>0 and r<total, but fall back defensively.
	return cases[len(cases)-1].OrigIdx, nil
}
