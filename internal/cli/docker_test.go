package cli_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/F4R1DS4NCH3Z26/basekit-go/internal/stack"
)

func requireDocker(t *testing.T) {
	t.Helper()
	skipOnWindows(t, "los stacks usan contenedores Linux y make")
	if exec.Command("docker", "info").Run() != nil {
		t.Skip("Docker no disponible")
	}
}

// stackData reads the fields of a stack.json the Docker tests need.
func stackData(t *testing.T, id string) (port int, health string) {
	t.Helper()
	var s struct {
		Port   int    `json:"port"`
		Health string `json:"health"`
	}
	b, err := os.ReadFile(filepath.Join("../../plantillas/stacks", id, "stack.json"))
	if err != nil || json.Unmarshal(b, &s) != nil || s.Health == "" {
		t.Fatalf("stack.json de %s sin port/health: %v", id, err)
	}
	return s.Port, s.Health
}

// Acceptance 9: the compose files merge with the DB overlay and migrate gets DATABASE_URL.
func TestComposeConfigWithDbOverlay(t *testing.T) {
	requireDocker(t)
	for _, s := range allStacks {
		if s.Kind == "frontend" {
			continue // a frontend stack has no database (S1b)
		}
		engines := stack.AcceptedEngines(s) // S6: only the engines its stack.json accepts
		stack := s.ID
		for _, db := range engines {
			t.Run(stack+"/"+db, func(t *testing.T) {
				d := scratch(t)
				mustOK(t, initq(t, d, "--stack", stack, "--db", db))
				if r := sh(t, d, "make", "env"); r.code != 0 {
					t.Fatal(r.stdout + r.stderr)
				}
				cfg := sh(t, d, "docker", "compose", "-f", "compose.yaml", "-f", "compose.dev.yaml", "-f", "compose.db.yaml",
					"--profile", "tools", "config", "--format", "json")
				if cfg.code != 0 {
					t.Fatal(cfg.stderr)
				}
				var c struct {
					Services map[string]struct {
						Environment map[string]*string `json:"environment"`
					} `json:"services"`
				}
				if err := json.Unmarshal([]byte(cfg.stdout), &c); err != nil {
					t.Fatal(err)
				}
				if url := c.Services["migrate"].Environment["DATABASE_URL"]; url == nil || *url == "" {
					t.Fatalf("migrate sin DATABASE_URL: %v", c.Services["migrate"].Environment)
				}
			})
		}
	}
}

// e2eDB is the database an E2E run of a stack uses, and why a subtest is skipped ("" when it runs). Precedence:
//  1. a frontend stack never has a database: an explicit BASEKIT_E2E_DB is not forced onto it (the run goes without
//     one and note says so);
//  2. BASEKIT_E2E_DB=sqlite skips Spring (guide 05 §3: SQLite is not offered for Spring), and any other engine the
//     stack does not accept (stack.json dbs; mongo only where listed) skips it too;
//  3. otherwise an explicit BASEKIT_E2E_DB wins ("none" for no database);
//  4. defaults: postgres for Spring (its new-app adds data-jpa, which needs a database to start), SQLite in prod (the
//     non-root image must write its volume) and none in dev.
//
// An unknown BASEKIT_E2E_DB is an error.
func e2eDB(env string, s stack.Stack, prod bool) (db, note, skip string, err error) {
	if env != "" && env != "none" && !slices.Contains(stack.Engines, env) {
		return "", "", "", fmt.Errorf("BASEKIT_E2E_DB=%q: usa %s o none", env, strings.Join(stack.Engines, ", "))
	}
	switch {
	case !stack.HasDatabase(s):
		if env != "" && env != "none" {
			note = fmt.Sprintf("BASEKIT_E2E_DB=%s no aplica a %s (stack frontend, sin base de datos): corre sin base de datos", env, s.ID)
		}
		return "", note, "", nil
	case env == "sqlite" && s.ID == "spring":
		return "", "", "BASEKIT_E2E_DB=sqlite no aplica a spring (guía 05, sección 3: Spring no ofrece SQLite): se omite", nil
	case env != "" && env != "none" && !slices.Contains(stack.AcceptedEngines(s), env):
		return "", "", fmt.Sprintf("BASEKIT_E2E_DB=%s no aplica a %s (motores admitidos: %s): se omite", env, s.ID, strings.Join(stack.AcceptedEngines(s), ", ")), nil
	case env == "none":
		return "", "", "", nil
	case env != "":
		return env, "", "", nil
	case s.ID == "spring":
		return "postgres", "", "", nil
	case prod:
		return "sqlite", "", "", nil
	}
	return "", "", "", nil
}

