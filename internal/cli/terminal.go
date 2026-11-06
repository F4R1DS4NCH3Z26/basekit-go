package cli

import (
	"bufio"
	"fmt"
	"io"
	"slices"
	"strconv"
	"strings"

	"github.com/F4R1DS4NCH3Z26/basekit-go/internal/usage"
)

// terminal implements scaffold.Prompter: it always logs, and asks only when the
// session is interactive. Each question reads one line; an empty line takes the
// default and end of input (EOF) also takes it, so a closed stdin never blocks.
type terminal struct {
	out         io.Writer
	in          *bufio.Reader
	interactive bool
}

func (u *terminal) Log(line string) { fmt.Fprintln(u.out, line) }

func (u *terminal) Interactive() bool { return u.interactive }

func (u *terminal) readLine(prompt string) (string, bool) {
	fmt.Fprint(u.out, prompt)
	line, err := u.in.ReadString('\n')
	if err != nil && line == "" {
		fmt.Fprintln(u.out)
		return "", false
	}
	return strings.TrimSpace(line), true
}

// pick matches an answer against the options: by number (1-based), by full label
// or by its key (the text before " — "), ignoring case.
func pick(answer string, options []string) int {
	if n, err := strconv.Atoi(answer); err == nil {
		if n >= 1 && n <= len(options) {
			return n - 1
		}
		return -1
	}
	for i, o := range options {
		key, _, _ := strings.Cut(o, " — ")
		if strings.EqualFold(answer, o) || strings.EqualFold(answer, key) {
			return i
		}
	}
	return -1
}

func (u *terminal) listOptions(title string, options []string) {
	u.Log(title + ":")
	for i, o := range options {
		u.Log(fmt.Sprintf("  %d) %s", i+1, o))
	}
}

// Choose asks for exactly one option. It has no default: without an answer it fails.
func (u *terminal) Choose(title string, options []string) (int, error) {
	u.listOptions(title, options)
	for {
		answer, ok := u.readLine(fmt.Sprintf("Elige 1-%d: ", len(options)))
		if !ok {
			return 0, usage.Errorf("Sin respuesta para %q", title)
		}
		if i := pick(answer, options); i >= 0 {
			return i, nil
		}
		u.Log("Opción no válida: " + answer)
	}
}

// ChooseMany asks for one or more options separated by commas; empty keeps defaults.
func (u *terminal) ChooseMany(title string, options []string, defaults []int) ([]int, error) {
	u.listOptions(title, options)
	shown := make([]string, len(defaults))
	for i, d := range defaults {
		shown[i] = strconv.Itoa(d + 1)
	}
	for {
		answer, ok := u.readLine(fmt.Sprintf("Elige uno o varios separados por coma [%s]: ", strings.Join(shown, ",")))
		if !ok || answer == "" {
			return defaults, nil
		}
		var chosen []int
		valid := true
		for _, part := range strings.Split(answer, ",") {
			i := pick(strings.TrimSpace(part), options)
			valid = valid && i >= 0
			if i >= 0 && !slices.Contains(chosen, i) {
				chosen = append(chosen, i)
			}
		}
		if valid && len(chosen) > 0 {
			slices.Sort(chosen)
			return chosen, nil
		}
		u.Log("Opción no válida: " + answer)
	}
}

// Confirm asks a yes/no question.
func (u *terminal) Confirm(question string, def bool) (bool, error) {
	hint := "[S/n]"
	if !def {
		hint = "[s/N]"
	}
	for {
		answer, ok := u.readLine(question + " " + hint + ": ")
		switch strings.ToLower(answer) {
		case "":
			return def, nil
		case "s", "si", "sí", "y", "yes":
			return true, nil
		case "n", "no":
			return false, nil
		}
		if !ok {
			return def, nil
		}
		u.Log("Responde s o n")
	}
}

// Text asks for free text; it may be empty.
func (u *terminal) Text(question string) (string, error) {
	answer, _ := u.readLine(question + ": ")
	return answer, nil
}
