package scenario

import "testing"

func TestRouteKeySignature(t *testing.T) {
	key := NewRouteKey("get", "/api/users")
	if key.Method != "GET" || key.Path != "/api/users" {
		t.Fatalf("NewRouteKey = %#v", key)
	}
	if got := key.String(); got != "GET /api/users" {
		t.Fatalf("String() = %q, want route signature", got)
	}

	parsed, err := ParseRouteKey(" post /api/users ")
	if err != nil {
		t.Fatalf("ParseRouteKey unexpected error: %v", err)
	}
	if parsed.Method != "POST" || parsed.Path != "/api/users" {
		t.Fatalf("ParseRouteKey = %#v", parsed)
	}
	if _, err := ParseRouteKey("GET"); err == nil {
		t.Fatal("ParseRouteKey should reject a signature without path")
	}
}

func TestStoreSelectedScenario(t *testing.T) {
	s := NewStore()
	if got := s.Selected(); got != "" {
		t.Fatalf("initial selected = %q, want empty", got)
	}
	s.SetSelected("emptyUsers")
	if got := s.Selected(); got != "emptyUsers" {
		t.Fatalf("selected = %q, want emptyUsers", got)
	}
	s.SetSelected("")
	if got := s.Selected(); got != "" {
		t.Fatalf("selected after clear = %q, want empty", got)
	}
}

func TestStoreSnapshotUsesRouteSignatures(t *testing.T) {
	s := NewStore()
	s.SetSelected("emptyUsers")
	s.SetOverride(NewRouteKey("get", "/api/users"), Override{CaseName: "empty", Mode: "always"})

	snapshot := s.Snapshot()
	if snapshot.Selected != "emptyUsers" {
		t.Fatalf("snapshot selected = %q, want emptyUsers", snapshot.Selected)
	}
	ov, ok := snapshot.Overrides["GET /api/users"]
	if !ok || ov.CaseName != "empty" {
		t.Fatalf("snapshot override = %#v ok=%v", ov, ok)
	}
}

func TestStoreConsumeOverrideModes(t *testing.T) {
	s := NewStore()
	key := RouteKey{Method: "GET", Path: "/api/users"}

	s.SetOverride(key, Override{CaseName: "empty", Mode: "next"})
	if ov, ok := s.ConsumeOverride(key); !ok || ov.CaseName != "empty" {
		t.Fatalf("next override first consume = %#v ok=%v", ov, ok)
	}
	if _, ok := s.ConsumeOverride(key); ok {
		t.Fatal("next override should be removed after one consume")
	}

	s.SetOverride(key, Override{CaseName: "error", Mode: "count", Remaining: 2})
	for i := 0; i < 2; i++ {
		if ov, ok := s.ConsumeOverride(key); !ok || ov.CaseName != "error" {
			t.Fatalf("count override consume %d = %#v ok=%v", i, ov, ok)
		}
	}
	if _, ok := s.ConsumeOverride(key); ok {
		t.Fatal("count override should be removed after remaining reaches zero")
	}

	s.SetOverride(key, Override{CaseName: "slow", Mode: "always"})
	for i := 0; i < 3; i++ {
		if ov, ok := s.ConsumeOverride(key); !ok || ov.CaseName != "slow" {
			t.Fatalf("always override consume %d = %#v ok=%v", i, ov, ok)
		}
	}
}
