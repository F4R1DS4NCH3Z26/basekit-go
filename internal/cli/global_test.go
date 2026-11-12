package cli_test

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

// fakeHome points HOME at a fresh directory, so global installs never touch the real one.
func fakeHome(t *testing.T) string {
	t.Helper()
	home := scratch(t)
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home) // os.UserHomeDir reads it on Windows
	return home
}

func TestGlobalInstallsKitSkillsWithLinksPerAgent(t *testing.T) {
	skipOnWindows(t, "crea symlinks")
	home := fakeHome(t)
	r := run(t, "init", "--global", "-y", "--no-external-skills", "--agents", "claude-code,opencode,codex,hermes-agent")
	mustOK(t, r)
	for _, s := range []string{"project-bootstrap", "infra-setup", "zone-check"} {
		if !exists(home, ".agents/skills/"+s+"/SKILL.md") {
			t.Fatalf("missing canonical skill %s", s)
		}
		for _, dir := range []string{".claude/skills", ".config/opencode/skills", ".hermes/skills"} {
			link := filepath.Join(home, dir, s)
			target, err := os.Readlink(link)
			if err != nil || filepath.IsAbs(target) {
				t.Fatalf("%s/%s must be a relative link: %q %v", dir, s, target, err)
			}
			a, _ := filepath.EvalSymlinks(link)
			b, _ := filepath.EvalSymlinks(filepath.Join(home, ".agents/skills", s))
			if a != b {
				t.Fatalf("%s -> %s", link, a)
			}
		}
	}
	info, _ := os.Stat(filepath.Join(home, ".agents/skills/zone-check/scripts/check_zones.sh"))
	if info.Mode().Perm() != 0o755 {
		t.Fatalf("check_zones.sh mode %o", info.Mode().Perm())
	}
	// Stack skills and project files never go global; codex reads ~/.agents/skills itself.
	for _, f := range []string{".agents/skills/nextjs-verify", "AGENTS.md", ".git", "Makefile", ".gitignore", "docs", ".agents/manifest.json", ".cursor"} {
		if exists(home, f) {
			t.Errorf("%s must not be created by --global", f)
		}
	}
	var m map[string]any
	readJSON(t, home, ".agents/basekit.json", &m)
	if m["scope"] != "global" || !reflect.DeepEqual(m["agents"], []any{"claude-code", "opencode", "codex", "hermes-agent"}) {
		t.Fatal(m)
	}
	if _, ok := m["stack"]; ok {
		t.Fatal("a global manifest has no stack")
	}
	match(t, r.stdout, `basekit init`)
}

func TestGlobalIsIdempotent(t *testing.T) {
	fakeHome(t)
	mustOK(t, run(t, "init", "--global", "-y", "--no-external-skills", "--agents", "claude-code"))
	r := run(t, "init", "--global", "-y", "--no-external-skills")
	mustOK(t, r)
	if changed := actionLines(r.stdout, "+~!"); len(changed) > 0 {
		t.Fatal(changed)
	}
	match(t, r.stdout, `al día`)
}

func TestGlobalKeepsLocallyModifiedSkillsUnlessForce(t *testing.T) {
	home := fakeHome(t)
	mustOK(t, run(t, "init", "--global", "-y", "--no-external-skills", "--agents", "claude-code"))
	skill := ".agents/skills/zone-check/SKILL.md"
	original := read(t, home, skill)
	write(t, home, skill, "mío\n")
	r := run(t, "init", "--global", "-y", "--no-external-skills")
	mustOK(t, r)
	if read(t, home, skill) != "mío\n" {
		t.Fatal("overwritten without --force")
	}
	mustOK(t, run(t, "init", "--global", "-y", "--no-external-skills", "--force"))
	if read(t, home, skill) != original {
		t.Fatal("not restored with --force")
	}
}

func TestGlobalExternalSkillsInstallWithGlobalFlag(t *testing.T) {
	npx := fakeNpx(t)
	fakeHome(t)
	r := run(t, "init", "--global", "-y", "--agents", "claude-code,opencode")
	mustOK(t, r)
	calls := argsCalls(npx())
	if len(calls) != 1 {
		t.Fatal(calls)
	}
	match(t, calls[0], `^ARGS `+strings.ReplaceAll(pinned, ".", `\.`)+` -a claude-code opencode -g -y$`)
	match(t, r.stdout, `(?m)^\+ skills externas`)
}

func TestGlobalDryRunWritesNothing(t *testing.T) {
	npx := fakeNpx(t)
	home := fakeHome(t)
	r := run(t, "init", "--global", "-y", "--dry-run", "--agents", "claude-code")
	mustOK(t, r)
	if got := entries(t, home); len(got) != 0 {
		t.Fatalf("wrote %v", got)
	}
	if npx() != nil {
		t.Fatal("npx ran in dry-run")
	}
	match(t, r.stdout, `(?m)^\+ \.agents/skills/zone-check/SKILL\.md`)
}

