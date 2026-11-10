package cli_test

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

const pinned = "-y skills@1.7.4 add https://github.com/mattpocock/skills/tree/5beb35b040e3de97e54fa9e386d7e42d15c295c2 " +
	"-s grill-me grill-with-docs grilling domain-modeling"

var externalSkills = []string{"grill-me", "grill-with-docs", "grilling", "domain-modeling"}

func argsCalls(log []string) []string {
	var calls []string
	for _, l := range log {
		if strings.HasPrefix(l, "ARGS") {
			calls = append(calls, l)
		}
	}
	return calls
}

func TestExternalSkillsInstalledPinnedWithTelemetryOff(t *testing.T) {
	npx := fakeNpx(t)
	d := scratch(t)
	r := run(t, "init", d, "-y", "--stack", "nextjs", "--agents", "claude-code,hermes-agent")
	mustOK(t, r)
	want := []string{"ARGS " + pinned + " -a claude-code hermes-agent -y", "ENV DISABLE_TELEMETRY=1 DO_NOT_TRACK=1"}
	if got := npx(); !reflect.DeepEqual(got, want) {
		t.Fatalf("%q", got)
	}
	for _, s := range externalSkills {
		if !exists(d, ".agents/skills/"+s+"/SKILL.md") {
			t.Fatal(s)
		}
	}
	match(t, r.stdout, `(?m)^\+ skills externas`)
}

func TestExternalSkillsNotReinstalled(t *testing.T) {
	npx := fakeNpx(t)
	d := scratch(t)
	mustOK(t, run(t, "init", d, "-y", "--stack", "nextjs"))
	r := run(t, "init", d, "-y")
	mustOK(t, r)
	if n := len(argsCalls(npx())); n != 1 {
		t.Fatalf("%d npx calls", n)
	}
	match(t, r.stdout, `(?m)^= skills externas`)
}

func TestExternalSkillsFailureDoesNotAbort(t *testing.T) {
	fakeNpx(t)
	t.Setenv("FAKE_NPX_FAIL", "1")
	d := scratch(t)
	r := run(t, "init", d, "-y", "--stack", "nextjs")
	mustOK(t, r)
	match(t, r.stdout, `(?m)^! skills externas.*npx -y skills@1\.7\.4 add`)
	if !exists(d, ".agents/manifest.json") {
		t.Fatal("manifest missing")
	}
}

func TestExternalSkillsDryRunRunsNoNpx(t *testing.T) {
	npx := fakeNpx(t)
	d := scratch(t)
	r := run(t, "init", d, "-y", "--stack", "nextjs", "--dry-run")
	mustOK(t, r)
	if got := npx(); got != nil {
		t.Fatal(got)
	}
	match(t, r.stdout, `ejecutaría: npx -y skills@1\.7\.4`)
}

func TestNoExternalSkillsFlagSkipsInstall(t *testing.T) {
	npx := fakeNpx(t)
	mustOK(t, initq(t, scratch(t), "--stack", "nextjs"))
	if got := npx(); got != nil {
		t.Fatal(got)
	}
}

func TestExternalSkillsSkippedWhenSkillsDirEscapesBySymlink(t *testing.T) {
	skipOnWindows(t, "symlinks need privileges on Windows")
	npx := fakeNpx(t)
	d := scratch(t)
	outsideDir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(d, ".agents"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outsideDir, filepath.Join(d, ".agents", "skills")); err != nil {
		t.Fatal(err)
	}
	r := run(t, "init", d, "-y", "--stack", "nextjs", "--agents", "claude-code")
	mustOK(t, r)
	if calls := argsCalls(npx()); calls != nil {
		t.Fatalf("npx ran: %q", calls)
	}
	match(t, r.stdout, `(?m)^! skills externas.*no se instalan: \.agents/skills está fuera del proyecto por un enlace simbólico`)
	if got := entries(t, outsideDir); len(got) != 0 {
		t.Fatalf("wrote outside the project: %q", got)
	}
}

// linkOutside makes rel inside dir a symlink to a fresh directory outside it and returns that directory.
func linkOutside(t *testing.T, dir, rel string) string {
	t.Helper()
	outsideDir := t.TempDir()
	if err := os.MkdirAll(filepath.Dir(filepath.Join(dir, rel)), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outsideDir, filepath.Join(dir, rel)); err != nil {
		t.Fatal(err)
	}
	return outsideDir
}

func TestExternalSkillsSkippedWhenAgentSkillsDirEscapes(t *testing.T) {
	skipOnWindows(t, "symlinks need privileges on Windows")
	npx := fakeNpx(t)
	d := scratch(t)
	outsideDir := linkOutside(t, d, ".claude/skills")
	r := run(t, "init", d, "-y", "--stack", "nextjs", "--agents", "claude-code")
	mustOK(t, r)
	if calls := argsCalls(npx()); calls != nil {
		t.Fatalf("npx ran: %q", calls)
	}
	match(t, r.stdout, `(?m)^! skills externas.*no se instalan: \.claude/skills está fuera`)
	if got := entries(t, outsideDir); len(got) != 0 {
		t.Fatalf("wrote outside the project: %q", got)
	}
}

func TestExternalSkillsIgnoreEscapingDirOfUnchosenAgent(t *testing.T) {
	skipOnWindows(t, "symlinks need privileges on Windows")
	npx := fakeNpx(t)
	d := scratch(t)
	linkOutside(t, d, ".claude/skills")
	r := run(t, "init", d, "-y", "--stack", "nextjs", "--agents", "opencode")
	mustOK(t, r)
	if n := len(argsCalls(npx())); n != 1 {
		t.Fatalf("%d npx calls, want 1", n)
	}
	noMatch(t, r.stdout, `no se instalan`)
}

func TestSyncSkipsExternalSkillsWhenSkillsDirEscapes(t *testing.T) {
	skipOnWindows(t, "symlinks need privileges on Windows")
	fakeNpx(t)
	d := scratch(t)
	mustOK(t, run(t, "init", d, "-y", "--stack", "nextjs", "--agents", "claude-code"))
	if err := os.RemoveAll(filepath.Join(d, ".agents", "skills")); err != nil {
		t.Fatal(err)
	}
	outsideDir := linkOutside(t, d, ".agents/skills")
	npx := fakeNpx(t)
	r := run(t, "sync", d, "-y")
	mustOK(t, r)
	if calls := argsCalls(npx()); calls != nil {
		t.Fatalf("npx ran: %q", calls)
	}
	match(t, r.stdout, `(?m)^! skills externas.*no se instalan: \.agents/skills está fuera`)
	if got := entries(t, outsideDir); len(got) != 0 {
		t.Fatalf("wrote outside the project: %q", got)
	}
}

func TestGlobalExternalSkillsFollowLinkedHomeSkillsDir(t *testing.T) {
	skipOnWindows(t, "symlinks need privileges on Windows")
	npx := fakeNpx(t)
	home := fakeHome(t)
	linkOutside(t, home, ".agents/skills")
	r := run(t, "init", "--global", "-y", "--agents", "claude-code")
	mustOK(t, r)
	noMatch(t, r.stdout, `no se instalan`)
	if n := len(argsCalls(npx())); n != 1 {
		t.Fatalf("%d npx calls, want 1: a home skills dir linked to dotfiles is a normal setup", n)
	}
}