// e2eDBFor applies e2eDB to a subtest: it fails on a bad BASEKIT_E2E_DB, skips what does not apply and logs the note.
func e2eDBFor(t *testing.T, id string, prod bool) string {
	t.Helper()
	db, note, skip, err := e2eDB(os.Getenv("BASEKIT_E2E_DB"), stackByID(t, id), prod)
	if err != nil {
		t.Fatal(err)
	}
	if skip != "" {
		t.Skip(skip)
	}
	if note != "" {
		t.Log(note)
	}
	return db
}

func TestE2EDBPrecedence(t *testing.T) {
	front := stacksOf("frontend")[0]
	spring, back := stackByID(t, "spring"), stackByID(t, "fastapi")
	mongoStack := back
	mongoStack.ID, mongoStack.Dbs = "mongo-listed", []string{"postgres", "mongo"}
	cases := []struct {
		env        string
		s          stack.Stack
		prod       bool
		db         string
		note, skip bool
	}{
		{"", front, true, "", false, false},
		{"none", front, false, "", false, false},
		{"postgres", front, false, "", true, false},
		{"sqlite", front, true, "", true, false},
		{"sqlite", spring, true, "", false, true},
		{"sqlite", spring, false, "", false, true},
		{"mysql", spring, true, "mysql", false, false},
		{"", spring, false, "postgres", false, false},
		{"none", spring, true, "", false, false},
		{"", back, true, "sqlite", false, false},
		{"", back, false, "", false, false},
		{"mysql", back, false, "mysql", false, false},
		{"none", back, true, "", false, false},
		{"mongo", back, false, "", false, true},
		{"mongo", spring, false, "", false, true},
		{"mongo", mongoStack, false, "mongo", false, false},
		{"mongo", mongoStack, true, "mongo", false, false},
		{"mongo", front, false, "", true, false},
	}
	for _, c := range cases {
		db, note, skip, err := e2eDB(c.env, c.s, c.prod)
		if err != nil || db != c.db || (note != "") != c.note || (skip != "") != c.skip {
			t.Errorf("%s/%q/prod=%v: db %q note %q skip %q err %v", c.s.ID, c.env, c.prod, db, note, skip, err)
		}
	}
	if _, note, _, _ := e2eDB("postgres", front, false); note != "BASEKIT_E2E_DB=postgres no aplica a "+front.ID+" (stack frontend, sin base de datos): corre sin base de datos" {
		t.Errorf("note: %q", note)
	}
	if _, _, _, err := e2eDB("oracle", back, false); err == nil {
		t.Error("an unknown BASEKIT_E2E_DB is an error")
	}
	if _, _, skip, _ := e2eDB("mongo", back, false); skip != "BASEKIT_E2E_DB=mongo no aplica a fastapi (motores admitidos: postgres, mysql, sqlite): se omite" {
		t.Errorf("skip: %q", skip)
	}
}

// composeArgs are the compose files of an environment plus the database and Redis overlays init wrote.
func composeArgs(d, env string) []string {
	args := []string{"-f", "compose.yaml", "-f", "compose." + env + ".yaml"}
	for _, f := range []string{"compose.db.yaml", "compose.redis.yaml"} {
		if exists(d, f) {
			args = append(args, "-f", f)
		}
	}
	return args
}

