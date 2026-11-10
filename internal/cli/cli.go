// Package cli is the delivery layer: flags, commands, help and the terminal that
// answers scaffold's questions.
package cli

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"slices"
	"sort"
	"strings"

	"github.com/F4R1DS4NCH3Z26/basekit-go/internal/adapters"
	"github.com/F4R1DS4NCH3Z26/basekit-go/internal/scaffold"
	"github.com/F4R1DS4NCH3Z26/basekit-go/internal/stack"
	"github.com/F4R1DS4NCH3Z26/basekit-go/internal/usage"
)

const helpBody = ` — prepara la base de un proyecto para agentes de código

Uso:
  basekit init [dir] [opciones]   Crea la base completa en un proyecto (por defecto, el directorio actual)
  basekit init --global           Instala las skills reutilizables para todos tus proyectos (~/.agents/skills)
  basekit sync [dir] [opciones]   Regenera el bloque de AGENTS.md, los adaptadores y los enlaces
  basekit doctor [dir]            Diagnóstico del entorno y del proyecto (OK / FALTA)
  basekit stacks                  Lista los stacks, sus lenguajes y los agentes soportados

En una terminal, init pregunta lo que falte en este orden: tipo de app, lenguaje, framework,
skills, agentes, Docker + Makefile, git init y contexto. Solo pregunta lo que no
resuelven los flags, el manifest o la detección. Flechas para moverte, espacio
para marcar, Enter para confirmar, Ctrl+C para cancelar sin escribir nada;
ACCESSIBLE=1 usa preguntas de texto plano (lectores de pantalla).
Las skills externas y los agentes son listas opcionales: desmarca todo, o
responde ninguna o ninguno en texto plano, para no elegir ninguno. La elección
de skills externas queda en el manifest y no se vuelve a preguntar.

Opciones de init:
  --global                Instalación global: solo skills del kit y externas, enlazadas en cada agente.
                          Admite --agents, --no-external-skills, --force, --dry-run y --yes
  --stack <id>            Stack (ver ` + "`basekit stacks`" + `; por defecto: detectado; si no, se pregunta
                          tipo de app → lenguaje → framework)
  --stack <back>+<front>  Fullstack combinado: un stack backend en backend/ y uno frontend en
                          frontend/, con un Makefile raíz que delega en los dos
  --agents <a,b>          Agentes (por defecto: detectados; si no, claude-code,opencode).
                          --agents none: ninguno (solo AGENTS.md y .agents/, sin adaptadores)
  --context <texto>       De qué trata el proyecto
  --db <motor>            auto | postgres | mysql | sqlite | none (por defecto: auto;
                          un stack frontend solo admite auto o none; en un combinado, va al backend)
  --redis                 Agrega Redis (no en un stack frontend; en un combinado, va al backend)
  --no-docker             Sin Docker ni Makefile (no compatible con --db, --redis ni un combinado)
  --no-git                No ejecuta git init (zone-check lo necesita)
  --no-external-skills    No instala las skills externas (CI o sin red)
  --force                 Reemplaza las skills del kit modificadas localmente
  --dry-run               Muestra lo que haría sin escribir nada
  -y, --yes               No interactivo: usa flags y valores por defecto

Opciones de sync:
  --agents <a,b>          Reemplaza la lista de agentes (los nuevos reciben adaptadores, enlaces y skills externas);
                          --agents none los quita todos
  --no-external-skills    No reinstala las skills externas
  --dry-run               Muestra lo que haría sin escribir nada
`

var (
	stringFlags = []string{"stack", "agents", "context", "db"}
	boolFlags   = []string{"global", "redis", "no-docker", "no-git", "no-external-skills", "force", "dry-run", "yes", "help", "version"}
	shortFlags  = map[string]string{"-y": "yes", "-h": "help", "-v": "version"}
	// allowed lists the options each command accepts; anything else is an error
	// instead of being silently ignored.
	allowed = map[string][]string{
		"init":   {"global", "stack", "agents", "context", "db", "redis", "no-docker", "no-git", "no-external-skills", "force", "dry-run", "yes"},
		"sync":   {"agents", "no-external-skills", "dry-run", "yes"},
		"doctor": {},
		"stacks": {},
	}
)

// options are the parsed command line. Presence matters: an absent flag lets the
// manifest, detection or a question decide.
type options struct {
	positionals []string
	dir         string
	values      map[string]string
	bools       map[string]bool
}

func (o *options) flag(name string) bool { return o.bools[name] }

// ptr returns the flag value, or nil when the flag was not given.
func (o *options) ptr(name string) *string {
	if v, ok := o.values[name]; ok {
		return &v
	}
	return nil
}

func (o *options) names() []string {
	var names []string
	for k := range o.values {
		names = append(names, k)
	}
	for k := range o.bools {
		names = append(names, k)
	}
	sort.Strings(names)
	return names
}

func parseArgs(args []string) (*options, error) {
	o := &options{values: map[string]string{}, bools: map[string]bool{}}
	for i := 0; i < len(args); i++ {
		a := args[i]
		switch {
		case a == "--":
			o.positionals = append(o.positionals, args[i+1:]...)
			i = len(args)
		case strings.HasPrefix(a, "--"):
			name, value, hasValue := strings.Cut(a[2:], "=")
			switch {
			case slices.Contains(stringFlags, name):
				if !hasValue {
					if i+1 >= len(args) {
						return nil, usage.Errorf("--%s necesita un valor", name)
					}
					i++
					value = args[i]
				}
				o.values[name] = value
			case slices.Contains(boolFlags, name):
				if hasValue {
					return nil, usage.Errorf("--%s no acepta valor", name)
				}
				o.bools[name] = true
			default:
				return nil, usage.Errorf("Opción desconocida: --%s", name)
			}
		case strings.HasPrefix(a, "-") && a != "-":
			name, ok := shortFlags[a]
			if !ok {
				return nil, usage.Errorf("Opción desconocida: %s", a)
			}
			o.bools[name] = true
		default:
			o.positionals = append(o.positionals, a)
		}
	}
	return o, nil
}

