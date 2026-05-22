package scenario

import (
	"fmt"
	"strings"
	"sync"
)

type RouteKey struct {
	Method string `json:"method"`
	Path   string `json:"path"`
}

func NewRouteKey(method, path string) RouteKey {
	return RouteKey{Method: strings.ToUpper(strings.TrimSpace(method)), Path: strings.TrimSpace(path)}
}

func (k RouteKey) String() string {
	return strings.ToUpper(strings.TrimSpace(k.Method)) + " " + strings.TrimSpace(k.Path)
}

func ParseRouteKey(sig string) (RouteKey, error) {
	method, path, ok := strings.Cut(strings.TrimSpace(sig), " ")
	if !ok || strings.TrimSpace(method) == "" || strings.TrimSpace(path) == "" {
		return RouteKey{}, fmt.Errorf("invalid route signature %q", sig)
	}
	return NewRouteKey(method, path), nil
}

type Override struct {
	CaseName  string `json:"caseName"`
	Mode      string `json:"mode"`
	Remaining int    `json:"remaining,omitempty"`
}

type State struct {
	Selected  string              `json:"selected"`
	Overrides map[string]Override `json:"overrides"`
}

type Store struct {
	mu        sync.RWMutex
	selected  string
	overrides map[RouteKey]Override
}

func NewStore() *Store {
	return &Store{overrides: map[RouteKey]Override{}}
}

func (s *Store) Selected() string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.selected
}

func (s *Store) SetSelected(name string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.selected = strings.TrimSpace(name)
}

func (s *Store) SetOverride(key RouteKey, ov Override) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if ov.Mode == "" {
		ov.Mode = "always"
	}
	if ov.Mode == "count" && ov.Remaining <= 0 {
		ov.Remaining = 1
	}
	s.overrides[NewRouteKey(key.Method, key.Path)] = ov
}

func (s *Store) ClearOverride(key RouteKey) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.overrides, NewRouteKey(key.Method, key.Path))
}

func (s *Store) ConsumeOverride(key RouteKey) (Override, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()

	key = NewRouteKey(key.Method, key.Path)
	ov, ok := s.overrides[key]
	if !ok {
		return Override{}, false
	}

	switch ov.Mode {
	case "next":
		delete(s.overrides, key)
	case "count":
		ov.Remaining--
		if ov.Remaining <= 0 {
			delete(s.overrides, key)
		} else {
			s.overrides[key] = ov
		}
	}
	return ov, true
}

func (s *Store) Snapshot() State {
	s.mu.RLock()
	defer s.mu.RUnlock()

	state := State{Selected: s.selected, Overrides: map[string]Override{}}
	for key, ov := range s.overrides {
		state.Overrides[key.String()] = ov
	}
	return state
}
