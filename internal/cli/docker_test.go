package basekit_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"
	"time"
)

func requireDocker(t *testing.T) {
	t.Helper()
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
	for _, stack := range stackIDs {
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
			mustOK(t, initq(t, d, "--stack", stack))
			t.Cleanup(func() {
				if t.Failed() {
					logs := sh(t, d, "docker", "compose", "-f", "compose.yaml", "-f", "compose.dev.yaml", "logs", "--no-color", "--tail=80")
					t.Log(logs.stdout + logs.stderr)
				}
				sh(t, d, "docker", "compose", "-f", "compose.yaml", "-f", "compose.dev.yaml", "down", "-v", "--rmi", "local")
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
