package cli_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestStacksListsStacksWithLanguageAndAgents(t *testing.T) {
	r := run(t, "stacks")
	mustOK(t, r)
	for _, id := range stackIDs {
		match(t, r.stdout, `\b`+id+`\b`)
	}
	for _, lang := range []string{"TypeScript", "Python", "PHP", "Java"} {
		match(t, r.stdout, `\b`+lang+`\b`)
	}
	for _, id := range []string{"claude-code", "opencode", "codex", "cursor", "gemini-cli", "github-copilot", "hermes-agent"} {
		match(t, r.stdout, `\b`+id+`\b`)
	}
}

func TestStacksRejectsExtraArguments(t *testing.T) {
	r := run(t, "stacks", "x")
	if r.code != 2 {
		t.Fatalf("exit %d", r.code)
	}
	match(t, r.stderr, `Argumentos de más`)
}

func TestDetectsStackFromDetectFiles(t *testing.T) {
	d := scratch(t)
	write(t, d, "next.config.ts", "export default {};\n")
	mustOK(t, initq(t, d))
	if manifest(t, d)["stack"] != "nextjs" {
		t.Fatal(manifest(t, d)["stack"])
	}
}

func TestDetectsStackFromManifestContains(t *testing.T) {
	d := scratch(t)
	write(t, d, "pyproject.toml", "[project]\ndependencies = [\"fastapi>=0.115\"]\n")
	mustOK(t, initq(t, d))
	if manifest(t, d)["stack"] != "fastapi" {
		t.Fatal(manifest(t, d)["stack"])
	}
}

func TestFailsWhenStackCannotBeDetectedNonInteractive(t *testing.T) {
	d := scratch(t)
	r := initq(t, d)
	if r.code == 0 {
		t.Fatal("expected failure")
	}
	match(t, r.stderr, `--stack`)
	if len(entries(t, d)) != 0 {
		t.Fatalf("wrote %v", entries(t, d))
	}
}

func TestSourceHasNoStackIDs(t *testing.T) {
	files := []string{"../../cmd/basekit/main.go", "../../plantillas/embed.go"}
	err := filepath.WalkDir("..", func(p string, d os.DirEntry, err error) error {
		if err == nil && !d.IsDir() && strings.HasSuffix(p, ".go") && !strings.HasSuffix(p, "_test.go") {
			files = append(files, p)
		}
		return err
	})
	if err != nil || len(files) < 10 {
		t.Fatalf("walk: %v (%d files)", err, len(files))
	}
	for _, f := range files {
		b, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		for _, id := range stackIDs {
			if strings.Contains(string(b), id) {
				t.Errorf("%s mentions %q", f, id)
			}
		}
	}
}

// Skill frontmatter must be valid YAML: a plain description with ": " is a parse
// error for strict loaders, so such a value has to be quoted.
func TestSkillDescriptionsAreValidYAML(t *testing.T) {
	files, _ := filepath.Glob("../../plantillas/skills/*/SKILL.md")
	more, _ := filepath.Glob("../../plantillas/stacks/*/skills/*/SKILL.md")
	files = append(files, more...)
	if len(files) < 10 {
		t.Fatalf("only %d skills found", len(files))
	}
	for _, f := range files {
		b, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		for _, line := range strings.Split(string(b), "\n") {
			v, ok := strings.CutPrefix(line, "description: ")
			if !ok {
				continue
			}
			if strings.Contains(v, ": ") && !strings.HasPrefix(v, `"`) && !strings.HasPrefix(v, "'") {
				t.Errorf("%s: unquoted description with \": \"", f)
			}
			break
		}
	}
}
