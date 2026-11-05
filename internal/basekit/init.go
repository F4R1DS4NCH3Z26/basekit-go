package basekit

import (
	"os"
	"path"
	"path/filepath"
	"slices"
	"strings"
)

var dbs = []string{"auto", "postgres", "mysql", "sqlite", "none"}

const infraScript = ".agents/skills/infra-setup/scripts/infra.sh"

// resolveDir expands ~ and returns the absolute project root.
func resolveDir(dir string) (string, error) {
	if dir == "" {
		dir = "."
	}
	if dir == "~" || strings.HasPrefix(dir, "~/") {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		dir = filepath.Join(home, strings.TrimPrefix(dir[1:], "/"))
	}
	return filepath.Abs(dir)
}

func checkDB(db string) error {
	if !slices.Contains(dbs, db) {
		return usageErr("--db '%s' no es válido. Valores: %s", db, strings.Join(dbs, ", "))
	}
	return nil
}

func isDir(p string) bool {
	info, err := os.Stat(p)
	return err == nil && info.IsDir()
}

func nextStep(u *ui, p *Project, s Stack, context string, docker bool) {
	u.log("")
	if p.Changed {
		u.log("Listo: base de " + s.Name + " preparada en " + p.Root)
	} else {
		u.log("Todo al día: no había nada que cambiar.")
	}
	if context == "" {
		context = "<una o dos frases>"
	}
	u.log("Próximo paso, dile a tu agente:")
	u.log("  «Lee AGENTS.md y ejecuta el bootstrap. El proyecto trata de: " + context + "»")
	marker := s.AppMarker
	fromStarter := marker != "" && slices.Contains(walk(path.Join(stackDir(s), "starter")), marker)
	switch {
	case marker == "" || p.exists(marker) || fromStarter:
	case docker:
		u.log("Falta la app del framework: ejecuta `make new-app` (corre dentro de Docker).")
	default:
		u.log("Falta la app del framework: créala con las herramientas del stack (" + marker + ").")
	}
}

