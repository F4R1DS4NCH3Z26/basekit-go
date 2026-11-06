package scaffold

import (
	"fmt"
	"slices"
	"strings"

	"github.com/F4R1DS4NCH3Z26/basekit-go/internal/adapters"
	"github.com/F4R1DS4NCH3Z26/basekit-go/internal/fsx"
	"github.com/F4R1DS4NCH3Z26/basekit-go/internal/manifest"
	"github.com/F4R1DS4NCH3Z26/basekit-go/internal/stack"
)

// SyncOptions are the explicit choices of sync; a nil Agents keeps the manifest's.
type SyncOptions struct {
	Dir        string
	Agents     *string
	NoExternal bool
	DryRun     bool
}

// Sync implements spec 02 §10: it regenerates the AGENTS.md block, the adapters
// and the links from .agents/.
func Sync(o SyncOptions, ui Prompter) error {
	root, err := resolveDir(o.Dir)
	if err != nil {
		return err
	}
	p := fsx.New(root, o.DryRun, ui.Log)
	prev, err := manifest.Read(p)
	if err != nil {
		return err
	}
	if prev.Str("stack") == "" {
		return fmt.Errorf("no existe %s en %s: ejecuta `basekit init` primero", manifest.Path, root)
	}
	stacks, err := stack.Load()
	if err != nil {
		return err
	}
	st, err := stack.Pick(stacks, prev.Str("stack"))
	if err != nil {
		return err
	}
	saved := prev.Strings("agents")
	agents := []string{}
	if o.Agents != nil {
		if agents, err = adapters.Parse(*o.Agents); err != nil {
			return err
		}
	} else if len(saved) > 0 {
		if agents, err = adapters.Parse(strings.Join(saved, ",")); err != nil {
			return err
		}
	}
	added := slices.ContainsFunc(agents, func(a string) bool { return !slices.Contains(saved, a) })
	if p.DryRun {
		ui.Log("(dry-run: no se escribe nada en " + root + ")")
	}

	writeAgentsMd(p, st, "")
	adapters.Write(p, st, agents)
	if !o.NoExternal && len(agents) > 0 {
		installExternalSkills(p, agents, ui.Log, added, false)
	}
	adapters.LinkSkills(p, st, agents)

	db, _ := prev.Option("db").(string)
	if db == "" {
		db = "auto"
	}
	redis, _ := prev.Option("redis").(bool)
	docker, ok := prev.Option("docker").(bool)
	manifest.Write(p, prev, manifest.Data{Version: Version, Stack: st.ID, Agents: agents, DB: db, Redis: redis, Docker: docker || !ok, Files: prev.Files()})
	if err := p.Err(); err != nil {
		return err
	}

	ui.Log("")
	if p.Changed {
		ui.Log("Sincronizado.")
	} else {
		ui.Log("Todo al día: no había nada que cambiar.")
	}
	return nil
}
