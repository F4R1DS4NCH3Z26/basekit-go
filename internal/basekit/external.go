package basekit

import (
	"fmt"
	"strings"
)

var telemetryOff = []string{"DISABLE_TELEMETRY=1", "DO_NOT_TRACK=1"}

func externalArgs(agents []string) []string {
	args := []string{"-y", externalCLI, "add", externalSource + "/tree/" + externalRef, "-s"}
	args = append(args, externalNames...)
	args = append(args, "-a")
	args = append(args, agents...)
	return append(args, "-y")
}

func missingExternal(p *Project) []string {
	var missing []string
	for _, s := range externalNames {
		if !p.exists(".agents/skills/" + s + "/SKILL.md") {
			missing = append(missing, s)
		}
	}
	return missing
}

// installExternalSkills installs the pinned external skills (spec 02 §9) when any
// is missing, or for new agents when reinstall is set. A failure never aborts:
// it prints the command to retry.
func installExternalSkills(p *Project, agents []string, log func(string), reinstall bool) {
	names := strings.Join(externalNames, ", ")
	missing := missingExternal(p)
	if len(missing) == 0 && !reinstall {
		p.report(kept, "skills externas", names)
		return
	}
	args := externalArgs(agents)
	if p.DryRun {
		log("· ejecutaría: npx " + strings.Join(args, " "))
		return
	}
	ok := runCmd(log, p.Root, telemetryOff, "npx", args...)
	still := missingExternal(p)
	switch {
	case ok && len(still) == 0 && len(missing) > 0:
		p.report(created, "skills externas", names)
	case ok && len(still) == 0:
		p.report(updated, "skills externas", names)
	default:
		which := ""
		if len(still) > 0 {
			which = fmt.Sprintf(" (%s)", strings.Join(still, ", "))
		}
		retry := strings.Join(telemetryOff, " ") + " npx " + strings.Join(args, " ")
		p.report(warned, "skills externas", fmt.Sprintf("no se instalaron%s; reinténtalo con: %s", which, retry))
	}
}
