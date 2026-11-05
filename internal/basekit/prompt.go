package basekit

import (
	"fmt"
	"slices"
	"strconv"
	"strings"
)

// Terminal questions. They read one line each; an empty line takes the default and
// end of input (EOF) also takes it, so a closed stdin never blocks.

func (u *ui) readLine(prompt string) (string, bool) {
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

func (u *ui) listOptions(title string, options []string) {
	u.log(title + ":")
	for i, o := range options {
		u.log(fmt.Sprintf("  %d) %s", i+1, o))
	}
}

// choose asks for exactly one option. It has no default: without an answer it fails.
func (u *ui) choose(title string, options []string) (int, error) {
	u.listOptions(title, options)
	for {
		answer, ok := u.readLine(fmt.Sprintf("Elige 1-%d: ", len(options)))
		if !ok {
			return 0, usageErr("Sin respuesta para %q", title)
		}
		if i := pick(answer, options); i >= 0 {
			return i, nil
		}
		u.log("Opción no válida: " + answer)
	}
}

// chooseMany asks for one or more options separated by commas; empty keeps defaults.
func (u *ui) chooseMany(title string, options []string, defaults []int) []int {
	u.listOptions(title, options)
	shown := make([]string, len(defaults))
	for i, d := range defaults {
		shown[i] = strconv.Itoa(d + 1)
	}
	for {
		answer, ok := u.readLine(fmt.Sprintf("Elige uno o varios separados por coma [%s]: ", strings.Join(shown, ",")))
		if !ok || answer == "" {
			return defaults
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
			return chosen
		}
		u.log("Opción no válida: " + answer)
	}
}

// confirm asks a yes/no question.
func (u *ui) confirm(question string, def bool) bool {
	hint := "[S/n]"
	if !def {
		hint = "[s/N]"
	}
	for {
		answer, ok := u.readLine(question + " " + hint + ": ")
		switch strings.ToLower(answer) {
		case "":
			return def
		case "s", "si", "sí", "y", "yes":
			return true
		case "n", "no":
			return false
		}
		if !ok {
			return def
		}
		u.log("Responde s o n")
	}
}

// text asks for free text; it may be empty.
func (u *ui) text(question string) string {
	answer, _ := u.readLine(question + ": ")
	return answer
}