// initWithDB runs init for an E2E test, with --db only when there is one.
func initWithDB(t *testing.T, d, stack, db string) {
	t.Helper()
	extra := []string{"--stack", stack}
	if db != "" {
		extra = append(extra, "--db", db)
	}
	mustOK(t, initq(t, d, extra...))
}

const writeTest = "touch /app/data/.write-test && rm /app/data/.write-test"

// canWriteData checks that the app (non-root in prod) can write its SQLite directory, as the real app process
// (compose exec). Only an image without a shell (`scratch`) is checked from a throwaway container with the app's user
// and volumes; a failed pull of that helper image is reported as such, never as a permission error.
func canWriteData(t *testing.T, d string, dc []string) {
	t.Helper()
	if sh(t, d, "docker", append(dc, "exec", "-T", "app", "sh", "-c", "exit 0")...).code == 0 {
		if w := sh(t, d, "docker", append(dc, "exec", "-T", "app", "sh", "-c", writeTest)...); w.code != 0 {
			t.Fatalf("the app cannot write /app/data: %s%s", w.stdout, w.stderr)
		}
		return
	}
	cid := strings.TrimSpace(sh(t, d, "docker", append(dc, "ps", "-q", "app")...).stdout)
	if cid == "" {
		t.Fatal("the app service is not running")
	}
	user := strings.TrimSpace(sh(t, d, "docker", "inspect", "-f", "{{.Config.User}}", cid).stdout)
	if sh(t, d, "docker", "image", "inspect", "alpine:3.22").code != 0 && sh(t, d, "docker", "pull", "-q", "alpine:3.22").code != 0 {
		t.Fatal("could not pull alpine:3.22 to check /app/data (network or registry), not a stack failure")
	}
	args := []string{"run", "--rm"}
	if user != "" {
		args = append(args, "--user", user)
	}
	args = append(args, "--volumes-from", cid, "alpine:3.22", "sh", "-c", writeTest)
	if w := sh(t, d, "docker", args...); w.code != 0 {
		t.Fatalf("the app cannot write /app/data: %s%s", w.stdout, w.stderr)
	}
}

// parseE2EPort returns the host port the e2e publishes the app on: BASEKIT_E2E_PORT when set (a busy port on
// the machine, for example another dev server, must not get in the way), otherwise the stack's own port.
func parseE2EPort(v string, stackPort int) (int, error) {
	if v == "" {
		return stackPort, nil
	}
	p, err := strconv.Atoi(v)
	if err != nil || p < 1 || p > 65535 {
		return 0, fmt.Errorf("BASEKIT_E2E_PORT=%q no es un puerto válido (1-65535)", v)
	}
	return p, nil
}

// e2ePort applies BASEKIT_E2E_PORT: it exports APP_PORT, which Compose interpolation prefers over the --env-file
// value, so the stack's compose publishes the app there.
func e2ePort(t *testing.T, stackPort int) int {
	v := os.Getenv("BASEKIT_E2E_PORT")
	p, err := parseE2EPort(v, stackPort)
	if err != nil {
		t.Fatal(err)
	}
	if v != "" {
		t.Setenv("APP_PORT", strconv.Itoa(p))
	}
	return p
}

func TestE2EPortOverride(t *testing.T) {
	for _, c := range []struct {
		v    string
		want int
		bad  bool
	}{{"", 3000, false}, {"3999", 3999, false}, {"0", 0, true}, {"70000", 0, true}, {"x", 0, true}} {
		got, err := parseE2EPort(c.v, 3000)
		if (err != nil) != c.bad || got != c.want {
			t.Errorf("parseE2EPort(%q) = %d, %v", c.v, got, err)
		}
	}
}

