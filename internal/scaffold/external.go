package scaffold

import (
	"fmt"
	"strings"

	"github.com/F4R1DS4NCH3Z26/basekit-go/internal/fsx"
	"github.com/F4R1DS4NCH3Z26/basekit-go/internal/manifest"
)

var telemetryOff = []string{"DISABLE_TELEMETRY=1", "DO_NOT_TRACK=1"}

// externalArgs builds the pinned `npx skills add` call; global adds -g, which
// installs into ~/.agents/skills and links each tool's global folder.
func externalArgs(agents []string, global bool) []string {
	args := []string{"-y", manifest.ExternalCLI, "add", manifest.ExternalSource + "/tree/" + manifest.ExternalRef, "-s"}
	args = append(args, manifest.ExternalNames...)
	args = append(args, "-a")
	args = append(args, agents...)
	if global {
		args = append(args, "-g")
	}
	return append(args, "-y")
}

func missingExternal(p *fsx.Project) []string {
	var missing []string
	for _, s := range manifest.ExternalNames {
		if !p.Exists(".agents/skills/" + s + "/SKILL.md") {
			missing = append(missing, s)
		}
	}
	return missing
}

// installExternalSkills installs the pinned external skills (spec 02 §9) when any
// is missing, or for new agents when reinstall is set. A failure never aborts:
// it prints the command to retry.
func installExternalSkills(p *fsx.Project, agents []string, log func(string), reinstall, global bool) {
	names := strings.Join(manifest.ExternalNames, ", ")
	missing := missingExternal(p)
	if len(missing) == 0 && !reinstall {
		p.Report(fsx.Kept, "skills externas", names)
		return
	}
	args := externalArgs(agents, global)
	if p.DryRun {
		log("· ejecutaría: npx " + strings.Join(args, " "))
		return
	}
	ok := runCmd(log, p.Root, telemetryOff, "npx", args...)
	still := missingExternal(p)
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