// runInit implements spec 02 §3 step by step.
func runInit(o *options, u *ui) error {
	root, err := resolveDir(o.dir)
	if err != nil {
		return err
	}
	stacks, err := LoadStacks()
	if err != nil {
		return err
	}
	// Validate every flag before touching the disk.
	if id, ok := o.str("stack"); ok {
		if _, err := pickStack(stacks, id); err != nil {
			return err
		}
	}
	var flagAgents []string
	if raw, ok := o.str("agents"); ok {
		if flagAgents, err = parseAgents(raw); err != nil {
			return err
		}
	}
	dbFlag, hasDB := o.str("db")
	if hasDB {
		if err := checkDB(dbFlag); err != nil {
			return err
		}
	}

	dryRun := o.flag("dry-run")
	p := newProject(root, dryRun, u.log)
	rootExists := isDir(root)
	var prev Manifest
	if rootExists {
		if prev, err = readManifest(p); err != nil {
			return err
		}
	}

	// The questions follow what the project needs: the language narrows the
	// templates, the template decides the skills, the agents decide the adapters.
	// Each one is skipped when a flag, the manifest or detection answers it.

	// Q1–Q2 · Language and template (stack): flag → manifest → detection → questions.
	stackID, _ := o.str("stack")
	if stackID == "" {
		stackID = prev.str("stack")
	}
	if stackID == "" && rootExists {
		if s := detectStack(stacks, root); s != nil {
			stackID = s.ID
		}
	}
	if stackID == "" {
		if !u.interactive {
			return usageErr("No se pudo detectar el stack. Usa --stack <id> (disponibles: %s)", stackIDs(stacks))
		}
		s, err := askStack(u, stacks)
		if err != nil {
			return err
		}
		stackID = s.ID
	}
	stack, err := pickStack(stacks, strings.TrimSpace(stackID))
	if err != nil {
		return err
	}

	// Q3 · Skills: the kit and stack skills are always installed because the roles
	// and `make verify` depend on them; only the external pack is optional.
	external := !o.flag("no-external-skills")
	if external && u.interactive && len(missingExternal(p)) > 0 {
		u.log("Skills incluidas: " + strings.Join(kitSkills(stack), ", "))
		external = u.confirm("¿Instalar también las skills externas ("+strings.Join(externalNames, ", ")+")? Usan npx y red", true)
	}

	// Q4 · Agents: flag → manifest → detection → question → default.
	agents := flagAgents
	if saved := prev.strings("agents"); agents == nil && len(saved) > 0 {
		if agents, err = parseAgents(strings.Join(saved, ",")); err != nil {
			return err
		}
	}
	if agents == nil && rootExists {
		agents = detectAgents(root)
	}
	if len(agents) == 0 && u.interactive {
		agents = askAgents(u)
	}
	if len(agents) == 0 {
		agents = defaultAgents
	}

	// Q5 · Docker + Makefile: flag → manifest → question.
	docker, decided := prev.option("docker").(bool)
	switch {
	case o.flag("no-docker"):
		docker = false
	case !decided && u.interactive:
		docker = u.confirm("¿Usar Docker + Makefile? Todo corre en contenedores y el Makefile es la interfaz", true)
	case !decided:
		docker = true
	}
	if !docker && ((hasDB && dbFlag != "auto" && dbFlag != "none") || o.flag("redis")) {
		return usageErr("--db y --redis necesitan Docker: no se pueden usar con --no-docker ni sin Docker en el manifest")
	}

	// Q6 · Git: only asked when .git is missing (zone-check needs it).
	gitInit := !p.exists(".git") && !o.flag("no-git")
	if gitInit && u.interactive {
		gitInit = u.confirm("¿Inicializar git (git init)? zone-check lo necesita", true)
	}

	// Q7 · Context: flag → question on the first run.
	context, hasContext := o.str("context")
	if !hasContext && prev == nil && u.interactive {
		context = u.text("¿De qué trata el proyecto? (puede ir vacío)")
	}
	context = strings.TrimSpace(context)

	db := dbFlag
	if !hasDB {
		db, _ = prev.option("db").(string)
	}
	if db == "" {
		db = "auto"
	}
	if err := checkDB(db); err != nil {
		return err
	}
	redis := o.flag("redis")
	if saved, ok := prev.option("redis").(bool); ok && !redis {
		redis = saved
	}

	if u.interactive {
		u.log("") // separates the answers from the report
	}
	if dryRun {
		u.log("(dry-run: no se escribe nada en " + root + ")")
	} else if err := os.MkdirAll(root, 0o755); err != nil {
		return err
	}

	// 5 · Git (zone-check needs it).
	switch {
	case p.exists(".git"):
	case gitInit:
		p.report(created, ".git", "git init")
		if !dryRun {
			runCmd(u.log, root, nil, "git", "init", "-q")
		}
	default:
		p.report(warned, ".git", "sin git: zone-check no funciona hasta que ejecutes git init")
	}

	sdir := stackDir(stack)
	prevFiles := prev.files()
	files := map[string]string{}

	// 6 · .agents/ core.
	p.create(".agents/README.md", tpl("core/agents-README.md"), 0)
	for _, f := range walk(path.Join(sdir, "roles")) {
		p.create(".agents/roles/"+f, tpl(path.Join(sdir, "roles", f)), 0)
	}
	for _, f := range walk(path.Join(sdir, "zones")) {
		if f != "orchestrator.stack.txt" {
			p.create(".agents/zones/"+f, tpl(path.Join(sdir, "zones", f)), 0)
		}
	}
	common := tpl("core/zones.orchestrator.common.txt")
	stackZones, _ := tplOptional(path.Join(sdir, "zones/orchestrator.stack.txt"))
	if common != "" && !strings.HasSuffix(common, "\n") {
		common += "\n"
	}
	p.create(".agents/zones/orchestrator.txt", common+stackZones, 0)
	for _, skillsRoot := range []string{"skills", path.Join(sdir, "skills")} {
		for _, f := range walk(skillsRoot) {
			p.tracked(".agents/skills/"+f, tpl(path.Join(skillsRoot, f)), modeOf(f), prevFiles, files, o.flag("force"))
		}
	}

	// 7 · AGENTS.md managed block.
	writeAgentsMd(p, stack, context)
	for _, f := range []string{".hermes.md", "HERMES.md"} {
		if p.exists(f) {
			p.report(warned, f, "Hermes lo carga en lugar de AGENTS.md; bórralo o fusiónalo")
		}
	}

	// 8 · Docker and Make.
	appMissing := stack.AppMarker != "" && !p.exists(stack.AppMarker)
	if docker {
		makefile := "Makefile"
		if own, ok := p.read("Makefile"); ok && !strings.Contains(own, "generado por basekit") {
			makefile = "Makefile.basekit"
		}
		mark := p.tracked(makefile, tpl("core/Makefile"), 0o644, prevFiles, files, o.flag("force"))
		if makefile == "Makefile.basekit" && (mark == created || mark == updated) {
			p.report(warned, "Makefile", "ya existe uno propio: el de basekit queda en Makefile.basekit; intégralos a mano")
		}
		for _, f := range walk(path.Join(sdir, "files")) {
			p.create(f, tpl(path.Join(sdir, "files", f)), modeOf(f))
		}
	} else {
		p.report(warned, "Makefile", "sin Docker: no se crean Makefile, Dockerfile ni compose; las reglas de AGENTS.md asumen Docker y make, adáptalas")
	}
	if appMissing {
		for _, f := range walk(path.Join(sdir, "starter")) {
			p.create(f, tpl(path.Join(sdir, "starter", f)), modeOf(f))
		}
	}
	p.block(".gitignore", tpl("core/gitignore.block"), func(l string) bool { return strings.TrimSpace(l) == "# >>> basekit" }, "# <<< basekit")

	// 9 · Documentation (GLOSSARY.md and docs/adr/ are created later by domain-modeling).
	vars := templateVars(stack, root, context)
	p.create("README.md", render(tpl("core/README.tpl.md"), vars), 0)
	for _, f := range walk("core/docs") {
		text := tpl("core/docs/" + f)
		if strings.HasSuffix(f, ".md") {
			text = render(text, vars)
		}
		p.create("docs/"+f, text, 0)
	}

	// 10 · Adapters. 12 · External skills. 11 · Skill links last, so they also cover the external skills.
	writeAdapters(p, stack, agents)
	if external {
		installExternalSkills(p, agents, u.log, false)
	}
	linkSkills(p, stack, agents)

	// 13 · Infrastructure, through the same script the agent uses. Only when --db is
	// passed: a rerun that takes the db from the manifest must not touch it again.
	if docker && hasDB && db != "auto" {
		args := []string{infraScript, "--db", db}
		if redis {
			args = append(args, "--redis")
		}
		if dryRun {
			u.log("· ejecutaría: bash " + strings.Join(args, " "))
		} else if p.Err() == nil && !runCmd(u.log, root, nil, "bash", args...) {
			p.report(warned, infraScript, "falló; reinténtalo con: bash "+strings.Join(args, " "))
		}
	}

	writeManifest(p, prev, manifestData{stack.ID, agents, db, redis, docker, files})
	if err := p.Err(); err != nil {
		return err
	}

	// 14 · Summary and next step.
	nextStep(u, p, stack, context, docker)
	return nil
}

// askStack asks the language first and then the template among that language's
// stacks; with a single template there is nothing to ask.
func askStack(u *ui, stacks []Stack) (Stack, error) {
	var langs []string
	for _, s := range stacks {
		if !slices.Contains(langs, s.Language) {
			langs = append(langs, s.Language)
		}
	}
	slices.Sort(langs)
	i, err := u.choose("Lenguaje", langs)
	if err != nil {
		return Stack{}, err
	}
	var candidates []Stack
	var labels []string
	for _, s := range stacks {
		if s.Language == langs[i] {
			candidates = append(candidates, s)
			labels = append(labels, s.ID+" — "+s.Name)
		}
	}
	if len(candidates) == 1 {
		u.log("Template: " + labels[0])
		return candidates[0], nil
	}
	j, err := u.choose("Template", labels)
	if err != nil {
		return Stack{}, err
	}
	return candidates[j], nil
}

func askAgents(u *ui) []string {
	ids := agentIDs()
	var defaults []int
	for i, id := range ids {
		if slices.Contains(defaultAgents, id) {
			defaults = append(defaults, i)
		}
	}
	var agents []string
	for _, i := range u.chooseMany("Agentes", ids, defaults) {
		agents = append(agents, ids[i])
	}
	return agents
}
