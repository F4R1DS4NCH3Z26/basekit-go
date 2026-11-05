package basekit

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"slices"
	"sort"
	"strings"
)

const usage = `basekit ` + Version + ` — prepara la base de un proyecto para agentes de código

Uso:
  basekit init [dir] [opciones]   Crea la base completa en un proyecto nuevo o existente
  basekit sync [dir] [opciones]   Regenera el bloque de AGENTS.md, los adaptadores y los enlaces
  basekit doctor [dir]            Diagnóstico del entorno y del proyecto (OK / FALTA)
  basekit stacks                  Lista los stacks, sus lenguajes y los agentes soportados

En una terminal, init pregunta lo que falte en este orden: lenguaje, template,
skills, agentes, Docker + Makefile, git init y contexto. Solo pregunta lo que no
resuelven los flags, el manifest o la detección.

Opciones de init:
  --stack <id>            Template/stack (por defecto: detectado; si no, se pregunta)
  --agents <a,b>          Agentes (por defecto: detectados; si no, claude-code,opencode)
  --context <texto>       De qué trata el proyecto
  --db <motor>            auto | postgres | mysql | sqlite | none (por defecto: auto)
  --redis                 Agrega Redis
  --no-docker             Sin Docker ni Makefile (no compatible con --db ni --redis)
  --no-git                No ejecuta git init (zone-check lo necesita)
  --no-external-skills    No instala las skills externas (CI o sin red)
  --force                 Reemplaza las skills del kit modificadas localmente
  --dry-run               Muestra lo que haría sin escribir nada
  -y, --yes               No interactivo: usa flags y valores por defecto

Opciones de sync:
  --agents <a,b>          Reemplaza la lista de agentes (los nuevos reciben adaptadores, enlaces y skills externas)
  --no-external-skills    No reinstala las skills externas
  --dry-run               Muestra lo que haría sin escribir nada
`

var (
	stringFlags = []string{"stack", "agents", "context", "db"}
	boolFlags   = []string{"redis", "no-docker", "no-git", "no-external-skills", "force", "dry-run", "yes", "help", "version"}
	shortFlags  = map[string]string{"-y": "yes", "-h": "help", "-v": "version"}
	// allowed lists the options each command accepts; anything else is an error
	// instead of being silently ignored.
	allowed = map[string][]string{
		"init":   {"stack", "agents", "context", "db", "redis", "no-docker", "no-git", "no-external-skills", "force", "dry-run", "yes"},
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

func (o *options) str(name string) (string, bool) {
	v, ok := o.values[name]
	return v, ok
}

func (o *options) flag(name string) bool { return o.bools[name] }

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
						return nil, usageErr("--%s necesita un valor", name)
					}
					i++
					value = args[i]
				}
				o.values[name] = value
			case slices.Contains(boolFlags, name):
				if hasValue {
					return nil, usageErr("--%s no acepta valor", name)
				}
				o.bools[name] = true
			default:
				return nil, usageErr("Opción desconocida: --%s", name)
			}
		case strings.HasPrefix(a, "-") && a != "-":
			name, ok := shortFlags[a]
			if !ok {
				return nil, usageErr("Opción desconocida: %s", a)
			}
			o.bools[name] = true
		default:
			o.positionals = append(o.positionals, a)
		}
	}
	return o, nil
}

// Main runs the CLI and returns the exit code: 0 ok, 1 failure, 2 usage error.
// interactive tells whether stdin is a terminal; init only asks questions then.
func Main(args []string, stdin io.Reader, stdout, stderr io.Writer, interactive bool) int {
	code, err := dispatch(args, stdin, stdout, interactive)
	if err != nil {
		fmt.Fprintf(stderr, "ERROR: %s\n", err)
		var ue *UsageError
		if errors.As(err, &ue) {
			return 2
		}
		return 1
	}
	return code
}

func dispatch(args []string, stdin io.Reader, stdout io.Writer, interactive bool) (int, error) {
	o, err := parseArgs(args)
	if err != nil {
		return 0, err
	}
	if o.flag("version") {
		fmt.Fprintln(stdout, Version)
		return 0, nil
	}
	if o.flag("help") || len(o.positionals) == 0 || o.positionals[0] == "help" {
		fmt.Fprint(stdout, usage)
		if len(o.positionals) == 0 && !o.flag("help") {
			return 2, nil
		}
		return 0, nil
	}
	command, rest := o.positionals[0], o.positionals[1:]
	accepted, known := allowed[command]
	if !known {
		return 0, usageErr("Comando desconocido: %s. Usa `basekit help`", command)
	}
	maxArgs := 1
	if command == "stacks" {
		maxArgs = 0
	}
	if len(rest) > maxArgs {
		return 0, usageErr("Argumentos de más: %s", strings.Join(rest[maxArgs:], " "))
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
		return 0, usageErr("%s no aplica a `%s`", strings.Join(extra, ", "), command)
	}

	u := &ui{out: stdout, in: bufio.NewReader(stdin)}
	switch command {
	case "stacks":
		return stacksCommand(u)
	case "init":
		u.interactive = interactive && !o.flag("yes")
		return 0, runInit(o, u)
	case "sync":
		return 0, runSync(o, u)
	default:
		return runDoctor(o, u)
	}
}

func stacksCommand(u *ui) (int, error) {
	stacks, err := LoadStacks()
	if err != nil {
		return 1, err
	}
	width, langWidth := 0, 0
	for _, s := range stacks {
		width = max(width, len(s.ID))
		langWidth = max(langWidth, len(s.Language))
	}
	u.log("Stacks:")
	for _, s := range stacks {
		u.log(fmt.Sprintf("  %-*s  %-*s  %s (puerto %d)", width, s.ID, langWidth, s.Language, s.Name, s.Port))
	}
	u.log("Agentes: " + strings.Join(agentIDs(), ", "))
	return 0, nil
}

// ui is the terminal: log lines always, questions only when interactive.
type ui struct {
	out         io.Writer
	in          *bufio.Reader
	interactive bool
}

func (u *ui) log(line string) { fmt.Fprintln(u.out, line) }
