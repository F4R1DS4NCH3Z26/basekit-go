// Package scaffold is the core of basekit: init, sync and doctor, and the
// questions init asks. It depends on interfaces it declares (Prompter), never on
// the terminal or the command line.
package scaffold

import (
	"errors"
	"os"
	"path"
	"path/filepath"
	"runtime/debug"
	"slices"
	"strings"

	"github.com/F4R1DS4NCH3Z26/basekit-go/internal/adapters"
	"github.com/F4R1DS4NCH3Z26/basekit-go/internal/fsx"
	"github.com/F4R1DS4NCH3Z26/basekit-go/internal/manifest"
	"github.com/F4R1DS4NCH3Z26/basekit-go/internal/stack"
	"github.com/F4R1DS4NCH3Z26/basekit-go/internal/usage"
)

// Version of basekit, rendered into the managed blocks and the manifest. Release
// builds set it from the git tag (-ldflags "-X <module>/internal/scaffold.Version=1.2.3");
// otherwise it comes from the module version (`go install …@v1.2.3`), and local
// builds fall back to defaultVersion.
var Version = ""

const defaultVersion = "0.1.0"

func init() {
	if Version != "" {
		return
	}
	Version = defaultVersion
	if bi, ok := debug.ReadBuildInfo(); ok && strings.HasPrefix(bi.Main.Version, "v") {
		Version = strings.TrimPrefix(bi.Main.Version, "v")
	}
}

// Prompter is what scaffold needs from whoever runs it: a log and, only when
// Interactive is true, answers to questions. An error (for example the user
// cancelling) stops the command before anything is written.
type Prompter interface {
	Log(line string)
	Interactive() bool
	// Select asks one step of a menu question and returns the chosen option's index. In the line-based mode an
	// answer may also be one of typed (ignoring case): then it returns -1 and that value. When the input ends
	// before a valid answer it returns ErrNoAnswer.
	Select(question string, options []Option, typed []string) (int, string, error)
	ChooseMany(title string, options []string, defaults []int) ([]int, error)
	Confirm(question string, def bool) (bool, error)
	Text(question string) (string, error)
}

// Option is one answer of a menu question: its label, a hint shown next to it and
// keys that are also accepted when typed.
type Option struct {
	Label, Hint string
	Keys        []string
}

// ErrNoAnswer is what Select returns when the input ends before a valid answer.
var ErrNoAnswer = errors.New("sin respuesta")

// InitOptions are the explicit choices. A nil pointer means "not given": the
// manifest, detection or a question decides.
type InitOptions struct {
	Dir                                string
	Stack, Agents, Context, DB         *string
	Redis, NoDocker, NoGit, NoExternal bool
	Force, DryRun                      bool
}

var dbs = []string{"auto", "postgres", "mysql", "sqlite", "none"}

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
		return usage.Errorf("--db '%s' no es válido. Valores: %s", db, strings.Join(dbs, ", "))
	}
	return nil
}

// checkStackDB rejects a database engine or Redis for a stack without a database (a frontend stack).
func checkStackDB(st stack.Stack, db *string, redis bool) error {
	if stack.HasDatabase(st) {
		return nil
	}
	if db != nil && *db != "auto" && *db != "none" {
		return usage.Errorf("--db '%s' no aplica a %s: un stack frontend no tiene base de datos (usa --db none o quita --db)", *db, st.ID)
	}
	if redis {
		return usage.Errorf("--redis no aplica a %s: un stack frontend no tiene base de datos ni Redis (quita --redis)", st.ID)
	}
	return nil
}

