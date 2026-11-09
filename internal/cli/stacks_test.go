package cli_test

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

func TestStacksListsStacksWithLanguageAndAgents(t *testing.T) {
	r := run(t, "stacks")
	mustOK(t, r)
	width := 0
	for _, s := range allStacks {
		width = max(width, len(s.ID))
	}
	for _, s := range allStacks {
		// The name starts with the language: "<language> · <framework> …".
		match(t, r.stdout, `(?m)^  `+regexp.QuoteMeta(fmt.Sprintf("%-*s  %s (puerto %d)", width, s.ID, s.Name, s.Port))+`$`)
	}
	for _, lang := range []string{"TypeScript", "Python", "PHP", "Java", "Go"} {
		match(t, r.stdout, `\b`+lang+` · `)
	}
	for _, id := range []string{"claude-code", "opencode", "codex", "cursor", "gemini-cli", "github-copilot", "hermes-agent"} {
		match(t, r.stdout, `\b`+id+`\b`)
	}
}

// The help names the stack question's steps and what a frontend stack does not accept.
func TestHelpDescribesStackQuestionAndFrontendLimits(t *testing.T) {
	r := run(t, "help")
	mustOK(t, r)
	match(t, r.stdout, `init pregunta lo que falte en este orden: tipo de app, lenguaje, framework,`)
	match(t, r.stdout, "--stack <id>            Stack \\(ver `basekit stacks`; por defecto: detectado; si no, se pregunta\n +tipo de app → lenguaje → framework\\)")
	match(t, r.stdout, `--db <motor>            auto \| postgres \| mysql \| sqlite \| none \(por defecto: auto;\n +un stack frontend solo admite auto o none\)`)
	match(t, r.stdout, `--redis                 Agrega Redis \(no en un stack frontend\)`)
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

// detectIn runs `init --yes` on a folder holding files (path -> content) and returns the result and the stack written.
func detectIn(t *testing.T, files map[string]string) (result, string) {
	t.Helper()
	d := scratch(t)
	for f, text := range files {
		write(t, d, f, text)
	}
	r := initq(t, d)
	if r.code != 0 {
		return r, ""
	}
	return r, manifest(t, d)["stack"].(string)
}

const (
	springPom = "<project><parent><artifactId>spring-boot-starter-parent</artifactId></parent></project>\n"
	viteReact = `{"dependencies":{"react":"19"},"devDependencies":{"@vitejs/plugin-react":"5","vite":"7"}}`
)

func pkgJSON(deps string) string { return `{"name":"app","dependencies":{` + deps + `}}` }

// S7: existing projects are detected as before, and the new stacks never take over another stack's project.
func TestDetectionWithEveryStack(t *testing.T) {
	cases := []struct {
		title string
		files map[string]string
		want  string
	}{
		{"nest-cli.json wins over the Express inside NestJS", map[string]string{"nest-cli.json": "{}\n", "package.json": pkgJSON(`"@nestjs/core":"11","@nestjs/platform-express":"11","express":"5"`)}, "nestjs"},
		{"a Next.js project that also uses Express and React stays nextjs", map[string]string{"next.config.ts": "export default {};\n", "package.json": pkgJSON(`"next":"16","react":"19","express":"5"`)}, "nextjs"},
		{"a Vite React project is react", map[string]string{"vite.config.ts": "", "package.json": viteReact}, "react"},
		{"a Vite Vue project is vue", map[string]string{"vite.config.ts": "", "package.json": `{"dependencies":{"vue":"3"},"devDependencies":{"@vitejs/plugin-vue":"6","vite":"7"}}`}, "vue"},
		{"manage.py is django, never django-htmx", map[string]string{"manage.py": "", "pyproject.toml": "[project]\ndependencies = [\"django\", \"django-htmx\"]\n"}, "django"},
		{"a FastAPI pyproject is fastapi", map[string]string{"pyproject.toml": "[project]\ndependencies = [\"fastapi>=0.115\"]\n"}, "fastapi"},
		{"a Flask pyproject is flask", map[string]string{"pyproject.toml": "[project]\ndependencies = [\"flask>=3\"]\n"}, "flask"},
		{"artisan is laravel", map[string]string{"artisan": "", "composer.json": `{"require":{"laravel/framework":"^12"}}` + "\n"}, "laravel"},
		{"symfony.lock is symfony", map[string]string{"symfony.lock": "{}\n", "composer.json": `{"require":{"symfony/framework-bundle":"7.*"}}` + "\n"}, "symfony"},
		{"a Gin go.mod is gin", map[string]string{"go.mod": "module app\n\ngo 1.27\n\nrequire github.com/gin-gonic/gin v1.11.0\n"}, "gin"},
		{"a Spring Boot pom.xml is spring", map[string]string{"pom.xml": springPom}, "spring"},
		// detect.priority: the stronger signal wins whatever the ids (a project with the markers of two stacks).
		{"a Spring pom.xml plus a Vite React package.json is spring", map[string]string{"pom.xml": springPom, "package.json": viteReact}, "spring"},
		{"next.config.ts plus nest-cli.json is nextjs", map[string]string{"next.config.ts": "export default {};\n", "nest-cli.json": "{}\n"}, "nextjs"},
		{"manage.py plus a FastAPI pyproject is fastapi", map[string]string{"manage.py": "", "pyproject.toml": "[project]\ndependencies = [\"fastapi>=0.115\"]\n"}, "fastapi"},
		{"artisan plus a Gin go.mod is laravel", map[string]string{"artisan": "", "go.mod": "module app\n\ngo 1.27\n\nrequire github.com/gin-gonic/gin v1.11.0\n"}, "laravel"},
		{"symfony.lock plus a Vite React package.json is symfony", map[string]string{"symfony.lock": "{}\n", "package.json": viteReact}, "symfony"},
	}
	for _, c := range cases {
		t.Run(c.title, func(t *testing.T) {
			r, got := detectIn(t, c.files)
			if got != c.want {
				t.Fatalf("detected %q, want %q\n%s", got, c.want, r.stderr)
			}
		})
	}

	undetected := []struct {
		title string
		files map[string]string
	}{
		{"a package.json with express is never express", map[string]string{"package.json": pkgJSON(`"express":"5"`)}},
		{"a package.json with only react (no @vitejs/plugin-react) is not react", map[string]string{"package.json": pkgJSON(`"react":"19"`)}},
		{"a plain go.mod is not go-native", map[string]string{"go.mod": "module app\n\ngo 1.27\n"}},
		{"a plain pyproject is not python-native", map[string]string{"pyproject.toml": "[project]\nname = \"app\"\n"}},
		{"a plain composer.json is not php-native", map[string]string{"composer.json": `{"require":{"php":">=8.5"}}` + "\n"}},
		{"a vite.config.ts alone is not vanilla-ts", map[string]string{"vite.config.ts": "", "package.json": `{"devDependencies":{"vite":"7","typescript":"5"}}`}},
	}
	for _, c := range undetected {
		t.Run(c.title, func(t *testing.T) {
			r, _ := detectIn(t, c.files)
			if r.code != 2 {
				t.Fatalf("exit %d", r.code)
			}
			match(t, r.stderr, `No se pudo detectar el stack\. Usa --stack <id>`)
		})
	}
}

func TestNativeStacksExpressAndDjangoHtmxHaveNoDetectRule(t *testing.T) {
	var none []string
	for _, s := range allStacks {
		if len(s.Detect.Files) == 0 && s.Detect.Manifest == "" && s.Detect.Contains == "" {
			none = append(none, s.ID)
		}
	}
	want := []string{"django-htmx", "express", "go-native", "node-native", "php-native", "python-native", "vanilla-ts"}
	if strings.Join(none, ",") != strings.Join(want, ",") {
		t.Fatalf("stacks without detect: %v, want %v", none, want)
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
			// Whole words, like Node's architecture test: "gin" inside "plugin" is not a stack id.
			if regexp.MustCompile(`\b` + regexp.QuoteMeta(id) + `\b`).Match(b) {
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
