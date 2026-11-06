package cli_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Template fixes that came from the Node implementation (ADR 0001: both ship the same plantillas/).

// A symlinked .env.example whose target is outside the project is refused before any change.
func TestInfraRefusesEnvExampleLinkOutsideProject(t *testing.T) {
	skipOnWindows(t, "usa scripts bash y enlaces simbólicos")
	d := scratch(t)
	mustOK(t, initq(t, d, "--stack", "nextjs"))
	gitCommitAll(t, d)
	outside := filepath.Join(t.TempDir(), "team.env.example")
	if err := os.WriteFile(outside, []byte("SHARED=1\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(filepath.Join(d, ".env.example")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(d, ".env.example")); err != nil {
		t.Fatal(err)
	}
	before := sh(t, d, "git", "status", "--porcelain").stdout
	r := sh(t, d, "bash", infra, "--db", "postgres")
	if r.code != 1 {
		t.Fatalf("exit %d: %s%s", r.code, r.stdout, r.stderr)
	}
	match(t, r.stdout, `ERROR: \.env\.example apunta fuera del proyecto`)
	if got, _ := os.ReadFile(outside); string(got) != "SHARED=1\n" {
		t.Fatalf("a file outside the project was modified: %q", got)
	}
	if after := sh(t, d, "git", "status", "--porcelain").stdout; after != before {
		t.Fatalf("the project changed:\n%s", after)
	}
}

// An in-project link is kept and its target is replaced in one step: a failing mv leaves it intact.
func TestInfraUpdatesInProjectLinkTargetAtomically(t *testing.T) {
	skipOnWindows(t, "usa scripts bash y enlaces simbólicos")
	d := scratch(t)
	mustOK(t, initq(t, d, "--stack", "nextjs"))
	if err := os.Rename(filepath.Join(d, ".env.example"), filepath.Join(d, "shared.env.example")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("shared.env.example", filepath.Join(d, ".env.example")); err != nil {
		t.Fatal(err)
	}
	before := read(t, d, "shared.env.example")
	log := fakeTool(t, "mv", 1)
	if r := sh(t, d, "bash", infra, "--db", "postgres"); r.code == 0 {
		t.Fatal("the failing mv must stop the script")
	}
	logged, _ := os.ReadFile(log)
	match(t, string(logged), `^\S*/\.env\.example\.\w+ \S*/shared\.env\.example\n$`)
	if read(t, d, "shared.env.example") != before {
		t.Fatal("the shared file was left half written")
	}
	for _, e := range entries(t, d) {
		if strings.HasPrefix(e, ".env.example.") {
			t.Fatalf("temporary copy left behind: %s", e)
		}
	}
	if info, err := os.Lstat(filepath.Join(d, ".env.example")); err != nil || info.Mode()&os.ModeSymlink == 0 {
		t.Fatal("the link was replaced by a regular file")
	}
}

// Laravel's new-app forwards COMPOSER_AUTH to the composer container by name, never its value.
func TestLaravelNewAppForwardsComposerAuthByName(t *testing.T) {
	skipOnWindows(t, "usa make")
	d := scratch(t)
	mustOK(t, initq(t, d, "--stack", "laravel"))
	log := fakeTool(t, "docker", 0)
	t.Setenv("COMPOSER_AUTH", `{"github-oauth":{"github.com":"tok-123"}}`)
	r := sh(t, d, "make", "_new-app")
	if r.code != 0 {
		t.Fatal(r.stdout + r.stderr)
	}
	logged, _ := os.ReadFile(log)
	match(t, string(logged), `(?m)^run .*-e COMPOSER_AUTH .*composer:2 `)
	if strings.Contains(string(logged)+r.stdout+r.stderr, "tok-123") {
		t.Fatal("the token reached argv or the log")
	}
}

// Next.js new-app resolves every dependency as of 14 days ago, set before any npx/npm call.
func TestNextjsNewAppResolvesDependenciesFourteenDaysOld(t *testing.T) {
	skipOnWindows(t, "usa make")
	d := scratch(t)
	mustOK(t, initq(t, d, "--stack", "nextjs"))
	r := sh(t, d, "make", "-n", "_new-app")
	if r.code != 0 {
		t.Fatal(r.stderr)
	}
	match(t, r.stdout, `export npm_config_before="\$\(node -p "new Date\(Date\.now\(\) - 14 \* 864e5\)\.toISOString\(\)"\)"`)
	if strings.Index(r.stdout, "npm_config_before") > strings.Index(r.stdout, "create-next-app") {
		t.Fatal("npm_config_before must be set before any npx/npm call")
	}
}
