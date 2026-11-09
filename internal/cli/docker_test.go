package cli_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
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
		stack := s.ID
		for _, db := range []string{"postgres", "mysql", "sqlite"} {
			if stack == "spring" && db == "sqlite" {
				continue // guide 05 §3: SQLite is not offered for Spring
			}
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

// e2eDB is the database an E2E run of a stack uses: BASEKIT_E2E_DB when set ("none" for no database), otherwise
// none for a frontend stack (it has no database), postgres for Spring (its new-app adds data-jpa, which needs a
// database to start), SQLite in prod (the non-root image must write its volume) and none in dev.
func e2eDB(t *testing.T, id string, prod bool) string {
	t.Helper()
	db := os.Getenv("BASEKIT_E2E_DB")
	switch {
	case db == "none":
		return ""
	case db != "":
		if db != "postgres" && db != "mysql" && db != "sqlite" {
			t.Fatalf("BASEKIT_E2E_DB=%q: usa postgres, mysql, sqlite o none", db)
		}
		return db
	case stackByID(t, id).Kind == "frontend":
		return ""
	case id == "spring":
		return "postgres"
	case prod:
		return "sqlite"
	}
	return ""
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
			initWithDB(t, d, stack, e2eDB(t, stack, false))
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
		db := e2eDB(t, stack, true)
		name := db
		if name == "" {
			name = "none"
		}
		t.Run(stack+"/"+name, func(t *testing.T) {
			d := scratch(t)
			port, health := stackData(t, stack)
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