func nextStep(ui Prompter, p *fsx.Project, parts []stack.Part, context string, docker bool) {
	ui.Log("")
	switch {
	case p.DryRun:
		ui.Log("Dry-run: no se escribió nada; arriba está lo que haría.")
	case p.Changed:
		ui.Log("Listo: base de " + stack.DisplayName(parts) + " preparada en " + p.Root)
	default:
		ui.Log("Todo al día: no había nada que cambiar.")
	}
	if context == "" {
		context = "<una o dos frases>"
	}
	ui.Log("Próximo paso, dile a tu agente:")
	ui.Log("  «Lee AGENTS.md y ejecuta el bootstrap. El proyecto trata de: " + context + "»")
	for _, part := range parts {
		marker := part.Stack.AppMarker
		fromStarter := marker != "" && slices.Contains(stack.Walk(path.Join(stack.Dir(part.Stack), "starter")), marker)
		where := ""
		if part.Side != "" {
			where = " en " + part.Side + "/"
		}
		switch {
		case marker == "" || p.Exists(stack.InPart(part, marker)) || fromStarter:
		case docker:
			ui.Log("Falta la app del framework" + where + ": ejecuta `" + stack.NewAppCommand(part) + "` (corre dentro de Docker).")
		default:
			ui.Log("Falta la app del framework" + where + ": créala con las herramientas del stack (" + marker + ").")
		}
	}
}

// writeMakefile writes the project's Makefile in dir ("" for the root). A user's own Makefile is kept: basekit's
// goes to Makefile.basekit next to it.
func writeMakefile(p *fsx.Project, dir, text string, prev, files map[string]string, force bool) {
	at := func(f string) string {
		if dir == "" {
			return f
		}
		return dir + "/" + f
	}
	makefile := at("Makefile")
	if own, ok := p.Read(makefile); ok && !strings.Contains(own, "generado por basekit") {
		makefile = at("Makefile.basekit")
	}
	mark := p.Tracked(makefile, text, 0o644, prev, files, force)
	if strings.HasSuffix(makefile, ".basekit") && (mark == fsx.Created || mark == fsx.Updated) {
		p.Report(fsx.Warned, at("Makefile"), "ya existe uno propio: el de basekit queda en Makefile.basekit; intégralos a mano")
	}
}

// overlayFiles are the database and Redis overlays infra-setup writes next to the compose files.
var overlayFiles = []string{"compose.db.yaml", "compose.redis.yaml"}

