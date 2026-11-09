package stack

import (
	"path"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"github.com/F4R1DS4NCH3Z26/basekit-go/internal/usage"
)

// Sides are the folders of a combined project (S16), in the order of `<backend>+<frontend>`: each side holds a stack
// of the kind with the same name.
var Sides = []string{"backend", "frontend"}

// Part is where one stack of a project lives. A single stack is one part at the root (Side ""), exactly as before;
// a combined project is a backend part in `backend/` plus a frontend part in `frontend/`. The `--stack` value and the
// manifest's `stack` field share one grammar: `<id>` or `<backend>+<frontend>`.
type Part struct {
	Stack Stack
	Side  string
}

const pairGrammar = "<backend>+<frontend>"

// ResolveParts returns the parts of a `--stack` value; any bad id or pair is a usage error. The first part is the one
// with the database. A single id is taken as is; the ids of a pair are trimmed.
func ResolveParts(stacks []Stack, spec string) ([]Part, error) {
	ids := strings.Split(spec, "+")
	if len(ids) == 1 {
		s, err := Pick(stacks, ids[0])
		if err != nil {
			return nil, err
		}
		return []Part{{Stack: s}}, nil
	}
	hint := "Backend: " + idsOfKind(stacks, "backend") + ". Frontend: " + idsOfKind(stacks, "frontend")
	for i := range ids {
		ids[i] = strings.TrimSpace(ids[i])
	}
	if len(ids) != len(Sides) || ids[0] == "" || ids[1] == "" {
		return nil, usage.Errorf("--stack '%s' no es válido: un proyecto combinado es %s (un stack backend y uno frontend). %s", spec, pairGrammar, hint)
	}
	parts := make([]Part, len(Sides))
	for i, side := range Sides {
		s, err := Pick(stacks, ids[i])
		if err != nil {
			return nil, err
		}
		if s.Kind != side {
			which := "primera"
			if i > 0 {
				which = "segunda"
			}
			return nil, usage.Errorf("--stack '%s': %s es %s, y en %s la %s parte es un stack %s. %s", spec, s.ID, s.Kind, pairGrammar, which, side, hint)
		}
		parts[i] = Part{Stack: s, Side: side}
	}
	return parts, nil
}

func idsOfKind(stacks []Stack, kind string) string {
	var ids []string
	for _, s := range stacks {
		if s.Kind == kind {
			ids = append(ids, s.ID)
		}
	}
	return strings.Join(ids, ", ")
}

// PartsOf returns the parts of a manifest's `stack`, or nil when this version does not know it.
func PartsOf(stacks []Stack, spec string) []Part {
	if spec == "" {
		return nil
	}
	parts, err := ResolveParts(stacks, spec)
	if err != nil {
		return nil
	}
	return parts
}

// SpecOf is the value recorded in the manifest: the stack id, or `<backend>+<frontend>`.
func SpecOf(parts []Part) string {
	ids := make([]string, len(parts))
	for i, p := range parts {
		ids[i] = p.Stack.ID
	}
	return strings.Join(ids, "+")
}

// IsCombined tells whether the parts are the two sides of a combined project.
func IsCombined(parts []Part) bool {
	for _, p := range parts {
		if p.Side != "" {
			return true
		}
	}
	return false
}

// InPart is rel inside the part's folder (unchanged for a single stack).
func InPart(p Part, rel string) string {
	if p.Side == "" {
		return rel
	}
	return p.Side + "/" + rel
}

// InfraSkill is the skill that writes the database and Redis overlays. Its script works on the project that holds
// it, so in a combined project the backend (the part with the database) has its own copy (S17).
const InfraSkill = "infra-setup"

// InfraScript is infra-setup's script, relative to a part's folder.
const InfraScript = ".agents/skills/" + InfraSkill + "/scripts/infra.sh"

// NewAppCommand is the command that creates the framework's app of a part.
func NewAppCommand(p Part) string {
	if p.Side == "" {
		return "make new-app"
	}
	return "make -C " + p.Side + " new-app"
}

