package cli

import (
	"bufio"
	"fmt"
	"io"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/F4R1DS4NCH3Z26/basekit-go/internal/scaffold"
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

var digits = regexp.MustCompile(`^\d+$`)

// optionIndex is the option an answer names: its number (from 1), its label or one of its keys, ignoring case;
// -1 if none.
func optionIndex(options []scaffold.Option, answer string) int {
	if digits.MatchString(answer) {
		if n, err := strconv.Atoi(answer); err == nil && n >= 1 && n <= len(options) {
			return n - 1
		}
		return -1
	}
	for i, o := range options {
		if strings.EqualFold(o.Label, answer) || slices.ContainsFunc(o.Keys, func(k string) bool { return strings.EqualFold(k, answer) }) {
			return i
		}
	}
	return -1
}

// menuLines are the options as a menu shows them: the labels padded to one width, each followed by its hint.
func menuLines(options []scaffold.Option) []string {
	width := 0
	for _, o := range options {
		width = max(width, utf8.RuneCountInString(o.Label))
	}
	lines := make([]string, len(options))
	for i, o := range options {
		lines[i] = o.Label
		if o.Hint != "" {
			lines[i] += strings.Repeat(" ", width-utf8.RuneCountInString(o.Label)) + "  " + o.Hint
		}
	}
	return lines
}

// Select asks one step of a menu question, numbered: a number, a label, a key or one of typed is accepted, an
// invalid answer asks again and the end of the input returns scaffold.ErrNoAnswer.
func (u *terminal) Select(question string, options []scaffold.Option, typed []string) (int, string, error) {
	u.Log(question)
	for i, line := range menuLines(options) {
		u.Log(fmt.Sprintf("  %d) %s", i+1, line))
	}
	for {
		answer, ok := u.readLine("Número o nombre: ")
		if !ok {
			return 0, "", scaffold.ErrNoAnswer
		}
		if i := optionIndex(options, answer); i >= 0 {
			return i, "", nil
		}
		for _, id := range typed {
			if strings.EqualFold(id, answer) {
				return -1, id, nil
			}
		}
		if answer == "" {
			u.Log("  Hace falta elegir una opción.")
		} else {
			u.Log("  «" + answer + "» no es una opción válida.")
		}
	}
}

// noneAnswers choose no option in a multiple choice, ignoring case.
var noneAnswers = []string{"ninguna", "ninguno"}

// ChooseMany asks for one or more options separated by commas; empty keeps defaults
// and `ninguna`/`ninguno` chooses none (an empty, non-nil list).
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
		if slices.ContainsFunc(noneAnswers, func(n string) bool { return strings.EqualFold(n, answer) }) {
			return []int{}, nil
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

// ChooseNames asks for names or numbers separated by commas, showing the defaults by name; empty or the end of the
// input keeps the defaults, `ninguna`/`ninguno` chooses none and an unknown answer is named and asked again.
func (u *terminal) ChooseNames(title string, names []string, defaults []int) ([]int, error) {
	u.Log(title)
	for i, n := range names {
		u.Log(fmt.Sprintf("  %d) %s", i+1, n))
	}
	shown := make([]string, len(defaults))
	for i, d := range defaults {
		shown[i] = names[d]
	}
	for {
		answer, ok := u.readLine("Números o nombres [" + strings.Join(shown, ",") + "]: ")
		if !ok || answer == "" {
			return defaults, nil
		}
		if slices.ContainsFunc(noneAnswers, func(n string) bool { return strings.EqualFold(n, answer) }) {
			return []int{}, nil
		}
		// Same rules as Node: every unknown part is named; only empty parts (",") choose none.
		chosen, unknown := []int{}, []string{}
		for _, part := range strings.Split(answer, ",") {
			part = strings.TrimSpace(part)
			if part == "" {
				continue
			}
			i := pick(part, names)
			if i < 0 {
				unknown = append(unknown, part)
				continue
			}
			if !slices.Contains(chosen, i) {
				chosen = append(chosen, i)
			}
		}
		if len(unknown) == 0 {
			slices.Sort(chosen)
			return chosen, nil
		}
		u.Log("  No reconozco: " + strings.Join(unknown, ", ") + ". Usa los números o los nombres de la lista.")
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
