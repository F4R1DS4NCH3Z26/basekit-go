package scaffold

import (
	"fmt"
	"path"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/F4R1DS4NCH3Z26/basekit-go/internal/fsx"
	"github.com/F4R1DS4NCH3Z26/basekit-go/internal/stack"
)

// agentsMdWarn is the size from which AGENTS.md gets close to Hermes' 20,000 limit.
const agentsMdWarn = 18_000

// templateVars are the placeholder values (spec 02 §5). context is free text: it is
// only ever written to files.
func templateVars(s stack.Stack, root, context string) map[string]string {
	rows := make([]string, len(s.Skills))
	for i, r := range s.Skills {
		rows[i] = fmt.Sprintf("| `%s` | %s |", r[0], r[1])
	}
	section, _ := stack.ReadOptional(path.Join(stack.Dir(s), "AGENTS.section.md"))
	oneLiner, ctx := context, context
	if context == "" {
		oneLiner = "Descripción pendiente: la completa el bootstrap."
		ctx = "_(sin contexto inicial: el bootstrap lo pedirá)_"
	}
	return map[string]string{
		"VERSION":       Version,
		"STACK_NAME":    s.Name,
		"STACK_SKILLS":  strings.Join(rows, "\n"),
		"STACK_SECTION": strings.TrimRight(section, "\n"),
		"PROJECT_NAME":  filepath.Base(root),
		"ONE_LINER":     oneLiner,
		"CONTEXT":       ctx,
		"PORT":          strconv.Itoa(s.Port),
	}
}

// writeAgentsMd writes the managed block of AGENTS.md and warns when the file gets
// close to Hermes' limit.
func writeAgentsMd(p *fsx.Project, s stack.Stack, context string) {
	block := stack.Render(stack.Read("core/AGENTS.block.md"), templateVars(s, p.Root, context))
	text := p.Block("AGENTS.md", block, func(l string) bool { return strings.HasPrefix(l, "<!-- basekit:start") }, "<!-- basekit:end -->")
	if n := len([]rune(text)); n > agentsMdWarn {
		p.Report(fsx.Warned, "AGENTS.md", fmt.Sprintf("%d caracteres: Hermes trunca a partir de 20.000", n))
	}
}