// RoleIn is the role name in the project: prefixed by side in a combined project (`backend-data-writer`).
func RoleIn(p Part, role string) string {
	if p.Side == "" {
		return role
	}
	return p.Side + "-" + role
}

// DisplayName is how the project's stack reads in AGENTS.md, the README and init's summary.
func DisplayName(parts []Part) string {
	if !IsCombined(parts) {
		return parts[0].Stack.Name
	}
	names := make([]string, len(parts))
	for i, p := range parts {
		names[i] = p.Stack.Name + " en " + p.Side + "/"
	}
	return strings.Join(names, " + ")
}

// SkillsOf are the kit skills of every part: core first, then each stack's, without repeats.
func SkillsOf(parts []Part) []string {
	skills := skillNames("skills")
	for _, p := range parts {
		for _, s := range skillNames(path.Join(Dir(p.Stack), "skills")) {
			if !contains(skills, s) {
				skills = append(skills, s)
			}
		}
	}
	return skills
}

// StackSkills are the top-level skill folders of a stack.
func StackSkills(s Stack) []string { return skillNames(path.Join(Dir(s), "skills")) }

func contains(list []string, s string) bool {
	for _, x := range list {
		if x == s {
			return true
		}
	}
	return false
}

// KitFile is a text file the kit writes under a known folder (Rel relative to it).
type KitFile struct {
	Rel, Text string
}

var frontMatter = regexp.MustCompile(`(?s)^---\n(.*?)\n---\n?(.*)$`)
var nameLine = regexp.MustCompile(`(?m)^name:.*$`)

// RoleFiles are `.agents/roles/` of the project. A single stack gets its role files as they are; in a combined
// project each role is renamed by side and opens with a note on where it works (`core/combo/role.note.md`).
func RoleFiles(parts []Part) []KitFile {
	var files []KitFile
	for _, p := range parts {
		dir := path.Join(Dir(p.Stack), "roles")
		for _, rel := range Walk(dir) {
			text := Read(path.Join(dir, rel))
			if p.Side == "" {
				files = append(files, KitFile{rel, text})
				continue
			}
			if strings.Contains(rel, "/") || !strings.HasSuffix(rel, ".md") {
				files = append(files, KitFile{p.Side + "-" + rel, text})
				continue
			}
			role := strings.TrimSuffix(rel, ".md")
			name := RoleIn(p, role)
			note := Render(Read("core/combo/role.note.md"), map[string]string{"SIDE": p.Side, "STACK_NAME": p.Stack.Name, "ROLE": name})
			note = strings.TrimRight(note, "\n") + "\n"
			body, meta := text, "name: "+name
			if m := frontMatter.FindStringSubmatch(text); m != nil {
				body = m[2]
				meta = m[1]
				if loc := nameLine.FindStringIndex(meta); loc != nil { // the first name: line, like Node's replace
					meta = meta[:loc[0]] + "name: " + name + meta[loc[1]:]
				}
			}
			body = strings.ReplaceAll(body, ".agents/zones/"+role+".txt", ".agents/zones/"+name+".txt")
			files = append(files, KitFile{name + ".md", "---\n" + meta + "\n---\n" + note + "\n" + body})
		}
	}
	return files
}

// prefixGlobs moves every glob line of a zone file under dir/; comments and blank lines stay as they are.
func prefixGlobs(text, dir string) string {
	lines := strings.Split(text, "\n")
	for i, l := range lines {
		if t := strings.TrimSpace(l); t != "" && !strings.HasPrefix(t, "#") {
			lines[i] = dir + "/" + t
		}
	}
	return strings.Join(lines, "\n")
}

// joined appends b to a on a new line.
func joined(a, b string) string {
	if a == "" || strings.HasSuffix(a, "\n") {
		return a + b
	}
	return a + "\n" + b
}

