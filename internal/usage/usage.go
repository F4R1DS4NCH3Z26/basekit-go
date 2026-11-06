// Package usage holds the error type for bad user input; the CLI exits with 2 on it.
package usage

import "fmt"

// Error is an error caused by the user's input (an unknown flag, value or stack).
type Error struct{ msg string }

func (e *Error) Error() string { return e.msg }

// Errorf builds a usage error.
func Errorf(format string, a ...any) error { return &Error{fmt.Sprintf(format, a...)} }
