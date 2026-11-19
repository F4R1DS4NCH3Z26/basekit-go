package cli_test

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

var estadoLine = regexp.MustCompile(`(?m)^Estado: .*$`)

// setEstado rewrites the "Estado:" line of docs/<file> in the project at dir.
func setEstado(t *testing.T, dir, file, state string) {
	t.Helper()
	rel := filepath.Join("docs", file)
	write(t, dir, rel, estadoLine.ReplaceAllLiteralString(read(t, dir, rel), "Estado: "+state))
}

// authProject is a fastapi project with both env files, so only the auth gate can stop make.
func authProject(t *testing.T) string {
	t.Helper()
	d := scratch(t)
	mustOK(t, initq(t, d, "--stack", "fastapi"))
	write(t, d, ".env", "APP_PORT=8000\n")
	write(t, d, ".env.prod", "APP_PORT=8000\n")
	return d
}

func dockerCalls(log string) string {
	b, _ := os.ReadFile(log)
	return string(b)
}

const pendingMsg = `docs/AUTH\.md dice "Estado: pendiente"`

func TestProdRefusesWhileAuthPending(t *testing.T) {
	skipOnWindows(t, "usa make")
	for _, args := range [][]string{{"prod-build"}, {"prod-up"}, {"build", "ENV=prod"}, {"up", "ENV=prod"}, {"dev", "ENV=prod"}} {
		t.Run(strings.Join(args, " "), func(t *testing.T) {
			log := fakeTool(t, "docker", 0)
			r := sh(t, authProject(t), "make", args...)
			if r.code == 0 {
				t.Fatal("expected failure")
			}
			match(t, r.stdout+r.stderr, pendingMsg)
			if c := dockerCalls(log); c != "" {
				t.Fatalf("docker was called: %s", c)
			}
		})
	}
}

func TestDevTargetsIgnoreThePendingAuth(t *testing.T) {
	skipOnWindows(t, "usa make")
	log := fakeTool(t, "docker", 0)
	d := authProject(t)
	for _, target := range []string{"dev", "up", "build"} {
		if r := sh(t, d, "make", target); r.code != 0 {
			t.Fatal(target + ": " + r.stdout + r.stderr)
		}
	}
	match(t, dockerCalls(log), `(?m)^compose `)
}

func TestProdBuildsOnceAuthIsDecided(t *testing.T) {
	skipOnWindows(t, "usa make")
	for _, state := range []string{"completo", "sin auth (ADR 0003)"} {
		log := fakeTool(t, "docker", 0)
		d := authProject(t)
		setEstado(t, d, "AUTH.md", state)
		if r := sh(t, d, "make", "prod-build"); r.code != 0 {
			t.Fatal(state + ": " + r.stdout + r.stderr)
		}
		match(t, dockerCalls(log), `(?m)^compose .*build`)
	}
}

func TestProdWithoutAuthDocIsNotBlocked(t *testing.T) {
	skipOnWindows(t, "usa make")
	fakeTool(t, "docker", 0)
	d := authProject(t)
	if err := os.Remove(filepath.Join(d, "docs/AUTH.md")); err != nil {
		t.Fatal(err)
	}
	if r := sh(t, d, "make", "prod-build"); r.code != 0 {
		t.Fatal(r.stdout + r.stderr)
	}
}

func TestCombinedSideReadsRootAuthDoc(t *testing.T) {
	skipOnWindows(t, "usa make")
	log := fakeTool(t, "docker", 0)
	d := scratch(t)
	mustOK(t, initq(t, d, "--stack", "fastapi+react"))
	for _, side := range []string{"backend", "frontend"} {
		write(t, d, side+"/.env.prod", "APP_PORT=8000\n")
	}
	backend := filepath.Join(d, "backend")
	r := sh(t, backend, "make", "prod-build")
	if r.code == 0 {
		t.Fatal("expected failure")
	}
	match(t, r.stdout+r.stderr, `\.\./`+pendingMsg)
	if c := dockerCalls(log); c != "" {
		t.Fatalf("docker was called: %s", c)
	}
	setEstado(t, d, "AUTH.md", "completo")
	if r := sh(t, backend, "make", "prod-build"); r.code != 0 {
		t.Fatal(r.stdout + r.stderr)
	}
}

func doctorProject(t *testing.T) string {
	t.Helper()
	d := scratch(t)
	mustOK(t, initq(t, d, "--stack", "fastapi", "--agents", "none"))
	return d
}

var authLabel = regexp.MustCompile(`AUTH\.md`)

func TestDoctorOnlyInformsBeforeBootstrap(t *testing.T) {
	r := run(t, "doctor", doctorProject(t))
	match(t, lineFor(r.stdout, authLabel), `^· .*pendiente.*bootstrap`)
}

func TestDoctorFlagsDefinedProjectWithPendingAuth(t *testing.T) {
	d := doctorProject(t)
	setEstado(t, d, "PROJECT.md", "definido")
	r := run(t, "doctor", d)
	match(t, lineFor(r.stdout, authLabel), `^FALTA docs/AUTH\.md .*project-bootstrap.*ADR`)
	if r.code != 1 {
		t.Fatalf("exit %d", r.code)
	}
}

func TestDoctorAuthDecidedOrAbsent(t *testing.T) {
	d := doctorProject(t)
	setEstado(t, d, "PROJECT.md", "definido")
	setEstado(t, d, "AUTH.md", "completo")
	match(t, lineFor(run(t, "doctor", d).stdout, authLabel), `^OK +docs/AUTH\.md`)
	if err := os.Remove(filepath.Join(d, "docs/AUTH.md")); err != nil {
		t.Fatal(err)
	}
	if l := lineFor(run(t, "doctor", d).stdout, authLabel); l != "" {
		t.Fatalf("unexpected line: %s", l)
	}
}