func TestGlobalRejectsProjectOnlyOptions(t *testing.T) {
	fakeHome(t)
	for _, args := range [][]string{
		{"--stack", "nextjs"}, {"--context", "x"}, {"--db", "postgres"}, {"--redis"}, {"--no-docker"}, {"--no-git"},
	} {
		r := run(t, append([]string{"init", "--global", "-y"}, args...)...)
		if r.code != 2 {
			t.Fatalf("%v: exit %d", args, r.code)
		}
		match(t, r.stderr, regexpQuote(args[0])+`.*--global`)
	}
	r := run(t, "init", scratch(t), "--global", "-y")
	if r.code != 2 {
		t.Fatalf("dir with --global: exit %d", r.code)
	}
	match(t, r.stderr, `--global`)
}

func TestGlobalWizardAsksOnlySkillsAndAgents(t *testing.T) {
	fakeNpx(t)
	home := fakeHome(t)
	r := runTTY(t, "ninguna\n2\n", "init", "--global")
	mustOK(t, r)
	assertOrder(t, r.stdout, "Skills", "¿Con qué herramientas vas a trabajar?")
	for _, h := range []string{askKind, askLanguage, askFramework, "¿Usar Docker?", "¿Usar Makefile?", "¿Integrar base de datos?", "git init", "De qué trata"} {
		if strings.Contains(r.stdout, h) {
			t.Fatalf("asked %q in global mode", h)
		}
	}
	if !exists(home, ".config/opencode/skills/zone-check") {
		t.Fatal("opencode link missing")
	}
}

func TestProjectInitStillDefault(t *testing.T) {
	home := fakeHome(t)
	d := scratch(t)
	mustOK(t, initq(t, d, "--stack", "nextjs"))
	if exists(home, ".agents") {
		t.Fatal("project init wrote into HOME")
	}
	if !exists(d, ".agents/manifest.json") {
		t.Fatal("project manifest missing")
	}
}

// A1: a dangling symlink as the file itself must not let a write leave the root.
func TestDanglingLeafSymlinkCannotWriteOutside(t *testing.T) {
	skipOnWindows(t, "crea symlinks")
	out := scratch(t)
	d := scratch(t)
	if err := os.Symlink(filepath.Join(out, "stolen.md"), filepath.Join(d, "AGENTS.md")); err != nil {
		t.Fatal(err)
	}
	r := initq(t, d, "--stack", "nextjs")
	mustOK(t, r)
	if got := entries(t, out); len(got) != 0 {
		t.Fatalf("wrote outside the project: %v", got)
	}
	match(t, r.stdout, `(?m)^! AGENTS\.md `)

	home := fakeHome(t)
	if err := os.MkdirAll(filepath.Join(home, ".agents"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(out, "stolen.json"), filepath.Join(home, ".agents/basekit.json")); err != nil {
		t.Fatal(err)
	}
	mustOK(t, run(t, "init", "--global", "-y", "--no-external-skills", "--agents", "claude-code"))
	if got := entries(t, out); len(got) != 0 {
		t.Fatalf("global wrote outside HOME: %v", got)
	}
}

// A2: a dry run never claims that something was installed or prepared.
func TestDryRunNeverSaysDone(t *testing.T) {
	fakeHome(t)
	g := run(t, "init", "--global", "-y", "--dry-run", "--no-external-skills")
	mustOK(t, g)
	noMatch(t, g.stdout, `Listo`)
	match(t, g.stdout, `dry-run`)
	p := initq(t, scratch(t), "--stack", "nextjs", "--dry-run")
	mustOK(t, p)
	noMatch(t, p.stdout, `Listo`)
}

// A3: only the kit's and the pinned external skills are linked, never the user's own.
func TestGlobalLinksOnlyKitAndExternalSkills(t *testing.T) {
	home := fakeHome(t)
	write(t, home, ".agents/skills/my-private/SKILL.md", "---\nname: my-private\n---\n")
	write(t, home, ".agents/skills/grilling/SKILL.md", "---\nname: grilling\n---\n")
	mustOK(t, run(t, "init", "--global", "-y", "--no-external-skills", "--agents", "claude-code"))
	if exists(home, ".claude/skills/my-private") {
		t.Fatal("linked a skill basekit does not own")
	}
	if !exists(home, ".claude/skills/grilling") || !exists(home, ".claude/skills/zone-check") {
		t.Fatal("kit or external skill not linked")
	}
}

// A4: no link is created to a canonical skill that was never written.
func TestNoLinkToMissingTarget(t *testing.T) {
	skipOnWindows(t, "crea symlinks")
	out := scratch(t)
	home := fakeHome(t)
	if err := os.Symlink(out, filepath.Join(home, ".agents")); err != nil {
		t.Fatal(err)
	}
	r := run(t, "init", "--global", "-y", "--no-external-skills", "--agents", "cursor")
	mustOK(t, r)
	if exists(home, ".cursor/skills/zone-check") || len(entries(t, out)) != 0 {
		t.Fatal("linked to a skill that was refused")
	}
	if _, err := os.Lstat(filepath.Join(home, ".cursor/skills/zone-check")); err == nil {
		t.Fatal("dangling link created")
	}
}
