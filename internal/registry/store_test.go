package registry

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func tmpRegFile(t *testing.T) string {
	t.Helper()
	d := t.TempDir()
	return filepath.Join(d, "projects.json")
}

func TestLoad_NotExist_ReturnsEmptyRegistry(t *testing.T) {
	reg, err := Load(tmpRegFile(t))
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if reg == nil {
		t.Fatal("expected non-nil registry")
	}
	if reg.Version != 1 {
		t.Errorf("Version=%d, want 1", reg.Version)
	}
	if len(reg.Projects) != 0 {
		t.Errorf("Projects len=%d, want 0", len(reg.Projects))
	}
}

func TestSave_Then_Load_RoundTrip(t *testing.T) {
	p := tmpRegFile(t)
	reg := &Registry{
		Version:      1,
		LastActiveId: "abc123",
		Projects: []Project{{
			ID:         "abc123",
			Name:       "my-app",
			ConfigPath: "/abs/cfg.json5",
			CWD:        "/abs",
			Envs:       []string{"dev", "staging"},
			LastEnv:    "dev",
			LastPort:   5090,
			LastRunAt:  time.Date(2026, 5, 20, 10, 0, 0, 0, time.UTC),
			PIDFile:    "/abs/.fakeserver/run.pid",
		}},
	}
	if err := Save(p, reg); err != nil {
		t.Fatalf("Save: %v", err)
	}
	got, err := Load(p)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got.LastActiveId != "abc123" {
		t.Errorf("LastActiveId=%q", got.LastActiveId)
	}
	if len(got.Projects) != 1 || got.Projects[0].Name != "my-app" {
		t.Errorf("Projects mismatch: %+v", got.Projects)
	}
}

func TestLoad_Corrupt_RenamesToBakAndReturnsEmpty(t *testing.T) {
	p := tmpRegFile(t)
	if err := os.WriteFile(p, []byte("{not valid json"), 0600); err != nil {
		t.Fatal(err)
	}
	reg, err := Load(p)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(reg.Projects) != 0 {
		t.Errorf("expected empty reg after corrupt; got %d", len(reg.Projects))
	}
	dir := filepath.Dir(p)
	entries, _ := os.ReadDir(dir)
	var hasBak bool
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), filepath.Base(p)+".bak-") {
			hasBak = true
			break
		}
	}
	if !hasBak {
		t.Error("expected projects.json.bak-<ts> after corrupt; not found")
	}
}

func TestProjectID_IsDeterministic_AndShort(t *testing.T) {
	id1 := ProjectID("/abs/a/b/cfg.json5")
	id2 := ProjectID("/abs/a/b/cfg.json5")
	id3 := ProjectID("/abs/a/c/cfg.json5")
	if id1 != id2 {
		t.Errorf("ID not deterministic: %s vs %s", id1, id2)
	}
	if id1 == id3 {
		t.Errorf("different paths collide: %s == %s", id1, id3)
	}
	if len(id1) != 12 {
		t.Errorf("ID length %d, want 12", len(id1))
	}
}

func TestUpsert_AddsNewAndReplacesExisting(t *testing.T) {
	reg := emptyRegistry()
	Upsert(reg, Project{ID: "a", Name: "Alpha"})
	Upsert(reg, Project{ID: "b", Name: "Beta"})
	if len(reg.Projects) != 2 {
		t.Fatalf("len=%d, want 2", len(reg.Projects))
	}
	Upsert(reg, Project{ID: "a", Name: "Alpha-v2"})
	if len(reg.Projects) != 2 {
		t.Fatalf("len=%d, want 2 after upsert-existing", len(reg.Projects))
	}
	got, ok := Get(reg, "a")
	if !ok || got.Name != "Alpha-v2" {
		t.Errorf("after upsert: %+v ok=%v", got, ok)
	}
	if reg.LastActiveId != "a" {
		t.Errorf("LastActiveId=%q, want a", reg.LastActiveId)
	}
}

func TestRemove_ClearsLastActiveWhenMatched(t *testing.T) {
	reg := emptyRegistry()
	Upsert(reg, Project{ID: "a"})
	Upsert(reg, Project{ID: "b"})
	reg.LastActiveId = "a"
	Remove(reg, "a")
	if _, ok := Get(reg, "a"); ok {
		t.Error("a should be removed")
	}
	if reg.LastActiveId != "" {
		t.Errorf("LastActiveId not cleared: %q", reg.LastActiveId)
	}
	Remove(reg, "nonexistent") // 不应 panic
}
