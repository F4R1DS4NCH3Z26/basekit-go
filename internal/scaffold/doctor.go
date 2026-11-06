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
	if docker {
		check(works("docker", "--version"), "docker", "instala Docker")
		check(works("docker", "compose", "version"), "docker compose (plugin v2 o superior)", "instala el plugin Compose v2")
		check(works("make", "--version"), "make", "instala GNU Make")
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
	st, err := stack.Pick(stacks, m.Str("stack"))
	if err != nil {
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

	if docker {
		for _, f := range []string{"Makefile", ".make/stack.mk", "Dockerfile", "compose.yaml", "compose.dev.yaml", "compose.prod.yaml", ".env.example"} {
			check(p.Exists(f), f, "ejecuta `basekit init`")
		}
	} else {
		ui.Log("·     sin Docker (manifest): se omiten Docker, Makefile y .env")
	}
	if st.AppMarker != "" {
		hint := "ejecuta `make new-app`"
		if !docker {
			hint = "crea la app con las herramientas del framework"
		}
		check(p.Exists(st.AppMarker), st.AppMarker+" (app del framework)", hint)
	}

	for _, s := range stack.KitSkills(st) {
		f := path.Join(".agents/skills", s, "SKILL.md")
		check(p.Exists(f), f, "ejecuta `basekit init`")
	}
	absent := missingExternal(p)
	for _, s := range manifest.ExternalNames {
		check(!slices.Contains(absent, s), ".agents/skills/"+s+"/SKILL.md (externa)", "ejecuta `basekit sync`")
	}

	skills := p.Dirs(".agents/skills")
	agents := m.Strings("agents")
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
		check(p.Exists(".env"), ".env", "ejecuta `make env`")
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
