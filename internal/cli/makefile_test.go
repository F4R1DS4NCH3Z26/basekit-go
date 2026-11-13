package cli_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// fakeTool puts a shell script named name first in PATH; it logs its arguments
// to the returned file and exits with code.
func fakeTool(t *testing.T, name string, code int) string {
	t.Helper()
	bin := t.TempDir()
	log := filepath.Join(bin, name+".log")
	script := "#!/bin/sh\nprintf '%s\\n' \"$*\" >> '" + log + "'\nexit " + string(rune('0'+code)) + "\n"
	if err := os.WriteFile(filepath.Join(bin, name), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	return log
}

func envValue(env, key string) string {
	m := regexp.MustCompile(`(?m)^` + regexp.QuoteMeta(key) + `=(.*)$`).FindStringSubmatch(env)
	if m == nil {
		return ""
	}
	return m[1]
}

// make env on an existing .env adds only the keys .env.example gained, and keeps the old values.
func TestMakeEnvAddsOnlyNewKeys(t *testing.T) {
	skipOnWindows(t, "usa make")
	d := scratch(t)
	mustOK(t, initq(t, d, "--stack", "nextjs"))
	if r := sh(t, d, "make", "env"); r.code != 0 {
		t.Fatal(r.stdout + r.stderr)
	}
	before := read(t, d, ".env")
	mustOK(t, initq(t, d, "--db", "postgres")) // infra-setup appends the DB variables to .env.example
	r := sh(t, d, "make", "env")
	if r.code != 0 {
		t.Fatal(r.stdout + r.stderr)
	}
	after := read(t, d, ".env")
	if !strings.HasPrefix(after, before) {
		t.Fatalf("existing values changed:\nbefore:\n%s\nafter:\n%s", before, after)
	}
	if envValue(after, "DB_PASSWORD") == "" || strings.Contains(after, "__GENERATE") {
		t.Fatalf("new DB variables not added:\n%s", after)
	}
	match(t, r.stdout, `variables nuevas agregadas`)
	if again := sh(t, d, "make", "env"); !strings.Contains(again.stdout, "al día") || read(t, d, ".env") != after {
		t.Fatalf("third run changed .env: %s", again.stdout)
	}
}

// Each __GENERATE__ marker gets its own secret, even two on the same line.
func TestMakeEnvOneSecretPerMarker(t *testing.T) {
	skipOnWindows(t, "usa make")
	d := scratch(t)
	mustOK(t, initq(t, d, "--stack", "fastapi"))
	f, err := os.OpenFile(filepath.Join(d, ".env.example"), os.O_APPEND|os.O_WRONLY, 0)
	if err != nil {
		t.Fatal(err)
	}
	_, _ = f.WriteString("PAIR=__GENERATE__:__GENERATE__\n")
	f.Close()
	if r := sh(t, d, "make", "env"); r.code != 0 {
		t.Fatal(r.stdout + r.stderr)
	}
	parts := strings.Split(envValue(read(t, d, ".env"), "PAIR"), ":")
	if len(parts) != 2 || parts[0] == "" || parts[0] == parts[1] {
		t.Fatalf("PAIR=%v", parts)
	}
}

// A failing openssl leaves no half-written .env behind.
func TestMakeEnvFailureLeavesNoFile(t *testing.T) {
	skipOnWindows(t, "usa make")
	d := scratch(t)
	mustOK(t, initq(t, d, "--stack", "nextjs"))
	fakeTool(t, "openssl", 1)
	if r := sh(t, d, "make", "env"); r.code == 0 {
		t.Fatalf("make env succeeded with a broken openssl: %s", r.stdout)
	}
	if exists(d, ".env") {
		t.Fatalf("partial .env left behind:\n%s", read(t, d, ".env"))
	}
}

// make new-app never calls Docker again once the app exists.
func TestNewAppDoesNothingWhenAppExists(t *testing.T) {
	skipOnWindows(t, "usa make")
	// The file whose presence makes each stack's new-app a no-op, read from its own guard in stack.mk.
	guard := regexp.MustCompile(`Ya existe (\S+); nada que hacer`)
	present := map[string][]string{}
	for _, id := range stackIDs {
		b, err := os.ReadFile(filepath.Join("../../plantillas/stacks", id, "files/.make/stack.mk"))
		m := guard.FindSubmatch(b)
		if err != nil || m == nil {
			t.Fatalf("%s: stack.mk has no \"Ya existe <file>; nada que hacer\" guard (%v)", id, err)
		}
		present[id] = []string{string(m[1])}
	}
	// The markers the stacks were written with.
	for id, want := range map[string]string{"fastapi": "uv.lock", "gin": "go.sum", "php-native": "composer.lock", "nextjs": "package.json", "laravel": "composer.json", "spring": "pom.xml"} {
		if present[id][0] != want {
			t.Errorf("%s: marker %v, want %s", id, present[id], want)
		}
	}
	for _, stack := range stackIDs {
		t.Run(stack, func(t *testing.T) {
			d := scratch(t)
			mustOK(t, initq(t, d, "--stack", stack))
			for _, f := range present[stack] {
				write(t, d, f, "{}\n")
			}
			log := fakeTool(t, "docker", 0)
			r := sh(t, d, "make", "new-app")
			if r.code != 0 {
				t.Fatal(r.stdout + r.stderr)
			}
			if b, err := os.ReadFile(log); err == nil {
				t.Fatalf("docker ran although the app exists: %s", b)
			}
			match(t, r.stdout, `nada que hacer`)
		})
	}
}

// Dev runs under its own Compose project, so `make clean` in dev can never remove prod volumes.
func TestDevAndProdUseDifferentComposeProjects(t *testing.T) {
	skipOnWindows(t, "usa make")
	d := scratch(t)
	mustOK(t, initq(t, d, "--stack", "nextjs"))
	write(t, d, ".env", "X=1\n")
	write(t, d, ".env.prod", "X=1\n")
	project := strings.TrimSpace(sh(t, d, "make", "-s", "_compose-project").stdout)
	match(t, project, `^[a-z0-9][a-z0-9_-]*-dev-[0-9]+$`)
	dev := sh(t, d, "make", "-n", "dev").stdout
	match(t, dev, `docker compose -p `+regexp.QuoteMeta(project)+` `)
	clean := sh(t, d, "make", "-n", "clean", "CONFIRM=1").stdout
	match(t, clean, `-p `+regexp.QuoteMeta(project)+` `)
	prod := sh(t, d, "make", "-n", "prod-up").stdout
	noMatch(t, prod, `docker compose -p `)
	if p := strings.TrimSpace(sh(t, d, "make", "-s", "_compose-project", "ENV=prod").stdout); p != "" {
		t.Fatalf("prod must keep Compose's default project, got %q", p)
	}
}

// With GNU Make 4 (Linux) the shell runs with -u: db-shell must not expand the
// container's variables ($POSTGRES_USER) on the host.
func TestDbShellDoesNotExpandContainerVariables(t *testing.T) {
	requireDocker(t)
	d := scratch(t)
	mustOK(t, initq(t, d, "--stack", "nextjs", "--db", "postgres"))
	write(t, d, ".env", "X=1\n")
	cmd := exec.Command("docker", "run", "--rm", "-v", d+":/w", "-w", "/w", "alpine:3.20", "sh", "-c",
		"apk add --no-cache make bash >/dev/null && make db-shell")
	out, _ := cmd.CombinedOutput()
	noMatch(t, string(out), `unbound variable`)
	match(t, string(out), `docker`) // reached the docker compose exec line
}

const infra = ".agents/skills/infra-setup/scripts/infra.sh"

// mongo-support S1: infra.sh --db mongo stays as the shared groundwork, even while no stack lists mongo.
func TestInfraMongoOverlay(t *testing.T) {
	skipOnWindows(t, "usa scripts bash")
	d := scratch(t)
	mustOK(t, initq(t, d, "--stack", "fastapi"))
	r := sh(t, d, "bash", infra, "--db", "mongo")
	if r.code != 0 {
		t.Fatal(r.stdout + r.stderr)
	}
	if read(t, d, "compose.db.yaml") != read(t, "../../plantillas/skills/infra-setup/assets", "compose.mongo.yaml") {
		t.Fatal("compose.db.yaml is not the mongo overlay")
	}
	match(t, read(t, d, ".make/db.mk"), `mongosh`)
	match(t, read(t, d, ".env.example"), `(?m)^DB_PASSWORD=__GENERATE__$`)
}

// Rerunning infra.sh with the same choice reports "=", not "+"/"~".
func TestInfraRerunReportsUnchanged(t *testing.T) {
	skipOnWindows(t, "usa scripts bash")
	d := scratch(t)
	mustOK(t, initq(t, d, "--stack", "nextjs", "--db", "postgres", "--redis"))
	r := sh(t, d, "bash", infra, "--db", "postgres", "--redis")
	if r.code != 0 {
		t.Fatal(r.stdout + r.stderr)
	}
	noMatch(t, r.stdout, `(?m)^\s+[+~] `)
	match(t, r.stdout, `(?m)^\s+= compose\.db\.yaml`)
	match(t, r.stdout, `(?m)^\s+= \.env\.example \[db\]`)
}

// --db without a value prints the usage and exits 2.
func TestInfraDbWithoutValue(t *testing.T) {
	skipOnWindows(t, "usa scripts bash")
	d := scratch(t)
	mustOK(t, initq(t, d, "--stack", "nextjs"))
	r := sh(t, d, "bash", infra, "--db")
	if r.code != 2 {
		t.Fatalf("exit %d: %s%s", r.code, r.stdout, r.stderr)
	}
	match(t, r.stdout+r.stderr, `Uso: infra\.sh`)
}

// infra.sh keeps the permissions of .env.example (it is committed and shared).
func TestInfraKeepsEnvExamplePermissions(t *testing.T) {
	skipOnWindows(t, "permisos Unix")
	d := scratch(t)
	mustOK(t, initq(t, d, "--stack", "fastapi", "--db", "postgres"))
	info, err := os.Stat(filepath.Join(d, ".env.example"))
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o644 {
		t.Fatalf(".env.example mode %o", info.Mode().Perm())
	}
}

// Every .env variant may hold secrets: all are ignored except the secret-free templates.
func TestGitignoreCoversEveryEnvFile(t *testing.T) {
	d := scratch(t)
	mustOK(t, initq(t, d, "--stack", "nextjs"))
	for _, f := range []string{".env", ".env.prod", ".env.local", ".env.staging", ".env.dev.local", "server.key"} {
		if r := sh(t, d, "git", "check-ignore", "-q", f); r.code != 0 {
			t.Errorf("%s is not ignored", f)
		}
	}
	for _, f := range []string{".env.example", ".env.sample", ".env.template"} {
		if r := sh(t, d, "git", "check-ignore", "-q", f); r.code == 0 {
			t.Errorf("%s must stay versioned", f)
		}
	}
}

const zoneCheck = ".agents/skills/zone-check/scripts/check_zones.sh"

// A rename counts as deleting the old path and adding the new one: moving a file
// out of another agent's zone is a violation.
func TestZoneCheckSeesBothSidesOfARename(t *testing.T) {
	skipOnWindows(t, "usa scripts bash")
	d := scratch(t)
	mustOK(t, initq(t, d, "--stack", "nextjs"))
	write(t, d, "src/components/Button.tsx", "export {}\n") // ui-writer's zone
	gitCommitAll(t, d)
	if err := os.MkdirAll(filepath.Join(d, "src/app/api"), 0o755); err != nil {
		t.Fatal(err)
	}
	if r := sh(t, d, "git", "mv", "src/components/Button.tsx", "src/app/api/button.ts"); r.code != 0 {
		t.Fatal(r.stderr)
	}
	r := sh(t, d, "bash", zoneCheck, "data-writer")
	if r.code != 1 {
		t.Fatalf("exit %d: the old path left ui-writer's zone unnoticed\n%s", r.code, r.stdout)
	}
	match(t, r.stdout, `FUERA DE ZONA: src/components/Button\.tsx`)
}

// Paths with accents are read literally, not as git's quoted escapes: after a
// snapshot, a later edit to such a file outside the agent's zone is still caught.
func TestZoneCheckNonASCIIPaths(t *testing.T) {
	skipOnWindows(t, "usa scripts bash")
	d := scratch(t)
	mustOK(t, initq(t, d, "--stack", "nextjs"))
	gitCommitAll(t, d)
	// git's default (core.quotePath=true) escapes non-ASCII bytes; force it whatever the user's config says.
	t.Setenv("GIT_CONFIG_COUNT", "1")
	t.Setenv("GIT_CONFIG_KEY_0", "core.quotePath")
	t.Setenv("GIT_CONFIG_VALUE_0", "true")
	file := "src/components/canción y más.tsx" // ui-writer's zone
	write(t, d, file, "export {}\n")
	if r := sh(t, d, "bash", zoneCheck, "--snapshot"); r.code != 0 {
		t.Fatal(r.stdout + r.stderr)
	}
	write(t, d, file, "export const x = 1\n") // changed after the snapshot
	r := sh(t, d, "bash", zoneCheck, "data-writer")
	if r.code != 1 {
		t.Fatalf("exit %d: an edit after the snapshot went unnoticed\n%s", r.code, r.stdout)
	}
	match(t, r.stdout, `FUERA DE ZONA: src/components/canción y más\.tsx`)
}

// An existing .env whose last line has no newline (common after hand edits) keeps
// that value intact: the new keys start on their own line.
func TestMakeEnvKeepsLastValueWithoutTrailingNewline(t *testing.T) {
	skipOnWindows(t, "usa make")
	d := scratch(t)
	mustOK(t, initq(t, d, "--stack", "nextjs"))
	write(t, d, ".env", "APP_PORT=3000\nCORS_ORIGINS=https://x.com") // no final newline
	if r := sh(t, d, "make", "env"); r.code != 0 {
		t.Fatal(r.stdout + r.stderr)
	}
	env := read(t, d, ".env")
	if envValue(env, "CORS_ORIGINS") != "https://x.com" {
		t.Fatalf("existing value corrupted:\n%s", env)
	}
	if envValue(env, "BETTER_AUTH_SECRET") == "" {
		t.Fatalf("new key not added on its own line:\n%s", env)
	}
	if again := sh(t, d, "make", "env"); read(t, d, ".env") != env {
		t.Fatalf("second run changed .env: %s", again.stdout)
	}
}
