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

// section is a stack's own AGENTS.md section, without trailing blank lines ("" when it has none).
func section(s stack.Stack) string {
	text, _ := stack.ReadOptional(path.Join(stack.Dir(s), "AGENTS.section.md"))
	return strings.TrimRight(text, "\n")
}

// templateVars are the placeholder values (spec 02 §5). context is free text: it is
// only ever written to files. A combined project (S16) lists the skills of both
// stacks and wraps both sections in core/combo/AGENTS.section.md; its README port
// is the frontend's, the page the user opens.
func templateVars(parts []stack.Part, root, context string) map[string]string {
	var rows []string
	var backend, frontend *stack.Part
	for i, p := range parts {
		for _, r := range p.Stack.Skills {
			rows = append(rows, fmt.Sprintf("| `%s` | %s |", r[0], r[1]))
		}
		switch p.Side {
		case "backend":
			backend = &parts[i]
		case "frontend":
			frontend = &parts[i]
		}
	}
	stackSection := section(parts[0].Stack)
	port := parts[0].Stack.Port
	if backend != nil && frontend != nil {
		stackSection = strings.TrimRight(stack.Render(stack.Read("core/combo/AGENTS.section.md"), map[string]string{
			"BACKEND_NAME":     backend.Stack.Name,
			"FRONTEND_NAME":    frontend.Stack.Name,
			"BACKEND_PORT":     strconv.Itoa(backend.Stack.Port),
			"FRONTEND_PORT":    strconv.Itoa(frontend.Stack.Port),
			"BACKEND_SECTION":  section(backend.Stack),
			"FRONTEND_SECTION": section(frontend.Stack),
		}), "\n")
		port = frontend.Stack.Port
	}
	oneLiner, ctx := context, context
	if context == "" {
		oneLiner = "Descripción pendiente: la completa el bootstrap."
		ctx = "_(sin contexto inicial: el bootstrap lo pedirá)_"
	}
	return map[string]string{
		"VERSION":       Version,
		"STACK_NAME":    stack.DisplayName(parts),
		"STACK_SKILLS":  strings.Join(rows, "\n"),
		"STACK_SECTION": stackSection,
		"PROJECT_NAME":  filepath.Base(root),
		"ONE_LINER":     oneLiner,
		"CONTEXT":       ctx,
		"PORT":          strconv.Itoa(port),
	}
}

// writeAgentsMd writes the managed block of AGENTS.md and warns when the file gets
// close to Hermes' limit.
func writeAgentsMd(p *fsx.Project, parts []stack.Part, context string) {
	block := stack.Render(stack.Read("core/AGENTS.block.md"), templateVars(parts, p.Root, context))
	text := p.Block("AGENTS.md", block, func(l string) bool { return strings.HasPrefix(l, "<!-- basekit:start") }, "<!-- basekit:end -->")
	if n := len([]rune(text)); n > agentsMdWarn {
		p.Report(fsx.Warned, "AGENTS.md", fmt.Sprintf("%d caracteres: Hermes trunca a partir de 20.000", n))
	}
}
