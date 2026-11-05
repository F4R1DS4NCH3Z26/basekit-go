package basekit

import (
	"fmt"
	"slices"
	"strings"
)

// runSync implements spec 02 §10: it regenerates the AGENTS.md block, the adapters
// and the links from .agents/.
func runSync(o *options, u *ui) error {
	root, err := resolveDir(o.dir)
	if err != nil {
		return err
	}
	p := newProject(root, o.flag("dry-run"), u.log)
	prev, err := readManifest(p)
	if err != nil {
		return err
	}
	if prev.str("stack") == "" {
		return fmt.Errorf("no existe %s en %s: ejecuta `basekit init` primero", manifestPath, root)
	}
	stacks, err := LoadStacks()
	if err != nil {
		return err
	}
	stack, err := pickStack(stacks, prev.str("stack"))
	if err != nil {
		return err
	}
	saved := prev.strings("agents")
	agents := []string{}
	if raw, ok := o.str("agents"); ok {
		if agents, err = parseAgents(raw); err != nil {
			return err
		}
	} else if len(saved) > 0 {
		if agents, err = parseAgents(strings.Join(saved, ",")); err != nil {
			return err
		}
	}
	added := slices.ContainsFunc(agents, func(a string) bool { return !slices.Contains(saved, a) })
	if p.DryRun {
		u.log("(dry-run: no se escribe nada en " + root + ")")
	}

	writeAgentsMd(p, stack, "")
	writeAdapters(p, stack, agents)
	if !o.flag("no-external-skills") && len(agents) > 0 {
		installExternalSkills(p, agents, u.log, added)
	}
	linkSkills(p, stack, agents)

	db, _ := prev.option("db").(string)
	if db == "" {
		db = "auto"
	}
	redis, _ := prev.option("redis").(bool)
	docker, ok := prev.option("docker").(bool)
	writeManifest(p, prev, manifestData{stack.ID, agents, db, redis, docker || !ok, prev.files()})
	if err := p.Err(); err != nil {
		return err
	}

	u.log("")
	if p.Changed {
		u.log("Sincronizado.")
	} else {
		u.log("Todo al día: no había nada que cambiar.")
	}
	return nil
}
