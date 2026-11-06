package cli_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
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

// fakeNpx puts a fake `npx` first in PATH: this very test binary, copied as npx
// (npx.exe on Windows). Started under that name, TestMain runs fakeNpxMain instead
// of the tests. It logs its arguments and telemetry variables, then creates
// `.agents/skills/<s>/SKILL.md` for every `-s` value, like `skills add` does.
// FAKE_NPX_FAIL=1 makes it fail without installing anything.
func fakeNpx(t *testing.T) func() []string {
	t.Helper()
	bin := t.TempDir()
	logFile := filepath.Join(bin, "calls.log")
	self, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(self)
	if err != nil {
		t.Fatal(err)
	}
	name := "npx"
	if runtime.GOOS == "windows" {
		name = "npx.exe"
	}
	if err := os.WriteFile(filepath.Join(bin, name), data, 0o755); err != nil {
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

func TestMain(m *testing.M) {
	if strings.HasPrefix(filepath.Base(os.Args[0]), "npx") {
		os.Exit(fakeNpxMain(os.Args[1:]))
	}
	os.Exit(m.Run())
}

func fakeNpxMain(args []string) int {
	f, err := os.OpenFile(os.Getenv("FAKE_NPX_LOG"), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return 2
	}
	defer f.Close()
	fmt.Fprintf(f, "ARGS %s\n", strings.Join(args, " "))
	fmt.Fprintf(f, "ENV DISABLE_TELEMETRY=%s DO_NOT_TRACK=%s\n", os.Getenv("DISABLE_TELEMETRY"), os.Getenv("DO_NOT_TRACK"))
	if os.Getenv("FAKE_NPX_FAIL") == "1" {
		return 1
	}
	collect := false
	for _, a := range args {
		switch {
		case a == "-s":
			collect = true
		case strings.HasPrefix(a, "-"):
			collect = false
		case collect:
			dir := filepath.Join(".agents", "skills", a)
			if os.MkdirAll(dir, 0o755) != nil || os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte("---\nname: "+a+"\n---\n"), 0o644) != nil {
				return 1
			}
		}
	}
	if os.WriteFile("skills-lock.json", []byte("{}\n"), 0o644) != nil {
		return 1
	}
	return 0
}

// skipOnWindows skips what needs Unix: make, bash scripts, permission bits or
// creating symlinks (which Windows only allows with extra privileges).
func skipOnWindows(t *testing.T, why string) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("Windows: " + why)
	}
}

func readJSON(t *testing.T, dir, rel string, v any) {
	t.Helper()
	if err := json.Unmarshal([]byte(read(t, dir, rel)), v); err != nil {
		t.Fatal(err)
	}
}

func regexpQuote(s string) string { return regexp.QuoteMeta(s) }
