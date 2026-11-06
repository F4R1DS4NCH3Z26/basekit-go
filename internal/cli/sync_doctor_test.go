package cli_test

import (
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"strings"
	"testing"
)

func TestSyncFailsWithoutManifest(t *testing.T) {
	r := run(t, "sync", scratch(t))
	if r.code == 0 {
		t.Fatal("expected failure")
	}
	match(t, r.stderr, `manifest`)
}

func TestSyncRegeneratesAdaptersFromRoles(t *testing.T) {
	d := scratch(t)
	mustOK(t, initq(t, d, "--stack", "nextjs", "--agents", "claude-code,opencode"))
	role := ".agents/roles/data-writer.md"
	write(t, d, role, regexp.MustCompile(`(?m)^description: .*$`).ReplaceAllString(read(t, d, role), "description: Nueva descripción: con dos puntos"))
	write(t, d, ".agents/zones/data-writer.txt", "src/nuevo/**\n")
	if err := os.Remove(filepath.Join(d, ".claude/agents/ui-writer.md")); err != nil {
		t.Fatal(err)
	}

	r := run(t, "sync", d, "--no-external-skills")
	mustOK(t, r)
	match(t, read(t, d, ".claude/agents/data-writer.md"), `(?m)^description: "Nueva descripción: con dos puntos"$`)
	match(t, read(t, d, ".opencode/agents/data-writer.md"), `(?m)^ {4}"src/nuevo/\*\*": allow$`)
	if !exists(d, ".claude/agents/ui-writer.md") {
		t.Fatal("deleted adapter not recreated")
	}
	match(t, r.stdout, `(?m)^~ \.claude/agents/data-writer\.md`)
	match(t, r.stdout, `(?m)^\+ \.claude/agents/ui-writer\.md`)
}

func TestSyncRerendersOnlyTheBlock(t *testing.T) {
	d := scratch(t)
	mustOK(t, initq(t, d, "--stack", "spring"))
	original := read(t, d, "AGENTS.md")
	write(t, d, "AGENTS.md", strings.Replace(original, "## Reglas globales", "## X", 1)+"\n## Mío\n")
	mustOK(t, run(t, "sync", d, "--no-external-skills"))
	if got := read(t, d, "AGENTS.md"); got != original+"\n## Mío\n" {
		t.Fatal(got)
	}
}

func TestSyncAddsAgent(t *testing.T) {
	npx := fakeNpx(t)
	d := scratch(t)
	mustOK(t, run(t, "init", d, "-y", "--stack", "fastapi", "--agents", "opencode"))
	mustOK(t, run(t, "sync", d, "--agents", "opencode,claude-code"))
	for _, f := range []string{".claude/agents/data-writer.md", ".claude/skills/py-scaffold/SKILL.md", ".claude/skills/grilling/SKILL.md"} {
		if !exists(d, f) {
			t.Fatal(f)
		}
	}
	if got := manifest(t, d)["agents"]; !reflect.DeepEqual(got, []any{"claude-code", "opencode"}) {
		t.Fatal(got)
	}
	calls := argsCalls(npx())
	if len(calls) != 2 {
		t.Fatal(calls)
	}
	match(t, calls[1], ` -a claude-code opencode -y$`)
}

func TestSyncIsIdempotent(t *testing.T) {
	d := scratch(t)
	mustOK(t, initq(t, d, "--stack", "laravel", "--agents", allAdapters))
	gitCommitAll(t, d)
	r := run(t, "sync", d, "--no-external-skills")
	mustOK(t, r)
	if changed := actionLines(r.stdout, "+~!"); len(changed) > 0 {
		t.Fatal(changed)
	}
	gitClean(t, d)
}

func TestSyncRejectsInitOnlyOptions(t *testing.T) {
	d := scratch(t)
	mustOK(t, initq(t, d, "--stack", "nextjs"))
	r := run(t, "sync", d, "--stack", "spring")
	if r.code != 2 {
		t.Fatalf("exit %d", r.code)
	}
	match(t, r.stderr, `--stack`)
}

func TestDoctorFreshProject(t *testing.T) {
	fakeNpx(t)
	d := scratch(t)
	mustOK(t, run(t, "init", d, "-y", "--stack", "nextjs", "--agents", "claude-code,hermes-agent"))
	r := run(t, "doctor", d)
	if r.code != 1 {
		t.Fatalf("exit %d: a fresh project lacks the app and .env", r.code)
	}
	for _, label := range []string{`(?i)node`, `git\b`, `manifest`, `AGENTS\.md`, `Makefile`, `\.make/stack\.mk`, `Dockerfile`, `compose\.yaml`, `\.env\.example`,
		`nextjs-verify`, `grill-with-docs`, `\.claude/skills`, `\.hermes/skills`, `HERMES`} {
		match(t, lineFor(r.stdout, regexp.MustCompile(label)), `^OK `)
	}
	match(t, lineFor(r.stdout, regexp.MustCompile(`package\.json`)), `^FALTA .*make new-app`)
	match(t, lineFor(r.stdout, regexp.MustCompile(`^\S+ +\.env(\s|$)`)), `^FALTA .*make env`)
}

func TestDoctorFlagsMissingPieces(t *testing.T) {
	d := scratch(t)
	mustOK(t, initq(t, d, "--stack", "fastapi", "--agents", "hermes-agent"))
	write(t, d, ".hermes.md", "x\n")
	r := run(t, "doctor", d)
	if r.code != 1 {
		t.Fatalf("exit %d", r.code)
	}
	match(t, lineFor(r.stdout, regexp.MustCompile(`grilling`)), `^FALTA `)
	match(t, lineFor(r.stdout, regexp.MustCompile(`HERMES`)), `^FALTA .*\.hermes\.md`)

	empty := run(t, "doctor", scratch(t))
	if empty.code != 1 {
		t.Fatalf("exit %d", empty.code)
	}
	match(t, lineFor(empty.stdout, regexp.MustCompile(`manifest`)), `^FALTA .*basekit init`)
}

func TestDoctorNeverReadsEnv(t *testing.T) {
	d := scratch(t)
	mustOK(t, initq(t, d, "--stack", "nextjs"))
	write(t, d, ".env", "SECRET=leak-me\n")
	r := run(t, "doctor", d)
	noMatch(t, r.stdout+r.stderr, `leak-me`)
	match(t, lineFor(r.stdout, regexp.MustCompile(`^\S+ +\.env(\s|$)`)), `^OK `)
}
