// Package adapters knows the supported AI tools and writes their thin adapters
// (guide 03 §3), derived from .agents/roles and .agents/zones.
package adapters

import (
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/F4R1DS4NCH3Z26/basekit-go/internal/usage"
)

// Agent is a supported tool (guide 03 §1). Detect lists the project paths that
// reveal it; SkillsDir is set only for tools that do not read .agents/skills/ natively.
// GlobalSkillsDir is where the tool reads user-wide skills, relative to the home
// directory (same table as the skills CLI, docs/agents.md of skills@1.7.4).
type Agent struct {
	ID              string
	Detect          []string
	SkillsDir       string
	GlobalSkillsDir string
}

// All is the whitelist of supported tools, in canonical order.
var All = []Agent{
	{"claude-code", []string{".claude", "CLAUDE.md"}, ".claude/skills", ".claude/skills"},
	{"opencode", []string{".opencode", "opencode.json"}, "", ".config/opencode/skills"},
	{"codex", nil, "", CanonicalSkillsDir},
	{"cursor", []string{".cursor"}, "", ".cursor/skills"},
	{"gemini-cli", []string{".gemini"}, "", ".gemini/skills"},
	{"github-copilot", nil, "", ".copilot/skills"},
	{"hermes-agent", []string{".hermes"}, ".hermes/skills", ".hermes/skills"},
}

// CanonicalSkillsDir holds the single copy of every skill, in a project or in the
// home directory; the tools' folders link to it.
const CanonicalSkillsDir = ".agents/skills"

// Defaults are the tools used when nothing else decides.
var Defaults = []string{"claude-code", "opencode"}

// IDs returns the supported tool ids in canonical order.
func IDs() []string {
	ids := make([]string, len(All))
	for i, a := range All {
		ids[i] = a.ID
	}
	return ids
}

// Detect returns the tools whose folders or files exist in root.
func Detect(root string) []string {
	var found []string
	for _, a := range All {
		for _, p := range a.Detect {
			if _, err := os.Stat(filepath.Join(root, p)); err == nil {
				found = append(found, a.ID)
				break
			}
		}
	}
	return found
}

// Parse validates a comma-separated list against the whitelist and returns it in
// canonical order.
func Parse(raw string) ([]string, error) {
	var ids, bad []string
	for _, s := range strings.Split(raw, ",") {
		if s = strings.TrimSpace(s); s != "" {
			ids = append(ids, s)
			if !slices.Contains(IDs(), s) {
				bad = append(bad, s)
			}
		}
	}
	if len(ids) == 0 || len(bad) > 0 {
		shown := strings.Join(bad, ",")
		if shown == "" {
			shown = raw
		}
		return nil, usage.Errorf("--agents '%s' no es un agente soportado. Soportados: %s", shown, strings.Join(IDs(), ", "))
	}
	var out []string
	for _, id := range IDs() {
		if slices.Contains(ids, id) {
			out = append(out, id)
		}
	}
	return out, nil
}
