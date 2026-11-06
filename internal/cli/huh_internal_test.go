package cli

import (
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
