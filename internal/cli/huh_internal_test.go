package cli

import (
	"errors"
	"fmt"
	"os"
	"strings"
	"testing"
)

func TestUseHuhOnlyOnTheRealTerminal(t *testing.T) {
	t.Setenv("ACCESSIBLE", "")
	if useHuh(strings.NewReader(""), true) {
		t.Error("a pipe or a test reader must use the line terminal")
	}
	if useHuh(os.Stdin, false) {
		t.Error("a non-interactive session must never use huh")
	}
	if !useHuh(os.Stdin, true) {
		t.Error("the interactive terminal must use huh")
	}
	t.Setenv("ACCESSIBLE", "1")
	if useHuh(os.Stdin, true) {
		t.Error("ACCESSIBLE=1 must use the line terminal")
	}
}

func TestCancelledQuestionExitsWith130(t *testing.T) {
	var stderr strings.Builder
	if code := exitCode(fmt.Errorf("init: %w", errCancelled), &stderr); code != 130 {
		t.Fatalf("code %d, want 130", code)
	}
	if got := stderr.String(); got != errCancelled.Error()+"\n" {
		t.Fatalf("stderr %q", got)
	}
	stderr.Reset()
	if code := exitCode(errors.New("boom"), &stderr); code != 1 || stderr.String() != "ERROR: boom\n" {
		t.Fatalf("plain error: code %d, stderr %q", code, stderr.String())
	}
}