// Init implements spec 02 §3 step by step.
func Init(o InitOptions, ui Prompter) error {
	root, err := resolveDir(o.Dir)
	if err != nil {
		return err
	}
	stacks, err := stack.Load()
	if err != nil {
		return err
	}
	// Validate every flag before touching the disk. `<backend>+<frontend>` is a combined project (S16).
	if o.Stack != nil {
		if _, err := stack.ResolveParts(stacks, *o.Stack); err != nil {
			return err
		}
	}
	var flagAgents []string
	if o.Agents != nil {
		if flagAgents, err = adapters.Parse(*o.Agents); err != nil {
			return err
		}
	}
	if o.DB != nil {
		if err := checkDB(*o.DB); err != nil {
			return err
		}
	}

	p := fsx.New(root, o.DryRun, ui.Log)
	rootExists := fsx.IsDir(root)
	var prev manifest.Manifest
	if rootExists {
		if prev, err = manifest.Read(p); err != nil {
			return err
		}
	}

	// The questions follow what the project needs: the language narrows the
	// templates, the template decides the skills, the agents decide the adapters.
	// Each one is skipped when a flag, the manifest or detection answers it.

	// Q1–Q2 · Kind, language and framework (stack): flag → manifest → detection → questions.
	stackID := prev.Str("stack")
	if o.Stack != nil {
		stackID = *o.Stack
	}
	if stackID == "" && rootExists {
		if s := stack.Detect(stacks, root); s != nil {
			stackID = s.ID
		}
	}
	if stackID == "" {
		if !ui.Interactive() {
			return usage.Errorf("No se pudo detectar el stack. Usa --stack <id> (disponibles: %s)", stack.IDs(stacks))
		}
		if stackID, err = askStack(ui, stacks); err != nil {
			return err
		}
	}
	parts, err := stack.ResolveParts(stacks, strings.TrimSpace(stackID))
	if err != nil {
		return err
	}
	combined := stack.IsCombined(parts)
	// The database belongs to the first part (the backend of a combined project). S1b: a frontend stack has none.
	// Checked before any other question or write.
	dbStack := parts[0].Stack
	if err := checkStackDB(dbStack, o.DB, o.Redis); err != nil {
		return err
	}
	if combined && o.NoDocker {
		return usage.Errorf("un proyecto combinado (<backend>+<frontend>) necesita Docker: cada lado corre con su Makefile y compose; quita --no-docker")
	}

	// Q3 · Skills: the kit and stack skills are always installed because the roles
	// and `make verify` depend on them; only the external pack is optional.
	external := !o.NoExternal
	if external && ui.Interactive() && len(missingExternal(p)) > 0 {
		ui.Log("Skills incluidas: " + strings.Join(stack.SkillsOf(parts), ", "))
		if external, err = ui.Confirm("¿Instalar también las skills externas ("+strings.Join(manifest.ExternalNames, ", ")+")? Usan npx y red", true); err != nil {
			return err
		}
	}

	// Q4 · Agents: flag → manifest → detection → question → default.
	agents := flagAgents
	if saved := prev.Strings("agents"); agents == nil && len(saved) > 0 {
		if agents, err = adapters.Parse(strings.Join(saved, ",")); err != nil {
			return err
		}
	}
	if agents == nil && rootExists {
		agents = adapters.Detect(root)
	}
	if len(agents) == 0 && ui.Interactive() {
		if agents, err = askAgents(ui); err != nil {
			return err
		}
	}
	if len(agents) == 0 {
		agents = adapters.Defaults
	}

	// Q5 · Docker + Makefile: flag → manifest → question. A combined project always uses them: each side is a
	// Docker project of its stack, and the root Makefile delegates to both.
	docker, decided := prev.Option("docker").(bool)
	switch {
	case combined:
		docker = true
	case o.NoDocker:
		docker = false
	case !decided && ui.Interactive():
		if docker, err = ui.Confirm("¿Usar Docker + Makefile? Todo corre en contenedores y el Makefile es la interfaz", true); err != nil {
			return err
		}
	case !decided:
		docker = true
	}
	if !docker && ((o.DB != nil && *o.DB != "auto" && *o.DB != "none") || o.Redis) {
		return usage.Errorf("--db y --redis necesitan Docker: no se pueden usar con --no-docker ni sin Docker en el manifest")
	}

	// Q6 · Git: only asked when .git is missing (zone-check needs it).
	gitInit := !p.Exists(".git") && !o.NoGit
	if gitInit && ui.Interactive() {
		if gitInit, err = ui.Confirm("¿Inicializar git (git init)? zone-check lo necesita", true); err != nil {
			return err
		}
	}

	// Q7 · Context: flag → question on the first run.
	context := ""
	if o.Context != nil {
		context = *o.Context
	} else if prev == nil && ui.Interactive() {
		if context, err = ui.Text("¿De qué trata el proyecto? (puede ir vacío)"); err != nil {
			return err
		}
	}
	context = strings.TrimSpace(context)

	// Flags win over the manifest. The manifest's database and Redis carry over only to a stack that has a
	// database: an existing project switched to a frontend stack drops them.
	db, redis := "", o.Redis
	if stack.HasDatabase(dbStack) {
		db, _ = prev.Option("db").(string)
		if saved, ok := prev.Option("redis").(bool); ok && !redis {
			redis = saved
		}
	}
	if o.DB != nil {
		db = *o.DB
	}
	if db == "" {
		db = "auto"
	}
	if err := checkDB(db); err != nil {
		return err
	}
	if !stack.HasDatabase(dbStack) {
		db = "none"
	}

	if ui.Interactive() {
		ui.Log("") // separates the answers from the report
	}
	// Overlays left by an earlier backend stay (they are the user's files): say they no longer apply.
	if !stack.HasDatabase(dbStack) && !combined {
		for _, f := range overlayFiles {
			if p.Exists(f) {
				p.Report(fsx.Warned, f, "queda del stack anterior: un stack frontend no usa base de datos; bórralo si ya no lo necesitas")
			}
		}
	}
	// A single-stack project switched to combined with the database carried from the manifest (S19): the new backend
	// side has no overlays yet, so infra.sh runs for it as with an explicit --db. The root ones stay (the user's files).
	before := stack.PartsOf(stacks, prev.Str("stack"))
	carried := combined && before != nil && !stack.IsCombined(before) && o.DB == nil && ((db != "none" && db != "auto") || redis)
	if combined {
		for _, f := range append(overlayFiles, ".make/db.mk") {
			if p.Exists(f) {
				p.Report(fsx.Warned, f, "queda del stack anterior: la base de datos ahora vive en "+stack.InPart(parts[0], "")+"; bórralo si ya no lo necesitas")
			}
		}
	}
	if o.DryRun {
		ui.Log("(dry-run: no se escribe nada en " + root + ")")
	} else if err := os.MkdirAll(root, 0o755); err != nil {
		return err
	}

	// 5 · Git (zone-check needs it).
	switch {
	case p.Exists(".git"):
	case gitInit:
		p.Report(fsx.Created, ".git", "git init")
		if !o.DryRun {
			runCmd(ui.Log, root, nil, "git", "init", "-q")
		}
		// Only a repository basekit creates gets it: in an existing one it would renormalize files.
		p.Create(".gitattributes", stack.Read("core/gitattributes"), 0)
	default:
		p.Report(fsx.Warned, ".git", "sin git: zone-check no funciona hasta que ejecutes git init")
	}

	prevFiles := prev.Files()
	files := map[string]string{}
	// copyTracked copies every embedded file under srcRoot to dest (a folder of the project), tracked in the manifest.
	copyTracked := func(srcRoot, dest string) {
		for _, f := range stack.Walk(srcRoot) {
			p.Tracked(dest+"/"+f, stack.Read(path.Join(srcRoot, f)), stack.ModeOf(f), prevFiles, files, o.Force)
		}
	}
	stackSkills := func(part stack.Part) string { return path.Join(stack.Dir(part.Stack), "skills") }

	// 6 · .agents/ core (once, at the root, with the roles and skills of every part).
	p.Create(".agents/README.md", stack.Read("core/agents-README.md"), 0)
	for _, f := range stack.RoleFiles(parts) {
		p.Create(".agents/roles/"+f.Rel, f.Text, 0)
	}
	for _, f := range stack.ZoneFiles(parts) {
		p.Create(".agents/zones/"+f.Rel, f.Text, 0)
	}
	copyTracked("skills", ".agents/skills")
	for _, part := range parts {
		copyTracked(stackSkills(part), ".agents/skills")
	}
	// A side runs its Makefile inside Docker with only its own folder mounted: it gets a copy of its stack's skills,
	// which `make verify` and `make new-app` read from `.agents/skills/`. The skill scripts work on the project that
	// holds them, so the side with the database also gets infra-setup (S17).
	for _, part := range parts {
		if part.Side == "" {
			continue
		}
		copyTracked(stackSkills(part), stack.InPart(part, ".agents/skills"))
		if stack.HasDatabase(part.Stack) {
			copyTracked(path.Join("skills", stack.InfraSkill), stack.InPart(part, ".agents/skills/"+stack.InfraSkill))
		}
	}

	// 7 · AGENTS.md managed block.
	writeAgentsMd(p, parts, context)
	for _, f := range []string{".hermes.md", "HERMES.md"} {
		if p.Exists(f) {
			p.Report(fsx.Warned, f, "Hermes lo carga en lugar de AGENTS.md; bórralo o fusiónalo")
		}
	}

	// 8 · Docker and Make: each part is a complete project of its stack (S16: in its side folder, with a root
	// Makefile that delegates to both).
	if docker && combined {
		writeMakefile(p, "", stack.Read("core/combo/Makefile"), prevFiles, files, o.Force)
	}
	if !docker {
		p.Report(fsx.Warned, "Makefile", "sin Docker: no se crean Makefile, Dockerfile ni compose; las reglas de AGENTS.md asumen Docker y make, adáptalas")
	}
	for _, part := range parts {
		sdir := stack.Dir(part.Stack)
		marker := part.Stack.AppMarker
		appMissing := marker != "" && !p.Exists(stack.InPart(part, marker))
		if docker {
			writeMakefile(p, part.Side, stack.Read("core/Makefile"), prevFiles, files, o.Force)
			for _, f := range stack.Walk(path.Join(sdir, "files")) {
				text := stack.Read(path.Join(sdir, "files", f))
				if part.Side != "" && f == ".env.example" && !p.Exists(stack.InPart(part, f)) {
					var warning string
					if text, warning = stack.WireEnv(parts, part, text); warning != "" {
						p.Report(fsx.Warned, stack.InPart(part, f), warning)
					}
				}
				if f == "compose.yaml" {
					text = stack.NameCompose(root, part, text)
				}
				p.Create(stack.InPart(part, f), text, stack.ModeOf(f))
			}
		}
		if appMissing {
			for _, f := range stack.Walk(path.Join(sdir, "starter")) {
				p.Create(stack.InPart(part, f), stack.Read(path.Join(sdir, "starter", f)), stack.ModeOf(f))
			}
		}
	}
	p.Block(".gitignore", stack.Read("core/gitignore.block"), func(l string) bool { return strings.TrimSpace(l) == "# >>> basekit" }, "# <<< basekit")

	// 9 · Documentation (GLOSSARY.md and docs/adr/ are created later by domain-modeling).
	vars := templateVars(parts, root, context)
	p.Create("README.md", stack.Render(stack.Read("core/README.tpl.md"), vars), 0)
	for _, f := range stack.Walk("core/docs") {
		text := stack.Read("core/docs/" + f)
		if strings.HasSuffix(f, ".md") {
			text = stack.Render(text, vars)
		}
		p.Create("docs/"+f, text, 0)
	}

	// 10 · Adapters. 12 · External skills. 11 · Skill links last, so they also cover the external skills.
	adapters.Write(p, parts, agents)
	if external {
		installExternalSkills(p, agents, ui.Log, false, false)
	}
	adapters.LinkSkills(p, parts, agents)

	// 13 · Infrastructure, through the same script the agent uses: the copy of the part with the database, which
	// works on the folder that holds it. Only when --db is passed or the database moves to a new backend side: a rerun
	// that takes the db from the manifest must not touch it again.
	if docker && ((o.DB != nil && *o.DB != "auto") || carried) {
		script := stack.InPart(parts[0], stack.InfraScript)
		args := []string{script, "--db", db}
		if redis {
			args = append(args, "--redis")
		}
		if o.DryRun {
			ui.Log("· ejecutaría: bash " + strings.Join(args, " "))
		} else if p.Err() == nil && !runCmd(ui.Log, root, nil, "bash", args...) {
			p.Report(fsx.Warned, script, "falló; reinténtalo con: bash "+strings.Join(args, " "))
		}
	}

	manifest.Write(p, prev, manifest.Data{Version: Version, Stack: stack.SpecOf(parts), Agents: agents, DB: db, Redis: redis, Docker: docker, Files: files})
	if err := p.Err(); err != nil {
		return err
	}

	// 14 · Summary and next step.
	nextStep(ui, p, parts, context, docker)
	return nil
}

