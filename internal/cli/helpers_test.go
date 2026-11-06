package cli_test

import (
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/F4R1DS4NCH3Z26/basekit-go/internal/cli"
)

var stackIDs = []string{"fastapi", "laravel", "nextjs", "spring"}

type result struct {
	code   int
	stdout string
	stderr string
}

// run executes the CLI in-process as a non-interactive session (like a pipe or CI).
func run(t *testing.T, args ...string) result {
	t.Helper()
	return runWith(t, "", false, args...)
}

// runTTY executes the CLI as an interactive session answering with stdin.
func runTTY(t *testing.T, stdin string, args ...string) result {
	t.Helper()
	return runWith(t, stdin, true, args...)
}

func runWith(t *testing.T, stdin string, interactive bool, args ...string) result {
	t.Helper()
	var out, errb bytes.Buffer
	code := cli.Main(args, strings.NewReader(stdin), &out, &errb, interactive)
	return result{code, out.String(), errb.String()}
}

// initq runs a non-interactive init without external skills (no network).
func initq(t *testing.T, dir string, extra ...string) result {
	t.Helper()
	return run(t, append([]string{"init", dir, "--yes", "--no-external-skills"}, extra...)...)
}

func mustOK(t *testing.T, r result) {
	t.Helper()
	if r.code != 0 {
		t.Fatalf("exit %d\nstdout:\n%s\nstderr:\n%s", r.code, r.stdout, r.stderr)
	}
}

// scratch is a directory outside the worktree, removed by the test framework.
func scratch(t *testing.T) string {
	t.Helper()
	return t.TempDir()
}

func sh(t *testing.T, dir, name string, args ...string) result {
	t.Helper()
	cmd := exec.Command(name, args...)
	cmd.Dir = dir
	var out, errb bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &errb
	code := 0
	if err := cmd.Run(); err != nil {
		code = -1
		if ee, ok := err.(*exec.ExitError); ok {
			code = ee.ExitCode()
		}
	}
	return result{code, out.String(), errb.String()}
}

func read(t *testing.T, dir, rel string) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(dir, rel))
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func write(t *testing.T, dir, rel, content string) {
	t.Helper()
	p := filepath.Join(dir, rel)
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func exists(dir, rel string) bool {
	_, err := os.Stat(filepath.Join(dir, rel))
	return err == nil
}

func entries(t *testing.T, dir string) []string {
	t.Helper()
	es, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	names := []string{}
	for _, e := range es {
		names = append(names, e.Name())
	}
	return names
}

func manifest(t *testing.T, dir string) map[string]any {
	t.Helper()
	var m map[string]any
	if err := json.Unmarshal([]byte(read(t, dir, ".agents/manifest.json")), &m); err != nil {
		t.Fatal(err)
	}
	return m
}

func gitCommitAll(t *testing.T, dir string) {
	t.Helper()
	sh(t, dir, "git", "add", "-A")
	if r := sh(t, dir, "git", "-c", "user.name=t", "-c", "user.email=t@example.com", "commit", "-qm", "init"); r.code != 0 {
		t.Fatalf("git commit failed: %s", r.stderr)
	}
}

func gitClean(t *testing.T, dir string) {
	t.Helper()
	if s := sh(t, dir, "git", "status", "--porcelain").stdout; s != "" {
		t.Fatalf("git status not empty:\n%s", s)
	}
}

// actionLines returns the report lines whose mark is in marks (e.g. "+~!").
func actionLines(out, marks string) []string {
	re := regexp.MustCompile(`^[` + regexp.QuoteMeta(marks) + `] `)
	var lines []string
	for _, l := range strings.Split(out, "\n") {
		if re.MatchString(l) {
			lines = append(lines, l)
		}
	}
	return lines
}

func lineFor(out string, label *regexp.Regexp) string {
	for _, l := range strings.Split(out, "\n") {
		if label.MatchString(l) {
			return l
		}
	}
	return ""
}

func match(t *testing.T, text, pattern string) {
	t.Helper()
	if !regexp.MustCompile(pattern).MatchString(text) {
		t.Fatalf("expected /%s/ in:\n%s", pattern, text)
	}
}

func noMatch(t *testing.T, text, pattern string) {
	t.Helper()
	if regexp.MustCompile(pattern).MatchString(text) {
		t.Fatalf("unexpected /%s/ in:\n%s", pattern, text)
	}
}

// fakeNpx puts a fake `npx` first in PATH. It logs its arguments and telemetry
// variables, then creates `.agents/skills/<s>/SKILL.md` for every `-s` value, like
// `skills add` does. FAKE_NPX_FAIL=1 makes it fail without installing anything.
func fakeNpx(t *testing.T) func() []string {
	t.Helper()
	bin := t.TempDir()
	logFile := filepath.Join(bin, "calls.log")
	script := `#!/bin/sh
printf 'ARGS %s\n' "$*" >> "$FAKE_NPX_LOG"
printf 'ENV DISABLE_TELEMETRY=%s DO_NOT_TRACK=%s\n' "$DISABLE_TELEMETRY" "$DO_NOT_TRACK" >> "$FAKE_NPX_LOG"
[ "${FAKE_NPX_FAIL:-0}" = 1 ] && exit 1
collect=0
for a in "$@"; do
  case "$a" in -s) collect=1; continue ;; -*) collect=0; continue ;; esac
  if [ "$collect" = 1 ]; then mkdir -p ".agents/skills/$a" && printf -- '---\nname: %s\n---\n' "$a" > ".agents/skills/$a/SKILL.md"; fi
done
echo '{}' > skills-lock.json
`
	if err := os.WriteFile(filepath.Join(bin, "npx"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("FAKE_NPX_LOG", logFile)
	return func() []string {
		b, err := os.ReadFile(logFile)
		if err != nil {
			return nil
		}
		return strings.Split(strings.TrimRight(string(b), "\n"), "\n")
	}
}

func readJSON(t *testing.T, dir, rel string, v any) {
	t.Helper()
	if err := json.Unmarshal([]byte(read(t, dir, rel)), v); err != nil {
		t.Fatal(err)
	}
}

func regexpQuote(s string) string { return regexp.QuoteMeta(s) }