// ZoneFiles are `.agents/zones/` of the project: one file per role and the orchestrator's, which starts with the
// common zones. In a combined project every glob of a side is moved under its folder, and the orchestrator also owns
// each side's project files (the common zones again, under `backend/` and `frontend/`).
func ZoneFiles(parts []Part) []KitFile {
	common := Read("core/zones.orchestrator.common.txt")
	orchestrator := common
	var files []KitFile
	for _, p := range parts {
		dir := path.Join(Dir(p.Stack), "zones")
		own := ""
		for _, rel := range Walk(dir) {
			text := Read(path.Join(dir, rel))
			switch {
			case rel == "orchestrator.stack.txt":
				own = text
			case p.Side == "":
				files = append(files, KitFile{rel, text})
			default:
				files = append(files, KitFile{p.Side + "-" + rel, prefixGlobs(text, p.Side)})
			}
		}
		if p.Side == "" {
			orchestrator = joined(orchestrator, own)
		} else {
			orchestrator = joined(orchestrator, "# "+p.Side+"/ ("+p.Stack.Name+")\n"+joined(prefixGlobs(common, p.Side), prefixGlobs(own, p.Side)))
		}
	}
	return append(files, KitFile{"orchestrator.txt", orchestrator})
}

// The variables of the API wiring (S16): the frontend's API base URL and the backend's allowed origins.
const (
	apiURLKey = "VITE_API_URL"
	corsKey   = "CORS_ORIGINS"
)

// WireEnv returns the `.env.example` of one half of a combined project, wired to the other half: the browser reaches
// the API through the host (`127.0.0.1:<backend port>`), and the backend allows the frontend's origin. warning says
// what to wire by hand when the stack does not have the variable: init never invents one.
func WireEnv(parts []Part, p Part, text string) (wired, warning string) {
	var backend, frontend *Part
	for i := range parts {
		switch parts[i].Side {
		case "backend":
			backend = &parts[i]
		case "frontend":
			frontend = &parts[i]
		}
	}
	if backend == nil || frontend == nil {
		return text, ""
	}
	api := "http://127.0.0.1:" + strconv.Itoa(backend.Stack.Port)
	web := "http://127.0.0.1:" + strconv.Itoa(frontend.Stack.Port)
	key, value := corsKey, web+",http://localhost:"+strconv.Itoa(frontend.Stack.Port)
	warning = p.Stack.ID + " no tiene " + corsKey + ": la API debe permitir (CORS) el origen " + web + " del frontend"
	if p.Side == "frontend" {
		key, value = apiURLKey, api
		warning = p.Stack.ID + " no tiene " + apiURLKey + ": apunta el frontend a la API en " + api
	}
	re := regexp.MustCompile(`(?m)^` + key + `=.*$`)
	loc := re.FindStringIndex(text)
	if loc == nil {
		return text, warning
	}
	return text[:loc[0]] + key + "=" + value + text[loc[1]:], ""
}

var (
	composeName     = regexp.MustCompile(`(?m)^name:`)
	notComposeChars = regexp.MustCompile(`[^a-z0-9_-]`)
	leadingDashes   = regexp.MustCompile(`^[-_]+`)
)

// NameCompose returns the `compose.yaml` of one half of a combined project with a top-level `name`. Prod uses
// Compose's default project name, the folder's: every combined project would be called `backend` and `frontend`, so
// two of them on one server would take over each other's containers and volumes. The name is `<project>-<side>`,
// normalized like Compose does; dev keeps its own `-p <side>-dev-<checksum>` from the Makefile, which wins over it.
func NameCompose(root string, p Part, text string) string {
	if p.Side == "" || composeName.MatchString(text) {
		return text
	}
	project := leadingDashes.ReplaceAllString(notComposeChars.ReplaceAllString(strings.ToLower(filepath.Base(root)), ""), "")
	if project == "" {
		project = "app"
	}
	rows := strings.Split(text, "\n")
	at := len(rows)
	for i, l := range rows {
		if strings.TrimSpace(l) != "" && !strings.HasPrefix(l, "#") {
			at = i
			break
		}
	}
	rows = append(rows[:at], append([]string{"name: " + project + "-" + p.Side}, rows[at:]...)...)
	return strings.Join(rows, "\n")
}
