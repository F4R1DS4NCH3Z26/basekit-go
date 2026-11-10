package scaffold

import (
	"os"
	"path"
	"slices"
	"strings"

	"github.com/F4R1DS4NCH3Z26/basekit-go/internal/adapters"
	"github.com/F4R1DS4NCH3Z26/basekit-go/internal/fsx"
	"github.com/F4R1DS4NCH3Z26/basekit-go/internal/manifest"
	"github.com/F4R1DS4NCH3Z26/basekit-go/internal/stack"
)

// GlobalOptions are the explicit choices of a global install.
type GlobalOptions struct {
	Agents *string
	// ExternalSkills is --external-skills: the skills for this run only (the global manifest keeps no options).
	ExternalSkills    *string
	NoExternal, Force bool
	DryRun            bool
}

// InitGlobal installs the reusable part of the kit for the user, in every project:
// the stack-agnostic skills go to ~/.agents/skills (the same canonical folder the
// skills CLI uses with -g) and each chosen tool's global folder links to them.
// Anything tied to a project (AGENTS.md, roles, stack skills, Docker, docs) stays
// with `basekit init` inside each project.
func InitGlobal(o GlobalOptions, ui Prompter) error {
	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}
	var flagAgents []string
	if o.Agents != nil {
		if flagAgents, err = adapters.Parse(*o.Agents); err != nil {
			return err
		}
	}
	flagExternal, err := parseExternal(o.ExternalSkills, o.NoExternal)
	if err != nil {
		return err
	}
	p := fsx.New(home, o.DryRun, ui.Log)
	prev, err := manifest.ReadAt(p, manifest.GlobalPath)
	if err != nil {
		return err
	}
	skills := stack.Walk("skills")
	names := stack.CoreSkills()

	// Q3 · Skills, then Q4 · agents: the only questions that make sense user-wide.
	// The global manifest keeps no options: the list is asked whenever a skill is missing, unless
	// --external-skills answers it for this run.
	externalNames := manifest.ExternalNames
	if flagExternal != nil {
		externalNames = flagExternal
	} else if !o.NoExternal && ui.Interactive() && len(missingExternal(p, externalNames)) > 0 {
		ui.Log("Skills incluidas: " + strings.Join(names, ", "))
		if externalNames, err = askExternal(ui); err != nil {
			return err
		}
	}
	agents, err := savedAgents(flagAgents, prev)
	if err != nil {
		return err
	}
	if agents == nil && ui.Interactive() {
		if agents, err = askAgents(ui); err != nil {
			return err
		}
	}
	if agents == nil {
		agents = adapters.Defaults
	}
	if ui.Interactive() {
		ui.Log("")
	}
	if o.DryRun {
		ui.Log("(dry-run: no se escribe nada en " + home + ")")
	}

	files := map[string]string{}
	for _, f := range skills {
		p.Tracked(path.Join(adapters.CanonicalSkillsDir, f), stack.Read(path.Join("skills", f)), stack.ModeOf(f), prev.Files(), files, o.Force)
	}
	if !o.NoExternal {
		installExternalSkills(p, externalNames, agents, ui.Log, false, true)
	}
	// Links last, so they also cover the external skills. Only the skills basekit
	// owns are linked: the user's own skills in ~/.agents/skills are not ours.
	linked := names
	for _, n := range manifest.ExternalNames {
		if p.Exists(path.Join(adapters.CanonicalSkillsDir, n)) && !slices.Contains(linked, n) {
			linked = append(linked, n)
		}
	}
	adapters.LinkGlobalSkills(p, linked, agents)

	manifest.Write(p, prev, manifest.Data{Global: true, Version: Version, Agents: agents, Files: files})
	if err := p.Err(); err != nil {
		return err
	}
	ui.Log("")
	switch {
	case o.DryRun:
		ui.Log("Dry-run: no se escribió nada; arriba está lo que haría.")
	case p.Changed:
		forAgents := ""
		if len(agents) > 0 {
			forAgents = " para " + strings.Join(agents, ", ")
		}
		ui.Log("Listo: skills globales instaladas en " + path.Join(home, adapters.CanonicalSkillsDir) + forAgents + ".")
	default:
		ui.Log("Todo al día: no había nada que cambiar.")
	}
	ui.Log("En cada proyecto, ejecuta `basekit init` para lo específico del stack (AGENTS.md, roles, Docker y docs).")
	return nil
}
