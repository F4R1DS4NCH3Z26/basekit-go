package scaffold

import (
	"fmt"
	"slices"
	"strings"

	"github.com/F4R1DS4NCH3Z26/basekit-go/internal/adapters"
	"github.com/F4R1DS4NCH3Z26/basekit-go/internal/fsx"
	"github.com/F4R1DS4NCH3Z26/basekit-go/internal/manifest"
)

var telemetryOff = []string{"DISABLE_TELEMETRY=1", "DO_NOT_TRACK=1"}

// externalArgs builds the pinned `npx skills add` call for the chosen skills; global adds -g, which
// installs into ~/.agents/skills and links each tool's global folder.
func externalArgs(names, agents []string, global bool) []string {
	args := []string{"-y", manifest.ExternalCLI, "add", manifest.ExternalSource + "/tree/" + manifest.ExternalRef, "-s"}
	args = append(args, names...)
	args = append(args, "-a")
	args = append(args, agents...)
	if global {
		args = append(args, "-g")
	}
	return append(args, "-y")
}

// chosenExternal is the user's choice of external skills stored in the manifest (options.externalSkills), in the
// pinned order; without it, all of them. decided tells whether the manifest holds a choice.
func chosenExternal(m manifest.Manifest) (names []string, decided bool) {
	saved, ok := m.OptionStrings("externalSkills")
	if !ok {
		return manifest.ExternalNames, false
	}
	names = []string{}
	for _, n := range manifest.ExternalNames {
		if slices.Contains(saved, n) {
			names = append(names, n)
		}
	}
	return names, true
}

// askExternal asks which external skills to install, all chosen by default; it may return none.
func askExternal(ui Prompter) ([]string, error) {
	all := manifest.ExternalNames
	defaults := make([]int, len(all))
	for i := range all {
		defaults[i] = i
	}
	chosen, err := ui.ChooseNames("¿Qué skills externas instalo? (usan npx y red)", all, defaults)
	if err != nil {
		return nil, err
	}
	names := []string{}
	for _, i := range chosen {
		names = append(names, all[i])
	}
	return names, nil
}

// missingExternal lists which of names are not installed in the project.
func missingExternal(p *fsx.Project, names []string) []string {
	var missing []string
	for _, s := range names {
		if !p.Exists(".agents/skills/" + s + "/SKILL.md") {
			missing = append(missing, s)
		}
	}
	return missing
}

// installExternalSkills installs the chosen pinned external skills (spec 02 §9) when any
// is missing, or for new agents when reinstall is set. A failure never aborts:
// it prints the command to retry. Without agents nothing is installed: the skills CLI needs -a.
func installExternalSkills(p *fsx.Project, chosen, agents []string, log func(string), reinstall, global bool) {
	if len(chosen) == 0 {
		p.Report(fsx.Kept, "skills externas", "ninguna elegida")
		return
	}
	names := strings.Join(chosen, ", ")
	missing := missingExternal(p, chosen)
	if len(missing) == 0 && !reinstall {
		p.Report(fsx.Kept, "skills externas", names)
		return
	}
	if len(agents) == 0 {
		command := "basekit sync --agents <a,b>"
		if global {
			command = "basekit init --global --agents <a,b>"
		}
		p.Report(fsx.Warned, "skills externas", "sin agentes: se instalan cuando agregues uno con "+command)
		return
	}
	if !global {
		if escaping := escapingSkillDirs(p, agents); len(escaping) > 0 {
			p.Report(fsx.Warned, "skills externas", fmt.Sprintf("no se instalan: %s está fuera del proyecto por un enlace simbólico", strings.Join(escaping, ", ")))
			return
		}
	}
	args := externalArgs(chosen, agents, global)
	if p.DryRun {
		log("· ejecutaría: npx " + strings.Join(args, " "))
		return
	}
	ok := runCmd(log, p.Root, telemetryOff, "npx", args...)
	still := missingExternal(p, chosen)
	switch {
	case ok && len(still) == 0 && len(missing) > 0:
		p.Report(fsx.Created, "skills externas", names)
	case ok && len(still) == 0:
		p.Report(fsx.Updated, "skills externas", names)
	default:
		which := ""
		if len(still) > 0 {
			which = fmt.Sprintf(" (%s)", strings.Join(still, ", "))
		}
		retry := strings.Join(telemetryOff, " ") + " npx " + strings.Join(args, " ")
		p.Report(fsx.Warned, "skills externas", fmt.Sprintf("no se instalaron%s; reinténtalo con: %s", which, retry))
	}
}

// escapingSkillDirs lists the folders `npx skills` writes into that lead outside
// the project through a symlink: the tool writes there itself, so basekit's own
// guard on writes cannot protect them. Global installs skip this check, because
// a home folder linked to dotfiles is a normal setup.
func escapingSkillDirs(p *fsx.Project, agents []string) []string {
	targets := []string{adapters.CanonicalSkillsDir}
	for _, a := range adapters.All {
		if a.SkillsDir != "" && slices.Contains(agents, a.ID) {
			targets = append(targets, a.SkillsDir)
		}
	}
	var escaping []string
	for _, t := range targets {
		if p.Escapes(t) {
			escaping = append(escaping, t)
		}
	}
	return escaping
}
