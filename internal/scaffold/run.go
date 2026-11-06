package scaffold

import (
	"errors"
	"os"
	"os/exec"
	"strings"
)

// runCmd runs a fixed command with an argument array, never through a shell, so
// user data cannot become shell syntax. Its output is logged indented.
func runCmd(log func(string), dir string, env []string, name string, args ...string) bool {
	cmd := exec.Command(name, args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), env...)
	out, err := cmd.CombinedOutput()
	for _, line := range strings.Split(string(out), "\n") {
		if strings.TrimSpace(line) != "" {
			log("  " + line)
		}
	}
	var exit *exec.ExitError
	if err != nil && !errors.As(err, &exit) {
		log("  " + name + ": " + err.Error())
	}
	return err == nil
}

// works reports whether `name args` exits 0. Output is discarded.
func works(name string, args ...string) bool {
	return exec.Command(name, args...).Run() == nil
}

// commandOutput returns the trimmed stdout of `name args`, or "" on failure.
func commandOutput(name string, args ...string) string {
	out, err := exec.Command(name, args...).Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}