// Main runs the CLI and returns the exit code: 0 ok, 1 failure, 2 usage error,
// 130 cancelled by the user (like a shell). interactive tells whether stdin is a
// terminal; init only asks questions then.
func Main(args []string, stdin io.Reader, stdout, stderr io.Writer, interactive bool) int {
	code, err := dispatch(args, stdin, stdout, interactive)
	if err != nil {
		return exitCode(err, stderr)
	}
	return code
}

// exitCode reports err on stderr and returns its exit code. A cancelled question
// is not a failure, so it has no ERROR prefix.
func exitCode(err error, stderr io.Writer) int {
	if errors.Is(err, errCancelled) {
		fmt.Fprintln(stderr, errCancelled)
		return 130
	}
	fmt.Fprintf(stderr, "ERROR: %s\n", err)
	var ue *usage.Error
	if errors.As(err, &ue) {
		return 2
	}
	return 1
}

func dispatch(args []string, stdin io.Reader, stdout io.Writer, interactive bool) (int, error) {
	o, err := parseArgs(args)
	if err != nil {
		return 0, err
	}
	if o.flag("version") {
		fmt.Fprintln(stdout, scaffold.Version)
		return 0, nil
	}
	if o.flag("help") || len(o.positionals) == 0 || o.positionals[0] == "help" {
		fmt.Fprint(stdout, "basekit "+scaffold.Version+helpBody)
		if len(o.positionals) == 0 && !o.flag("help") {
			return 2, nil
		}
		return 0, nil
	}
	command, rest := o.positionals[0], o.positionals[1:]
	accepted, known := allowed[command]
	if !known {
		return 0, usage.Errorf("Comando desconocido: %s. Usa `basekit help`", command)
	}
	maxArgs := 1
	if command == "stacks" {
		maxArgs = 0
	}
	if len(rest) > maxArgs {
		return 0, usage.Errorf("Argumentos de más: %s", strings.Join(rest[maxArgs:], " "))
	}
	if len(rest) == 1 {
		o.dir = rest[0]
	}
	var extra []string
	for _, name := range o.names() {
		if !slices.Contains(accepted, name) {
			extra = append(extra, "--"+name)
		}
	}
	if len(extra) > 0 {
		return 0, usage.Errorf("%s no aplica a `%s`", strings.Join(extra, ", "), command)
	}

	t := &terminal{out: stdout, in: bufio.NewReader(stdin)}
	switch command {
	case "stacks":
		return stacksCommand(t)
	case "init":
		t.interactive = interactive && !o.flag("yes")
		var ui scaffold.Prompter = t
		if useHuh(stdin, t.interactive) {
			ui = &huhPrompter{out: stdout}
		}
		if o.flag("global") {
			if err := checkGlobal(o); err != nil {
				return 0, err
			}
			return 0, scaffold.InitGlobal(scaffold.GlobalOptions{
				Agents: o.ptr("agents"), NoExternal: o.flag("no-external-skills"), Force: o.flag("force"), DryRun: o.flag("dry-run"),
			}, ui)
		}
		return 0, scaffold.Init(scaffold.InitOptions{
			Dir: o.dir, Stack: o.ptr("stack"), Agents: o.ptr("agents"), Context: o.ptr("context"), DB: o.ptr("db"),
			Redis: o.flag("redis"), NoDocker: o.flag("no-docker"), NoGit: o.flag("no-git"),
			NoExternal: o.flag("no-external-skills"), Force: o.flag("force"), DryRun: o.flag("dry-run"),
		}, ui)
	case "sync":
		return 0, scaffold.Sync(scaffold.SyncOptions{
			Dir: o.dir, Agents: o.ptr("agents"), NoExternal: o.flag("no-external-skills"), DryRun: o.flag("dry-run"),
		}, t)
	default:
		return scaffold.Doctor(o.dir, t)
	}
}

// projectOnly are the init options that only make sense inside a project.
var projectOnly = []string{"stack", "context", "db", "redis", "no-docker", "no-git"}

// checkGlobal rejects what a global install cannot honor instead of ignoring it.
func checkGlobal(o *options) error {
	if o.dir != "" {
		return usage.Errorf("`init --global` no recibe directorio: instala en tu home para todos los proyectos")
	}
	for _, name := range projectOnly {
		if _, ok := o.values[name]; ok || o.bools[name] {
			return usage.Errorf("--%s no aplica con --global: es de cada proyecto (usa `basekit init` dentro del proyecto)", name)
		}
	}
	return nil
}

func stacksCommand(t *terminal) (int, error) {
	stacks, err := stack.Load()
	if err != nil {
		return 1, err
	}
	width := 0
	for _, s := range stacks {
		width = max(width, len(s.ID))
	}
	// The name reads "<language> · <framework> …", so it already names the language.
	t.Log("Stacks:")
	for _, s := range stacks {
		t.Log(fmt.Sprintf("  %-*s  %s (puerto %d)", width, s.ID, s.Name, s.Port))
	}
	t.Log("Agentes: " + strings.Join(adapters.IDs(), ", "))
	return 0, nil
}
