package cli

import (
	"errors"
	"fmt"
	"io"
	"os"
	"slices"

	"charm.land/huh/v2"

	"github.com/F4R1DS4NCH3Z26/basekit-go/internal/scaffold"
)

// errCancelled is returned when the user leaves a question (Ctrl+C or Esc):
// scaffold stops before writing anything.
var errCancelled = errors.New("cancelado: no se escribió nada")

// huhPrompter implements scaffold.Prompter with huh forms: arrow keys, checkboxes
// and inline help. It is used only on a real terminal; pipes, CI, tests and
// ACCESSIBLE=1 use the line-based terminal instead.
type huhPrompter struct {
	out io.Writer
}

func (h *huhPrompter) Log(line string) { fmt.Fprintln(h.out, line) }

func (h *huhPrompter) Interactive() bool { return true }

// run shows one field and maps an aborted form to errCancelled.
func run(field huh.Field) error {
	err := huh.NewForm(huh.NewGroup(field)).Run()
	if errors.Is(err, huh.ErrUserAborted) {
		return errCancelled
	}
	return err
}

// Select shows one step of a menu question with the arrow keys; typed answers only exist in the line-based mode.
func (h *huhPrompter) Select(question string, options []scaffold.Option, _ []string) (int, string, error) {
	choice := 0
	opts := make([]huh.Option[int], len(options))
	for i, line := range menuLines(options) {
		opts[i] = huh.NewOption(line, i)
	}
	err := run(huh.NewSelect[int]().Title(question).Options(opts...).Value(&choice))
	return choice, "", err
}

func (h *huhPrompter) ChooseMany(title string, options []string, defaults []int) ([]int, error) {
	var chosen []int
	opts := make([]huh.Option[int], len(options))
	for i, o := range options {
		opts[i] = huh.NewOption(o, i)
	}
	for _, d := range defaults {
		opts[d] = opts[d].Selected(true)
	}
	err := run(huh.NewMultiSelect[int]().
		Title(title).
		Description("Espacio marca o desmarca · Enter confirma").
		Options(opts...).
		Validate(func(v []int) error {
			if len(v) == 0 {
				return errors.New("elige al menos uno")
			}
			return nil
		}).
		Value(&chosen))
	slices.Sort(chosen) // callers rely on the options' order, not the toggling order
	return chosen, err
}

func (h *huhPrompter) Confirm(question string, def bool) (bool, error) {
	answer := def
	err := run(huh.NewConfirm().Title(question).Affirmative("Sí").Negative("No").Value(&answer))
	return answer, err
}

func (h *huhPrompter) Text(question string) (string, error) {
	var answer string
	err := run(huh.NewInput().Title(question).Value(&answer))
	return answer, err
}

// useHuh tells whether the rich prompts can run: a real terminal on stdin and no
// request for the accessible, line-based mode.
func useHuh(stdin io.Reader, interactive bool) bool {
	f, ok := stdin.(*os.File)
	return interactive && ok && f == os.Stdin && os.Getenv("ACCESSIBLE") == ""
}
