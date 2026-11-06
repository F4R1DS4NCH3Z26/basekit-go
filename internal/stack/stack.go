// Package stack reads the embedded templates: the stacks (stack.json) and every
// file basekit copies. Everything stack-specific lives in the templates, never here.
package stack

import (
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/F4R1DS4NCH3Z26/basekit-go/internal/usage"
	"github.com/F4R1DS4NCH3Z26/basekit-go/plantillas"
)

var templates fs.FS = plantillas.FS

// Stack is a `plantillas/stacks/<id>/stack.json` (spec 02 §6).
type Stack struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Language string `json:"language"`
	Port     int    `json:"port"`
	Detect   struct {
		Files    []string `json:"files"`
		Manifest string   `json:"manifest"`
		Contains string   `json:"contains"`
	} `json:"detect"`
	AppMarker   string     `json:"appMarker"`
	Roles       []string   `json:"roles"`
	Skills      [][]string `json:"skills"`
	DefaultAuth string     `json:"defaultAuth"`
}

var idPattern = regexp.MustCompile(`^[a-z0-9][a-z0-9-]*$`)

// Load reads every `stacks/<id>/stack.json`, sorted by id (detection order).
func Load() ([]Stack, error) {
	dirs, err := fs.ReadDir(templates, "stacks")
	if err != nil {
		return nil, err
	}
	var stacks []Stack
	for _, e := range dirs {
		if !e.IsDir() {
			continue
		}
		name := e.Name()
		file := path.Join("stacks", name, "stack.json")
		data, err := fs.ReadFile(templates, file)
		if err != nil {
			return nil, err
		}
		var s Stack
		valid := json.Unmarshal(data, &s) == nil &&
			s.ID == name && idPattern.MatchString(s.ID) && s.Name != "" && s.Language != "" &&
			s.Port > 0 && s.Roles != nil && s.Skills != nil
		for _, row := range s.Skills {
			valid = valid && len(row) == 2
		}
		if !valid {
			return nil, fmt.Errorf("stack.json inválido: %s", file)
		}
		stacks = append(stacks, s)
	}
	return stacks, nil
}

// IDs joins the stack ids for messages.
func IDs(stacks []Stack) string {
	ids := make([]string, len(stacks))
	for i, s := range stacks {
		ids[i] = s.ID
	}
	return strings.Join(ids, ", ")
}

// Pick returns the stack with that id or a usage error listing the valid ones.
func Pick(stacks []Stack, id string) (Stack, error) {
	for _, s := range stacks {
		if s.ID == id {
			return s, nil
		}
	}
	return Stack{}, usage.Errorf("--stack '%s' no existe. Disponibles: %s", id, IDs(stacks))
}

// Dir is the stack's folder inside the templates.
func Dir(s Stack) string { return path.Join("stacks", s.ID) }

// Detect implements spec 02 §6: any of detect.files exists, or detect.manifest
// exists and contains detect.contains. The first match wins.
func Detect(stacks []Stack, root string) *Stack {
	for i, s := range stacks {
		for _, f := range s.Detect.Files {
			if _, err := os.Stat(filepath.Join(root, f)); err == nil {
				return &stacks[i]
			}
		}
		if s.Detect.Manifest == "" || s.Detect.Contains == "" {
			continue
		}
		data, err := os.ReadFile(filepath.Join(root, s.Detect.Manifest))
		if err == nil && strings.Contains(string(data), s.Detect.Contains) {
			return &stacks[i]
		}
	}
	return nil
}

// Read returns an embedded template. Templates ship inside the binary, so a
// missing one is a packaging bug, not a runtime condition.
func Read(rel string) string {
	data, err := fs.ReadFile(templates, rel)
	if err != nil {
		panic(fmt.Sprintf("plantilla ausente: %s", rel))
	}
	return string(data)
}

// ReadOptional returns an embedded template and whether it exists.
func ReadOptional(rel string) (string, bool) {
	data, err := fs.ReadFile(templates, rel)
	return string(data), err == nil
}

// Exists reports whether an embedded template exists.
func Exists(rel string) bool {
	_, err := fs.Stat(templates, rel)
	return err == nil
}

// Walk lists the regular files under an embedded dir as sorted, slash-separated
// relative paths. A missing dir yields nothing.
func Walk(dir string) []string {
	var out []string
	_ = fs.WalkDir(templates, dir, func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || !d.Type().IsRegular() || d.Name() == ".DS_Store" {
			return nil
		}
		out = append(out, strings.TrimPrefix(p, dir+"/"))
		return nil
	})
	return out
}

// skillNames returns the top-level skill folders of an embedded skills dir.
func skillNames(dir string) []string {
	var names []string
	for _, f := range Walk(dir) {
		name, _, _ := strings.Cut(f, "/")
		if len(names) == 0 || names[len(names)-1] != name {
			names = append(names, name)
		}
	}
	return names
}

// CoreSkills are the stack-agnostic skills of the kit (the ones that can go global).
func CoreSkills() []string { return skillNames("skills") }

// KitSkills are the skills basekit installs itself: core first, then the stack's.
func KitSkills(s Stack) []string {
	return append(skillNames("skills"), skillNames(path.Join(Dir(s), "skills"))...)
}

// ModeOf is the mode a copied template file gets: scripts 755, the rest 644.
// Embedded files carry no permissions, so the extension decides.
func ModeOf(rel string) os.FileMode {
	if strings.HasSuffix(rel, ".sh") {
		return 0o755
	}
	return 0o644
}

var placeholder = regexp.MustCompile(`\{\{([A-Z_]+)\}\}`)

// Render replaces {{KEY}} placeholders; unknown keys are left as they are.
func Render(text string, vars map[string]string) string {
	return placeholder.ReplaceAllStringFunc(text, func(m string) string {
		if v, ok := vars[m[2:len(m)-2]]; ok {
			return v
		}
		return m
	})
}