// Acceptance 12: `make dev` after `make new-app` answers on the stack healthcheck.
// It builds images and downloads dependencies, so it only runs with BASEKIT_E2E=1.
// BASEKIT_E2E_STACK limits it to one stack (CI runs one job per stack).
func TestDevAnswersHealthcheck(t *testing.T) {
	if os.Getenv("BASEKIT_E2E") != "1" {
		t.Skip("BASEKIT_E2E=1 para construir y levantar cada stack")
	}
	requireDocker(t)
	only := os.Getenv("BASEKIT_E2E_STACK")
	for _, stack := range stackIDs {
		if only != "" && only != stack {
			continue
		}
		t.Run(stack, func(t *testing.T) {
			d := scratch(t)
			port, health := stackData(t, stack)
			port = e2ePort(t, port)
			initWithDB(t, d, stack, e2eDBFor(t, stack, false))
			t.Cleanup(func() {
				// Dev runs under its own Compose project (make _compose-project).
				project := strings.TrimSpace(sh(t, d, "make", "-s", "_compose-project").stdout)
				dc := append([]string{"compose", "-p", project}, composeArgs(d, "dev")...)
				if t.Failed() {
					logs := sh(t, d, "docker", append(dc, "logs", "--no-color", "--tail=80")...)
					t.Log(logs.stdout + logs.stderr)
				}
				sh(t, d, "docker", append(dc, "down", "-v", "--rmi", "local")...)
				if runtime.GOOS == "linux" {
					sh(t, d, "make", "fix-perms") // container-created files must be removable
				}
			})
			for _, target := range []string{"new-app", "env", "dev"} {
				if r := sh(t, d, "make", target); r.code != 0 {
					t.Fatalf("make %s: %s%s", target, r.stdout, r.stderr)
				}
			}
			url := fmt.Sprintf("http://localhost:%d%s", port, health)
			deadline := time.Now().Add(5 * time.Minute)
			for {
				resp, err := http.Get(url)
				if err == nil {
					resp.Body.Close()
					if resp.StatusCode == http.StatusOK {
						return
					}
				}
				if time.Now().After(deadline) {
					t.Fatalf("%s no respondió 200 en 5 minutos (último error: %v)", url, err)
				}
				time.Sleep(3 * time.Second)
			}
		})
	}
}

// Production images: `make prod-up` answers on the healthcheck and, with SQLite,
// the non-root app can write its database directory. BASEKIT_E2E=1 like test 12.
func TestProdAnswersHealthcheck(t *testing.T) {
	if os.Getenv("BASEKIT_E2E") != "1" {
		t.Skip("BASEKIT_E2E=1 para construir y levantar cada stack")
	}
	requireDocker(t)
	only := os.Getenv("BASEKIT_E2E_STACK")
	for _, stack := range stackIDs {
		if only != "" && only != stack {
			continue
		}
		t.Run(stack, func(t *testing.T) {
			db := e2eDBFor(t, stack, true)
			d := scratch(t)
			port, health := stackData(t, stack)
			port = e2ePort(t, port)
			initWithDB(t, d, stack, db)
			dc := append([]string{"compose", "--env-file", ".env.prod"}, composeArgs(d, "prod")...)
			t.Cleanup(func() {
				if t.Failed() {
					logs := sh(t, d, "docker", append(dc, "logs", "--no-color", "--tail=80")...)
					t.Log(logs.stdout + logs.stderr)
				}
				sh(t, d, "docker", append(dc, "down", "-v", "--rmi", "local")...)
				if runtime.GOOS == "linux" {
					sh(t, d, "make", "fix-perms")
				}
			})
			for _, args := range [][]string{{"new-app"}, {"env", "ENV=prod"}, {"prod-up"}} {
				if r := sh(t, d, "make", args...); r.code != 0 {
					t.Fatalf("make %v: %s%s", args, r.stdout, r.stderr)
				}
			}
			waitHealthy(t, fmt.Sprintf("http://localhost:%d%s", port, health))
			if db == "sqlite" {
				canWriteData(t, d, dc)
			}
		})
	}
}

func waitHealthy(t *testing.T, url string) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Minute)
	for {
		resp, err := http.Get(url)
		if err == nil {
			resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				return
			}
		}
		if time.Now().After(deadline) {
			t.Fatalf("%s no respondió 200 en 5 minutos (último error: %v)", url, err)
		}
		time.Sleep(3 * time.Second)
	}
}