// kindOptions are the kinds of app (S1b) as the first step of the stack question shows them.
var kindOptions = map[string]Option{
	"backend":   {Label: "Backend", Hint: "API o servicio, con base de datos"},
	"frontend":  {Label: "Frontend", Hint: "SPA que corre en el navegador, sin base de datos"},
	"fullstack": {Label: "Fullstack", Hint: "Una app integrada, o un backend y un frontend combinados"},
}

// combineOption is the extra option of the fullstack step (S16): a backend stack in backend/ plus a frontend stack in
// frontend/.
var combineOption = Option{Label: "Combinar backend + frontend", Hint: "monorepo backend/ + frontend/; make dev levanta los dos", Keys: []string{"combinar", "combo"}}

// askStack asks Tipo → Lenguaje → Framework, each step filtered by the previous answers, and returns the stack id.
// Fullstack also offers «Combinar backend + frontend» (S16): then it asks Lenguaje → Framework for the backend and
// again for the frontend, and returns `<backend>+<frontend>`. A step with a single option is chosen by itself and
// announced; a stack id typed at any step is taken as is.
func askStack(ui Prompter, stacks []stack.Stack) (string, error) {
	combinable := true
	for _, side := range stack.Sides {
		combinable = combinable && slices.ContainsFunc(stacks, func(s stack.Stack) bool { return s.Kind == side })
	}
	var kinds []string
	var options []Option
	for _, k := range stack.Kinds {
		if slices.ContainsFunc(stacks, func(s stack.Stack) bool { return s.Kind == k }) || (k == "fullstack" && combinable) {
			o := kindOptions[k]
			o.Keys = []string{k}
			kinds, options = append(kinds, k), append(options, o)
		}
	}
	i, typed, err := askStep(ui, "Tipo", "¿Qué tipo de app es?", options, idsOf(stacks))
	if err != nil || typed != "" {
		return typed, err
	}
	var pool []stack.Stack
	for _, s := range stacks {
		if s.Kind == kinds[i] {
			pool = append(pool, s)
		}
	}
	id, combine, err := askLanguageThenFramework(ui, pool, stacks, "", kinds[i] == "fullstack" && combinable)
	if err != nil || !combine {
		return id, err
	}
	// «Combinar backend + frontend» (S16): the backend first, then the frontend.
	ids := make([]string, len(stack.Sides))
	for n, side := range stack.Sides {
		var ofSide []stack.Stack
		for _, s := range stacks {
			if s.Kind == side {
				ofSide = append(ofSide, s)
			}
		}
		// Without «Combinar» among the options, the answer is always a stack id; one typed is taken only from that side.
		if ids[n], _, err = askLanguageThenFramework(ui, ofSide, ofSide, side, false); err != nil {
			return "", err
		}
	}
	return strings.Join(ids, "+"), nil
}

