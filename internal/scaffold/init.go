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

func nextStep(ui Prompter, p *fsx.Project, st stack.Stack, context string, docker bool) {
	ui.Log("")
	switch {
	case p.DryRun:
		ui.Log("Dry-run: no se escribió nada; arriba está lo que haría.")
	case p.Changed:
		ui.Log("Listo: base de " + st.Name + " preparada en " + p.Root)
	default:
		ui.Log("Todo al día: no había nada que cambiar.")
	}
	if context == "" {
		context = "<una o dos frases>"
	}
	ui.Log("Próximo paso, dile a tu agente:")
	ui.Log("  «Lee AGENTS.md y ejecuta el bootstrap. El proyecto trata de: " + context + "»")
	marker := st.AppMarker
	fromStarter := marker != "" && slices.Contains(stack.Walk(path.Join(stack.Dir(st), "starter")), marker)
	switch {
	case marker == "" || p.Exists(marker) || fromStarter:
	case docker:
		ui.Log("Falta la app del framework: ejecuta `make new-app` (corre dentro de Docker).")
	default:
		ui.Log("Falta la app del framework: créala con las herramientas del stack (" + marker + ").")
	}
}

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
	// Validate every flag before touching the disk.
	if o.Stack != nil {
		if _, err := stack.Pick(stacks, *o.Stack); err != nil {
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
	st, err := stack.Pick(stacks, strings.TrimSpace(stackID))
	if err != nil {
		return err
	}
	// S1b: a frontend stack has no database. Checked before any other question or write.
	if err := checkStackDB(st, o.DB, o.Redis); err != nil {
		return err
	}

	// Q3 · Skills: the kit and stack skills are always installed because the roles
	// and `make verify` depend on them; only the external pack is optional.
	external := !o.NoExternal
	if external && ui.Interactive() && len(missingExternal(p)) > 0 {
		ui.Log("Skills incluidas: " + strings.Join(stack.KitSkills(st), ", "))
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

	// Q5 · Docker + Makefile: flag → manifest → question.
	docker, decided := prev.Option("docker").(bool)
	switch {
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
	if stack.HasDatabase(st) {
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
	if !stack.HasDatabase(st) {
		db = "none"
	}

	if ui.Interactive() {
		ui.Log("") // separates the answers from the report
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

	sdir := stack.Dir(st)
	prevFiles := prev.Files()
	files := map[string]string{}

	// 6 · .agents/ core.
	p.Create(".agents/README.md", stack.Read("core/agents-README.md"), 0)
	for _, f := range stack.Walk(path.Join(sdir, "roles")) {
		p.Create(".agents/roles/"+f, stack.Read(path.Join(sdir, "roles", f)), 0)
	}
	for _, f := range stack.Walk(path.Join(sdir, "zones")) {
		if f != "orchestrator.stack.txt" {
			p.Create(".agents/zones/"+f, stack.Read(path.Join(sdir, "zones", f)), 0)
		}
	}
	common := stack.Read("core/zones.orchestrator.common.txt")
	stackZones, _ := stack.ReadOptional(path.Join(sdir, "zones/orchestrator.stack.txt"))
	if common != "" && !strings.HasSuffix(common, "\n") {
		common += "\n"
	}
	p.Create(".agents/zones/orchestrator.txt", common+stackZones, 0)
	for _, skillsRoot := range []string{"skills", path.Join(sdir, "skills")} {
		for _, f := range stack.Walk(skillsRoot) {
			p.Tracked(".agents/skills/"+f, stack.Read(path.Join(skillsRoot, f)), stack.ModeOf(f), prevFiles, files, o.Force)
		}
	}

	// 7 · AGENTS.md managed block.
	writeAgentsMd(p, st, context)
	for _, f := range []string{".hermes.md", "HERMES.md"} {
		if p.Exists(f) {
			p.Report(fsx.Warned, f, "Hermes lo carga en lugar de AGENTS.md; bórralo o fusiónalo")
		}
	}

	// 8 · Docker and Make.
	appMissing := st.AppMarker != "" && !p.Exists(st.AppMarker)
	if docker {
		makefile := "Makefile"
		if own, ok := p.Read("Makefile"); ok && !strings.Contains(own, "generado por basekit") {
			makefile = "Makefile.basekit"
		}
		mark := p.Tracked(makefile, stack.Read("core/Makefile"), 0o644, prevFiles, files, o.Force)
		if makefile == "Makefile.basekit" && (mark == fsx.Created || mark == fsx.Updated) {
			p.Report(fsx.Warned, "Makefile", "ya existe uno propio: el de basekit queda en Makefile.basekit; intégralos a mano")
		}
		for _, f := range stack.Walk(path.Join(sdir, "files")) {
			p.Create(f, stack.Read(path.Join(sdir, "files", f)), stack.ModeOf(f))
		}
	} else {
		p.Report(fsx.Warned, "Makefile", "sin Docker: no se crean Makefile, Dockerfile ni compose; las reglas de AGENTS.md asumen Docker y make, adáptalas")
	}
	if appMissing {
		for _, f := range stack.Walk(path.Join(sdir, "starter")) {
			p.Create(f, stack.Read(path.Join(sdir, "starter", f)), stack.ModeOf(f))
		}
	}
	p.Block(".gitignore", stack.Read("core/gitignore.block"), func(l string) bool { return strings.TrimSpace(l) == "# >>> basekit" }, "# <<< basekit")

	// 9 · Documentation (GLOSSARY.md and docs/adr/ are created later by domain-modeling).
	vars := templateVars(st, root, context)
	p.Create("README.md", stack.Render(stack.Read("core/README.tpl.md"), vars), 0)
	for _, f := range stack.Walk("core/docs") {
		text := stack.Read("core/docs/" + f)
		if strings.HasSuffix(f, ".md") {
			text = stack.Render(text, vars)
		}
		p.Create("docs/"+f, text, 0)
	}

	// 10 · Adapters. 12 · External skills. 11 · Skill links last, so they also cover the external skills.
	adapters.Write(p, st, agents)
	if external {
		installExternalSkills(p, agents, ui.Log, false, false)
	}
	adapters.LinkSkills(p, st, agents)

	// 13 · Infrastructure, through the same script the agent uses. Only when --db is
	// passed: a rerun that takes the db from the manifest must not touch it again.
	if docker && o.DB != nil && *o.DB != "auto" {
		args := []string{infraScript, "--db", db}
		if redis {
			args = append(args, "--redis")
		}
		if o.DryRun {
			ui.Log("· ejecutaría: bash " + strings.Join(args, " "))
		} else if p.Err() == nil && !runCmd(ui.Log, root, nil, "bash", args...) {
			p.Report(fsx.Warned, infraScript, "falló; reinténtalo con: bash "+strings.Join(args, " "))
		}
	}

	manifest.Write(p, prev, manifest.Data{Version: Version, Stack: st.ID, Agents: agents, DB: db, Redis: redis, Docker: docker, Files: files})
	if err := p.Err(); err != nil {
		return err
	}

	// 14 · Summary and next step.
	nextStep(ui, p, st, context, docker)
	return nil
}

// kindOptions are the kinds of app (S1b) as the first step of the stack question shows them.
var kindOptions = map[string]Option{
	"backend":   {Label: "Backend", Hint: "API o servicio, con base de datos"},
	"frontend":  {Label: "Frontend", Hint: "SPA que corre en el navegador, sin base de datos"},
	"fullstack": {Label: "Fullstack", Hint: "Una app integrada, o un backend y un frontend combinados"},
}

// askStack asks Tipo → Lenguaje → Framework, each step filtered by the previous answers, and returns the stack id.
// A step with a single option is chosen by itself and announced; a stack id typed at any step is taken as is.
func askStack(ui Prompter, stacks []stack.Stack) (string, error) {
	ids := make([]string, len(stacks))
	for i, s := range stacks {
		ids[i] = s.ID
	}
	var kinds []string
	var options []Option
	for _, k := range stack.Kinds {
		if slices.ContainsFunc(stacks, func(s stack.Stack) bool { return s.Kind == k }) {
			o := kindOptions[k]
			o.Keys = []string{k}
			kinds, options = append(kinds, k), append(options, o)
		}
	}
	i, typed, err := askStep(ui, "Tipo", "¿Qué tipo de app es?", options, ids)
	if err != nil || typed != "" {
		return typed, err
	}
	var pool []stack.Stack
	var languages []string
	for _, s := range stacks {
		if s.Kind == kinds[i] {
			pool = append(pool, s)
			if !slices.Contains(languages, s.Language) {
				languages = append(languages, s.Language)
			}
		}
	}
	slices.Sort(languages)
	options = nil
	for _, l := range languages {
		var of []string
		for _, s := range pool {
			if s.Language == l {
				of = append(of, s.ID)
			}
		}
		options = append(options, Option{Label: l, Hint: strings.Join(of, ", "), Keys: []string{l}})
	}
	if i, typed, err = askStep(ui, "Lenguaje", "¿En qué lenguaje?", options, ids); err != nil || typed != "" {
		return typed, err
	}
	var candidates []stack.Stack
	options = nil
	for _, s := range pool {
		if s.Language == languages[i] {
			candidates = append(candidates, s)
			options = append(options, Option{Label: s.Name, Hint: s.ID, Keys: []string{s.ID}})
		}
	}
	if i, typed, err = askStep(ui, "Framework", "¿Con qué framework?", options, ids); err != nil || typed != "" {
		return typed, err
	}
	return candidates[i].ID, nil
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
