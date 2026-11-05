// Command basekit prepares the base of a project for AI coding agents.
package main

import (
	"os"

	"github.com/F4R1DS4NCH3Z26/basekit-go/internal/basekit"
)

func main() {
	info, err := os.Stdin.Stat()
	tty := err == nil && info.Mode()&os.ModeCharDevice != 0
	os.Exit(basekit.Main(os.Args[1:], os.Stdin, os.Stdout, os.Stderr, tty))
}
