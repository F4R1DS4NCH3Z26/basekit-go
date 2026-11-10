package stack

import (
	"encoding/json"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"testing/fstest"
)

// expectedKind is the kind of every stack, as the user classified them (S1b).
var expectedKind = map[string]string{
	"fastapi": "backend", "django": "backend", "flask": "backend", "python-native": "backend",
	"nestjs": "backend", "express": "backend", "node-native": "backend",
	"symfony": "backend", "php-native": "backend", "spring": "backend", "gin": "backend", "go-native": "backend",
	"react": "frontend", "vue": "frontend", "vanilla-ts": "frontend",
	"nextjs": "fullstack", "laravel": "fullstack", "django-htmx": "fullstack",
}

func TestEveryStackDeclaresKindLanguageAndHealth(t *testing.T) {
	stacks, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	var ids []string
	for _, s := range stacks {
		ids = append(ids, s.ID)
		if s.Kind != expectedKind[s.ID] {
			t.Errorf("%s: kind %q, want %q", s.ID, s.Kind, expectedKind[s.ID])
		}
		if !slices.Contains(Kinds, s.Kind) || s.Language == "" || !strings.HasPrefix(s.Health, "/") {
			t.Errorf("%s: kind %q language %q health %q", s.ID, s.Kind, s.Language, s.Health)
		}
		if !strings.HasPrefix(s.Name, s.Language+" · ") {
			t.Errorf("%s: name %q does not start with its language", s.ID, s.Name)
		}
	}
	if len(ids) != len(expectedKind) {
		t.Errorf("stacks %v, want the %d of expectedKind", ids, len(expectedKind))
	}
	if !slices.IsSorted(ids) {
		t.Errorf("not in id order (ties of detect.priority keep it): %v", ids)
	}
}

func TestOnlyFrontendStacksHaveNoDatabase(t *testing.T) {
	for kind, want := range map[string]bool{"backend": true, "fullstack": true, "frontend": false} {
		if got := HasDatabase(Stack{Kind: kind}); got != want {
			t.Errorf("HasDatabase(%s) = %v", kind, got)
		}
	}
}

// withStack swaps the templates for a folder holding one stack `x`: the real fastapi stack.json with patch applied
// (a nil value removes the key).
func withStack(t *testing.T, patch map[string]any) {
	t.Helper()
	var s map[string]any
	data, err := fs.ReadFile(templates, "stacks/fastapi/stack.json")
	if err != nil || json.Unmarshal(data, &s) != nil {
		t.Fatal(err)
	}
	s["id"] = "x"
	for k, v := range patch {
		if v == nil {
			delete(s, k)
		} else {
			s[k] = v
		}
	}
	out, _ := json.Marshal(s)
	saved := templates
	templates = fstest.MapFS{"stacks/x/stack.json": &fstest.MapFile{Data: out}}
	t.Cleanup(func() { templates = saved })
}

func TestLoadAcceptsAValidStack(t *testing.T) {
	withStack(t, map[string]any{})
	stacks, err := Load()
	if err != nil || len(stacks) != 1 || stacks[0].ID != "x" || stacks[0].Kind != "backend" || stacks[0].Health != "/health" {
		t.Fatalf("%v %v", stacks, err)
	}
}

func TestLoadRejectsAnInvalidStackJSON(t *testing.T) {
	for name, patch := range map[string]map[string]any{
		"kind mobile":       {"kind": "mobile"},
		"empty kind":        {"kind": ""},
		"no kind":           {"kind": nil},
		"empty language":    {"language": ""},
		"blank language":    {"language": "  "},
		"no language":       {"language": nil},
		"numeric language":  {"language": 3},
		"no health":         {"health": nil},
		"relative health":   {"health": "health"},
		"no port":           {"port": nil},
		"no detect":         {"detect": nil},
		"decimal priority":  {"detect": map[string]any{"files": []string{"x"}, "priority": 1.5}},
		"text priority":     {"detect": map[string]any{"files": []string{"x"}, "priority": "10"}},
		"skills row of one": {"skills": [][]string{{"a"}}},
	} {
		t.Run(name, func(t *testing.T) {
			withStack(t, patch)
			_, err := Load()
			if err == nil || err.Error() != "stack.json inválido: stacks/x/stack.json" {
				t.Fatalf("err = %v", err)
			}
		})
	}
}

func TestEmptyDetectNeverMatches(t *testing.T) {
	withStack(t, map[string]any{"detect": map[string]any{}})
	stacks, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	d := t.TempDir()
	for _, f := range []string{"pyproject.toml", "package.json", "go.mod", "composer.json", "manage.py"} {
		if err := os.WriteFile(filepath.Join(d, f), []byte("fastapi\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if s := Detect(stacks, d); s != nil {
		t.Fatalf("detected %s with an empty detect", s.ID)
	}
}

func TestDetectTriesPriorityThenID(t *testing.T) {
	d := t.TempDir()
	if err := os.WriteFile(filepath.Join(d, "marker"), nil, 0o644); err != nil {
		t.Fatal(err)
	}
	stack := func(id string, priority *int) Stack {
		s := Stack{ID: id}
		s.Detect = &DetectRule{Files: []string{"marker"}, Priority: priority}
		return s
	}
	ten, sixty := 10, 60
	for _, c := range []struct {
		stacks []Stack
		want   string
	}{
		{[]Stack{stack("a", nil), stack("b", &ten)}, "b"},   // lower priority first, whatever the ids
		{[]Stack{stack("a", &sixty), stack("b", nil)}, "b"}, // the default is 50
		{[]Stack{stack("a", &ten), stack("b", &ten)}, "a"},  // ties keep the id order
		{[]Stack{stack("b", nil), stack("a", nil)}, "a"},    // even when given out of order
	} {
		if got := Detect(c.stacks, d); got == nil || got.ID != c.want {
			t.Errorf("Detect(%v) = %v, want %s", c.stacks, got, c.want)
		}
	}
}
