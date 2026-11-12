//go:build !windows

package scaffold

import "errors"

var errWindowsOnly = errors.New("solo en Windows")

// removeFromUserPath only exists on Windows, where install.ps1 adds basekit's folder to the user PATH.
func removeFromUserPath(string, func(string)) error { return errWindowsOnly }

// deleteAfterExit only exists on Windows: elsewhere a running executable can remove itself.
func deleteAfterExit(string, bool) error { return errWindowsOnly }
