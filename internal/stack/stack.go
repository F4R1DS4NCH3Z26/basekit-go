// Package stack reads the embedded templates: the stacks (stack.json) and every
// file basekit copies. Everything stack-specific lives in the templates, never here.
package stack

import (
	"archive/tar"
	"bytes"
	_ "embed"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing/fstest"

	"github.com/F4R1DS4NCH3Z26/basekit-go/internal/usage"
	"github.com/F4R1DS4NCH3Z26/basekit-go/plantillas"
)

// modulesTar holds the template folders that have their own go.mod (the starter of a Go stack): go:embed cannot
// include a nested module, so `make templates-pack` packs them from plantillas/ and a test keeps the copy current.
//
//go:embed modules.tar
var modulesTar []byte

var templates = withModules(plantillas.FS, modulesTar)

// withModules is base plus the regular files of a tar archive (an archive file replaces a base file with the same
// path), as one read-only file system. Templates ship inside the binary, so an unreadable one is a packaging bug: it
// panics at start-up instead of failing later on a missing template.
//
// fstest.MapFS is used at runtime on purpose: it is the standard library's in-memory fs.FS (ReadDir, Stat, WalkDir
// and Glob all work on it), and merging two sources needs an FS that can be filled file by file, which embed.FS
// cannot. Nothing of the testing package is involved; the import adds no dependency.
func withModules(base fs.FS, archive []byte) fs.FS {
	files := fstest.MapFS{}
	if err := copyFS(files, base); err != nil {
		panic(fmt.Sprintf("plantillas embebidas ilegibles: %v", err))
	}
	if err := copyTar(files, archive); err != nil {
		panic(fmt.Sprintf("plantillas embebidas ilegibles: %v", err))
	}
	return files
}

// readOnly is how every template file is served.
func readOnly(data []byte) *fstest.MapFile { return &fstest.MapFile{Data: data, Mode: 0o444} }

// copyFS stores every regular file of base into files.
func copyFS(files fstest.MapFS, base fs.FS) error {
	return fs.WalkDir(base, ".", func(p string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil || d.IsDir() {
			return walkErr
		}
		data, err := fs.ReadFile(base, p)
		if err != nil {
			return err
		}
		files[p] = readOnly(data)
		return nil
	})
}

// copyTar stores every regular file of a tar archive into files; directories and other entries are skipped. An empty
// archive adds nothing.
func copyTar(files fstest.MapFS, archive []byte) error {
	r := tar.NewReader(bytes.NewReader(archive))
	for {
		hdr, err := r.Next()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return err
		}
		if hdr.Typeflag != tar.TypeReg {
			continue
		}
		data, err := io.ReadAll(r)
		if err != nil {
			return err
		}
		files[hdr.Name] = readOnly(data)
	}
}

// Kinds are the kinds of app a stack builds (S1b), in the order init offers them.
var Kinds = []string{"backend", "frontend", "fullstack"}

// Stack is a `plantillas/stacks/<id>/stack.json` (spec 02 §6).
type Stack struct {
	ID string `json:"id"`
	// Name is "<language> · <framework> …": the label of the framework question and how the stack reads in
	// AGENTS.md, the README, init's summary and `basekit stacks`.
	Name     string `json:"name"`
	Language string `json:"language"`
	Kind     string `json:"kind"`
	Port     int    `json:"port"`
	// Health is the path of the health endpoint (`/health`, `/up`…).
	Health      string      `json:"health"`
	Detect      *DetectRule `json:"detect"`
	AppMarker   string      `json:"appMarker"`
	Roles       []string    `json:"roles"`
	Skills      [][]string  `json:"skills"`
	DefaultAuth string      `json:"defaultAuth"`
}

// DetectRule is how an existing project is recognized (spec 02 §6). Priority orders
// the stacks for detection: lower is tried first, DefaultPriority when absent.
type DetectRule struct {
	Files    []string `json:"files"`
	Manifest string   `json:"manifest"`
	Contains string   `json:"contains"`
	Priority *int     `json:"priority"`
}

// DefaultPriority is the detection priority of a stack whose detect has none.
const DefaultPriority = 50

func (d *DetectRule) rank() int {
	if d == nil || d.Priority == nil {
		return DefaultPriority
	}
	return *d.Priority
}

// HasDatabase tells whether a stack can have a database and Redis: a frontend stack has none (S1b).
func HasDatabase(s Stack) bool { return s.Kind != "frontend" }

var idPattern = regexp.MustCompile(`^[a-z0-9][a-z0-9-]*$`)

// Load reads every `stacks/<id>/stack.json`, sorted by id.
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
			s.ID == name && idPattern.MatchString(s.ID) && s.Name != "" && strings.TrimSpace(s.Language) != "" &&
			slices.Contains(Kinds, s.Kind) && strings.HasPrefix(s.Health, "/") &&
			s.Port > 0 && s.Detect != nil && s.Roles != nil && s.Skills != nil
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
// exists and contains detect.contains. Stacks are tried by detect.priority, then
// by id, and the first match wins: a project that carries the markers of two
// stacks goes to the one with the stronger signal. An empty detect never matches.
func Detect(stacks []Stack, root string) *Stack {
	order := make([]int, len(stacks))
	for i := range order {
		order[i] = i
	}
	slices.SortStableFunc(order, func(a, b int) int {
		if d := stacks[a].Detect.rank() - stacks[b].Detect.rank(); d != 0 {
			return d
		}
		return strings.Compare(stacks[a].ID, stacks[b].ID)
	})
	for _, i := range order {
		d := stacks[i].Detect
		if d == nil {
			continue
		}
		for _, f := range d.Files {
			if _, err := os.Stat(filepath.Join(root, f)); err == nil {
				return &stacks[i]
			}
		}
		if d.Manifest == "" || d.Contains == "" {
			continue
		}
		data, err := os.ReadFile(filepath.Join(root, d.Manifest))
		if err == nil && strings.Contains(string(data), d.Contains) {
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
