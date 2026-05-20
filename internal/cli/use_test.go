package cli

import (
	"bytes"
	"path/filepath"
	"strings"
	"testing"

	"github.com/inhere/fakeserver/internal/registry"
)

func newRegFor(t *testing.T, projects []registry.Project) string {
	t.Helper()
	d := t.TempDir()
	regPath := filepath.Join(d, "projects.json")
	reg := &registry.Registry{Version: 1, Projects: projects}
	if err := registry.Save(regPath, reg); err != nil {
		t.Fatal(err)
	}
	return regPath
}

func TestResolveProjectID_ExactMatch(t *testing.T) {
	reg := &registry.Registry{Projects: []registry.Project{
		{ID: "a1b2c3d4e5f6"}, {ID: "f7e6d5c4b3a2"},
	}}
	got, err := resolveProjectID(reg, "a1b2c3d4e5f6")
	if err != nil || got != "a1b2c3d4e5f6" {
		t.Errorf("got %q err=%v", got, err)
	}
}

func TestResolveProjectID_UniquePrefix(t *testing.T) {
	reg := &registry.Registry{Projects: []registry.Project{
		{ID: "a1b2c3d4e5f6"}, {ID: "f7e6d5c4b3a2"},
	}}
	got, err := resolveProjectID(reg, "a1b2")
	if err != nil || got != "a1b2c3d4e5f6" {
		t.Errorf("got %q err=%v", got, err)
	}
}

func TestResolveProjectID_AmbiguousPrefix_Errors(t *testing.T) {
	reg := &registry.Registry{Projects: []registry.Project{
		{ID: "abc111"}, {ID: "abc222"},
	}}
	_, err := resolveProjectID(reg, "abc")
	if err == nil || !strings.Contains(err.Error(), "ambiguous") {
		t.Errorf("expected ambiguous error; got %v", err)
	}
}

func TestResolveProjectID_NoMatch_Errors(t *testing.T) {
	reg := &registry.Registry{Projects: []registry.Project{
		{ID: "a1b2c3d4e5f6"},
	}}
	_, err := resolveProjectID(reg, "ZZ")
	if err == nil || !strings.Contains(err.Error(), "no project matches") {
		t.Errorf("expected no-match error; got %v", err)
	}
}

func TestUse_UpdatesLastActiveId(t *testing.T) {
	regPath := newRegFor(t, []registry.Project{
		{ID: "alpha-id"}, {ID: "beta-id"},
	})
	var buf bytes.Buffer
	err := runUse(useOptions{regPath: regPath, out: &buf, id: "beta-id"})
	if err != nil {
		t.Fatal(err)
	}
	reg, _ := registry.Load(regPath)
	if reg.LastActiveId != "beta-id" {
		t.Errorf("LastActiveId=%q, want beta-id", reg.LastActiveId)
	}
	if !strings.Contains(buf.String(), "beta-id") {
		t.Errorf("output should mention activated id; got %q", buf.String())
	}
}

func TestUse_EmptyRegistry_Errors(t *testing.T) {
	d := t.TempDir()
	regPath := filepath.Join(d, "projects.json")
	var buf bytes.Buffer
	err := runUse(useOptions{regPath: regPath, out: &buf, id: "any"})
	if err == nil {
		t.Error("expected error on empty registry")
	}
}

func TestUse_MissingIdArg_Errors(t *testing.T) {
	var buf bytes.Buffer
	err := runUse(useOptions{regPath: "/no", out: &buf, id: "   "})
	if err == nil {
		t.Error("expected error when id is whitespace")
	}
}
