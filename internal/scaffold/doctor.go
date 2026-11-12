package scaffold

import (
	"fmt"
	"path"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"github.com/F4R1DS4NCH3Z26/basekit-go/internal/adapters"
	"github.com/F4R1DS4NCH3Z26/basekit-go/internal/fsx"
	"github.com/F4R1DS4NCH3Z26/basekit-go/internal/manifest"
	"github.com/F4R1DS4NCH3Z26/basekit-go/internal/stack"
)

var nodeMajor = regexp.MustCompile(`^v?(\d+)\.`)

// Doctor implements spec 02 §11. It is read-only: it prints OK or FALTA with a
// suggestion and fails when anything is missing.
func Doctor(dir string, ui Prompter) (int, error) {
	root, err := resolveDir(dir)
	if err != nil {
		return 1, err
	}
	p := fsx.New(root, true, func(string) {})
	missing := 0
	check := func(ok bool, label, hint string) {
		if ok {
			ui.Log("OK    " + label)
			return
		}
		missing++
		ui.Log("FALTA " + label + "  → " + hint)
	}

	node := commandOutput("node", "--version")
	major := 0
	if m := nodeMajor.FindStringSubmatch(node); m != nil {
		major, _ = strconv.Atoi(m[1])
	}
	check(major >= 20, fmt.Sprintf("Node ≥ 20 (%s)", orNone(node)), "instala Node 20 o superior (lo usa npx skills)")
	check(works("git", "--version"), "git", "instala git")

	m, err := manifest.Read(p)
	if err != nil {
		return 1, err
	}
	docker, ok := m.Option("docker").(bool)
	docker = docker || !ok
	// Docker without the Makefile (S3): make, the Makefile and .make/ are not checked.
	makefile := docker && m.Option("makefile") != false
	if docker {
		check(works("docker", "--version"), "docker", "instala Docker")
		check(works("docker", "compose", "version"), "docker compose (plugin v2 o superior)", "instala el plugin Compose v2")
		if makefile {
			check(works("make", "--version"), "make", "instala GNU Make")
		}
		check(works("openssl", "version"), "openssl", "instala OpenSSL (lo usa make env)")
	}

	check(m != nil, manifest.Path, "ejecuta `basekit init`")
	if m == nil {
		return 1, nil
	}
	stacks, err := stack.Load()
	if err != nil {
		return 1, err
	}
	parts := stack.PartsOf(stacks, m.Str("stack"))
	if parts == nil {
		check(false, fmt.Sprintf("stack '%s'", m.Str("stack")), "el manifest apunta a un stack que esta versión no conoce")
		return 1, nil
	}

	agentsMd, _ := p.Read("AGENTS.md")
	size := len([]rune(agentsMd))
	hasBlock := strings.Contains(agentsMd, "<!-- basekit:start") && strings.Contains(agentsMd, "<!-- basekit:end -->")
	hint := "ejecuta `basekit sync`"
	if hasBlock {
		hint = "acórtalo: Hermes trunca a partir de 20.000"
	}
	check(hasBlock && size <= agentsMdWarn, fmt.Sprintf("AGENTS.md (bloque de basekit, %d caracteres)", size), hint)

	// A combined project (S16): the root Makefile delegates, and each side is a complete project of its stack.
	if makefile && stack.IsCombined(parts) {
		check(p.Exists("Makefile"), "Makefile (raíz)", "ejecuta `basekit init`")
	}
	if !docker {
		ui.Log("·     sin Docker (manifest): se omiten Docker, Makefile y .env")
	} else if !makefile {
		ui.Log("·     sin Makefile (manifest): se omiten make y el Makefile")
	}
	for _, part := range parts {
		if docker {
			for _, f := range []string{"Makefile", ".make/stack.mk", "Dockerfile", "compose.yaml", "compose.dev.yaml", "compose.prod.yaml", ".env.example"} {
				if !makefile && (f == "Makefile" || strings.HasPrefix(f, ".make/")) {
					continue
				}
				check(p.Exists(stack.InPart(part, f)), stack.InPart(part, f), "ejecuta `basekit init`")
			}
		}
		if marker := part.Stack.AppMarker; marker != "" {
			hint := "ejecuta `" + stack.NewAppCommand(part) + "`"
			if !makefile {
				hint = "crea la app con las herramientas del framework"
			}
			check(p.Exists(stack.InPart(part, marker)), stack.InPart(part, marker)+" (app del framework)", hint)
		}
	}

	for _, s := range stack.SkillsOf(parts) {
		f := path.Join(".agents/skills", s, "SKILL.md")
		check(p.Exists(f), f, "ejecuta `basekit init`")
	}
	for _, part := range parts {
		if part.Side == "" {
			continue
		}
		for _, s := range stack.StackSkills(part.Stack) {
			f := stack.InPart(part, path.Join(".agents/skills", s, "SKILL.md"))
			check(p.Exists(f), f, "ejecuta `basekit init` (make verify la lee dentro de Docker)")
		}
	}
	// Only the external skills the user chose, and none without agents: nothing could install them (they need -a).
	agents := m.Strings("agents")
	external, _ := chosenExternal(m)
	if len(agents) == 0 {
		external = nil
	}
	absent := missingExternal(p, external)
	for _, s := range external {
		check(!slices.Contains(absent, s), ".agents/skills/"+s+"/SKILL.md (externa)", "ejecuta `basekit sync`")
	}

	skills := p.Dirs(".agents/skills")
	for _, a := range adapters.All {
		if a.SkillsDir == "" || !slices.Contains(agents, a.ID) {
			continue
		}
		var unlinked []string
		for _, s := range skills {
			if !p.Exists(a.SkillsDir + "/" + s + "/SKILL.md") {
				unlinked = append(unlinked, s)
			}
		}
		check(len(unlinked) == 0, a.SkillsDir+" (enlaces de "+a.ID+")", "faltan "+strings.Join(unlinked, ", ")+": ejecuta `basekit sync`")
	}

	var hermes []string
	for _, f := range []string{".hermes.md", "HERMES.md"} {
		if p.Exists(f) {
			hermes = append(hermes, f)
		}
	}
	check(len(hermes) == 0, "Hermes: sin .hermes.md ni HERMES.md", "borra "+strings.Join(hermes, " y ")+": Hermes lo carga en lugar de AGENTS.md")
	if docker {
		// Only existence is checked: basekit never reads .env.
		for _, part := range parts {
			check(p.Exists(stack.InPart(part, ".env")), stack.InPart(part, ".env"), "ejecuta `make env`")
		}
	}
	if missing > 0 {
		return 1, nil
	}
	return 0, nil
}

func orNone(s string) string {
	if s == "" {
		return "no encontrado"
	}
	return s
}
