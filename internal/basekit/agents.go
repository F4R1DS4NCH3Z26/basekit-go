package basekit

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
)

// agent is a supported tool (guide 03 §1). detect lists the project paths that
// reveal it; skillsDir is set only for tools that do not read .agents/skills/ natively.
type agent struct {
	id        string
	detect    []string
	skillsDir string
}

var agentList = []agent{
	{"claude-code", []string{".claude", "CLAUDE.md"}, ".claude/skills"},
	{"opencode", []string{".opencode", "opencode.json"}, ""},
	{"codex", nil, ""},
	{"cursor", []string{".cursor"}, ""},
	{"gemini-cli", []string{".gemini"}, ""},
	{"github-copilot", nil, ""},
	{"hermes-agent", []string{".hermes"}, ".hermes/skills"},
}

var defaultAgents = []string{"claude-code", "opencode"}

func agentIDs() []string {
	ids := make([]string, len(agentList))
	for i, a := range agentList {
		ids[i] = a.id
	}
	return ids
}

func detectAgents(root string) []string {
	var found []string
	for _, a := range agentList {
		for _, p := range a.detect {
			if _, err := os.Stat(filepath.Join(root, p)); err == nil {
				found = append(found, a.id)
				break
			}
		}
	}
	return found
}

// parseAgents validates a comma-separated list against the whitelist and returns
// it in canonical order.
func parseAgents(raw string) ([]string, error) {
	var ids, bad []string
	for _, s := range strings.Split(raw, ",") {
		if s = strings.TrimSpace(s); s != "" {
			ids = append(ids, s)
			if !slices.Contains(agentIDs(), s) {
				bad = append(bad, s)
			}
		}
	}
	if len(ids) == 0 || len(bad) > 0 {
		shown := strings.Join(bad, ",")
		if shown == "" {
			shown = raw
		}
		return nil, usageErr("--agents '%s' no es un agente soportado. Soportados: %s", shown, strings.Join(agentIDs(), ", "))
	}
	var out []string
	for _, id := range agentIDs() {
		if slices.Contains(ids, id) {
			out = append(out, id)
		}
	}
	return out, nil
}