func idsOf(stacks []stack.Stack) []string {
	ids := make([]string, len(stacks))
	for i, s := range stacks {
		ids[i] = s.ID
	}
	return ids
}

// askLanguageThenFramework asks the Lenguaje and Framework steps among pool and returns the chosen stack id; typed
// are the stacks whose id is accepted when typed directly. side names the half of a combined project in the
// questions. With combine, the language step also offers «Combinar backend + frontend», and choosing it returns
// combine true instead of a stack id.
func askLanguageThenFramework(ui Prompter, pool, typed []stack.Stack, side string, combine bool) (string, bool, error) {
	var languages []string
	for _, s := range pool {
		if !slices.Contains(languages, s.Language) {
			languages = append(languages, s.Language)
		}
	}
	slices.Sort(languages)
	var options []Option
	for _, l := range languages {
		var of []string
		for _, s := range pool {
			if s.Language == l {
				of = append(of, s.ID)
			}
		}
		options = append(options, Option{Label: l, Hint: strings.Join(of, ", "), Keys: []string{l}})
	}
	if combine {
		options = append(options, combineOption)
	}
	title, question := "Lenguaje", "¿En qué lenguaje?"
	if side != "" {
		title, question = "Lenguaje del "+side, "¿En qué lenguaje es el "+side+"?"
	}
	i, id, err := askStep(ui, title, question, options, idsOf(typed))
	if err != nil || id != "" {
		return id, false, err
	}
	if combine && i == len(languages) {
		return "", true, nil
	}
	var candidates []stack.Stack
	options = nil
	for _, s := range pool {
		if s.Language == languages[i] {
			candidates = append(candidates, s)
			options = append(options, Option{Label: s.Name, Hint: s.ID, Keys: []string{s.ID}})
		}
	}
	title, question = "Framework", "¿Con qué framework?"
	if side != "" {
		title, question = "Framework del "+side, "¿Con qué framework de "+side+"?"
	}
	if i, id, err = askStep(ui, title, question, options, idsOf(typed)); err != nil || id != "" {
		return id, false, err
	}
	return candidates[i].ID, false, nil
}

// askStep is one step of the stack question: the chosen option's index, or the id of a stack typed directly.
func askStep(ui Prompter, title, question string, options []Option, typed []string) (int, string, error) {
	if len(options) == 1 {
		ui.Log(title + ": " + options[0].Label + " (única opción)")
		return 0, "", nil
	}
	i, id, err := ui.Select(question, options, typed)
	if errors.Is(err, ErrNoAnswer) {
		return 0, "", usage.Errorf("No se eligió un stack. Usa --stack <id>")
	}
	return i, id, err
}

func askAgents(ui Prompter) ([]string, error) {
	ids := adapters.IDs()
	var defaults []int
	for i, id := range ids {
		if slices.Contains(adapters.Defaults, id) {
			defaults = append(defaults, i)
		}
	}
	chosen, err := ui.ChooseMany("Agentes", ids, defaults)
	if err != nil {
		return nil, err
	}
	var agents []string
	for _, i := range chosen {
		agents = append(agents, ids[i])
	}
	return agents, nil
}
