// Package fsx is the only way basekit touches the target project on disk.
package fsx

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

// Mark is how each action is reported (spec 02 §3).
type Mark string

const (
	Created Mark = "+"
	Updated Mark = "~"
	Kept    Mark = "="
	Warned  Mark = "!"
)

// secretName matches files basekit must never read (spec 02 §12).
var secretName = regexp.MustCompile(`^\.env(\.prod|\..+\.local)?$`)

func sha(text string) string {
	sum := sha256.Sum256([]byte(text))
	return hex.EncodeToString(sum[:])
}

// IsDir reports whether p is an existing directory.
func IsDir(p string) bool {
	info, err := os.Stat(p)
	return err == nil && info.IsDir()
}

// Project confines every read and write to the project root: unchanged content is
// never rewritten and dry-run writes nothing. The first I/O error is kept and
// later operations become no-ops (see Err).
type Project struct {
	Root    string
	DryRun  bool
	Changed bool // true once anything was created or updated
	log     func(string)
	err     error
}

// New returns a Project rooted at root; log receives one line per action.
func New(root string, dryRun bool, log func(string)) *Project {
	return &Project{Root: filepath.Clean(root), DryRun: dryRun, log: log}
}

// Err returns the first error any operation hit.
func (p *Project) Err() error { return p.err }

func (p *Project) fail(err error) {
	if err != nil && p.err == nil {
		p.err = err
	}
}

// path resolves rel inside the project; anything outside is an error.
func (p *Project) path(rel string) (string, bool) {
	abs := filepath.Join(p.Root, filepath.FromSlash(rel))
	if outside(abs, p.Root) {
		p.fail(fmt.Errorf("ruta fuera del proyecto: %s", rel))
		return "", false
	}
	return abs, true
}

func outside(abs, root string) bool {
	return abs != root && !strings.HasPrefix(abs, root+string(filepath.Separator))
}

// escapes reports whether rel really lives outside the project because a
// directory on its way (or the file itself) is a symlink pointing out (02 §12).
func (p *Project) escapes(rel string) bool {
	if !IsDir(p.Root) {
		return false
	}
	probe, ok := p.path(rel)
	if !ok {
		return true
	}
	root, err := filepath.EvalSymlinks(p.Root)
	if err != nil {
		return true
	}
	// The file itself may be a symlink, even a dangling one that Stat cannot see:
	// writing would follow it, so it must resolve inside the root.
	if info, err := os.Lstat(probe); err == nil && info.Mode()&os.ModeSymlink != 0 {
		real, err := filepath.EvalSymlinks(probe)
		return err != nil || outside(real, root)
	}
	for probe != p.Root {
		if _, err := os.Stat(probe); err == nil {
			break
		}
		probe = filepath.Dir(probe)
	}
	real, err := filepath.EvalSymlinks(probe)
	return err != nil || outside(real, root)
}

// refuse reports and returns true when writing rel would leave the project.
func (p *Project) refuse(rel string) bool {
	if !p.escapes(rel) {
		return false
	}
	p.Report(Warned, rel, "fuera del proyecto por un enlace simbólico; no se escribe")
	return true
}

// Exists reports whether rel exists (following symlinks).
func (p *Project) Exists(rel string) bool {
	abs, ok := p.path(rel)
	if !ok {
		return false
	}
	_, err := os.Stat(abs)
	return err == nil
}

// Read returns the file content, or false when it does not exist. Secrets are never read.
func (p *Project) Read(rel string) (string, bool) {
	if secretName.MatchString(filepath.Base(rel)) {
		p.fail(fmt.Errorf("basekit nunca lee %s", rel))
		return "", false
	}
	abs, ok := p.path(rel)
	if !ok || p.escapes(rel) {
		return "", false
	}
	data, err := os.ReadFile(abs)
	if err != nil {
		if !errors.Is(err, fs.ErrNotExist) {
			p.fail(err)
		}
		return "", false
	}
	return string(data), true
}

func (p *Project) list(rel string, dirs bool) []string {
	abs, ok := p.path(rel)
	if !ok {
		return nil
	}
	es, err := os.ReadDir(abs)
	if err != nil {
		return nil
	}
	var names []string
	for _, e := range es {
		if (dirs && e.IsDir()) || (!dirs && e.Type().IsRegular()) {
			names = append(names, e.Name())
		}
	}
	sort.Strings(names)
	return names
}

// Dirs lists the sub-directories of rel; Files lists its regular files.
func (p *Project) Dirs(rel string) []string  { return p.list(rel, true) }
func (p *Project) Files(rel string) []string { return p.list(rel, false) }

// Report logs one action; created and updated marks flag the project as changed.
func (p *Project) Report(mark Mark, rel, note string) {
	if mark == Created || mark == Updated {
		p.Changed = true
	}
	if note != "" {
		p.log(fmt.Sprintf("%s %s  (%s)", mark, rel, note))
	} else {
		p.log(fmt.Sprintf("%s %s", mark, rel))
	}
}

// write stores content; mode 0 keeps the default (0644 for new files).
func (p *Project) write(rel, content string, mode os.FileMode) {
	if p.DryRun || p.err != nil {
		return
	}
	abs, ok := p.path(rel)
	if !ok {
		return
	}
	if err := os.MkdirAll(filepath.Dir(abs), 0o755); err != nil {
		p.fail(err)
		return
	}
	if err := os.WriteFile(abs, []byte(content), 0o644); err != nil {
		p.fail(err)
		return
	}
	if mode != 0 {
		p.fail(os.Chmod(abs, mode))
	}
}

// Create writes the file only if it does not exist.
func (p *Project) Create(rel, content string, mode os.FileMode) Mark {
	if p.refuse(rel) {
		return Warned
	}
	if p.Exists(rel) {
		p.Report(Kept, rel, "")
		return Kept
	}
	p.write(rel, content, mode)
	p.Report(Created, rel, "")
	return Created
}

// Generated creates or updates a file basekit owns, only when the content differs.
func (p *Project) Generated(rel, content string, mode os.FileMode) Mark {
	if p.refuse(rel) {
		return Warned
	}
	cur, ok := p.Read(rel)
	mark := Updated
	switch {
	case !ok:
		mark = Created
	case cur == content:
		mark = Kept
	}
	if mark != Kept {
		p.write(rel, content, mode)
	}
	p.Report(mark, rel, "")
	return mark
}

// Tracked writes a kit file the user may edit. It is updated only if it still
// matches what basekit wrote last time (hash in prev), or with force. The hash to
// remember goes to next.
func (p *Project) Tracked(rel, content string, mode os.FileMode, prev, next map[string]string, force bool) Mark {
	if p.refuse(rel) {
		return Warned
	}
	cur, ok := p.Read(rel)
	if !ok || cur == content {
		next[rel] = sha(content)
		return p.Generated(rel, content, mode)
	}
	untouched := prev[rel] == sha(cur)
	if untouched || force {
		p.write(rel, content, mode)
		next[rel] = sha(content)
		note := ""
		if !untouched {
			note = "reemplazado con --force"
		}
		p.Report(Updated, rel, note)
		return Updated
	}
	if h, ok := prev[rel]; ok {
		next[rel] = h
	}
	p.Report(Warned, rel, "modificado localmente; se conserva (usa --force para reemplazarlo)")
	return Warned
}

// Block replaces the managed block between its markers, or appends it after a
// blank line. Everything outside the block belongs to the user. It returns the
// resulting file content.
func (p *Project) Block(rel, block string, isStart func(string) bool, endLine string) string {
	body := strings.TrimRight(block, "\n") + "\n"
	if p.refuse(rel) {
		return ""
	}
	cur, ok := p.Read(rel)
	next := body
	if ok {
		lines := strings.Split(cur, "\n")
		start, end := -1, -1
		for i, l := range lines {
			if start < 0 && isStart(l) {
				start = i
			} else if start >= 0 && strings.TrimSpace(l) == endLine {
				end = i
				break
			}
		}
		if start >= 0 && end < 0 {
			// Without the end marker the user's text cannot be told apart: touch nothing.
			p.Report(Warned, rel, "tiene el marcador de inicio sin «"+endLine+"»; corrígelo a mano")
			return cur
		}
		if start >= 0 && end > start {
			out := append([]string{}, lines[:start]...)
			out = append(out, strings.Split(strings.TrimSuffix(body, "\n"), "\n")...)
			out = append(out, lines[end+1:]...)
			next = strings.Join(out, "\n")
		} else if kept := strings.TrimRight(cur, "\n"); kept != "" {
			next = kept + "\n\n" + body
		}
	}
	p.Generated(rel, next, 0)
	return next
}

// Link creates the relative symlink rel → targetRel, or copies the target when
// links are not allowed. It never replaces an existing entry.
func (p *Project) Link(rel, targetRel string) {
	if p.refuse(rel) {
		return
	}
	abs, ok := p.path(rel)
	target, ok2 := p.path(targetRel)
	if !ok || !ok2 {
		return
	}
	if _, err := os.Lstat(abs); err == nil {
		p.Report(Kept, rel, "")
		return
	}
	if _, err := os.Stat(target); err != nil && !p.DryRun {
		p.Report(Warned, rel, "falta "+targetRel+"; no se enlaza")
		return
	}
	if !p.DryRun && p.err == nil {
		if err := os.MkdirAll(filepath.Dir(abs), 0o755); err != nil {
			p.fail(err)
			return
		}
		relTarget, err := filepath.Rel(filepath.Dir(abs), target)
		if err == nil {
			err = os.Symlink(relTarget, abs)
		}
		if err != nil {
			p.fail(os.CopyFS(abs, os.DirFS(target)))
			p.Report(Created, rel, "copia: no se pudo crear el enlace")
			return
		}
	}
	p.Report(Created, rel, "")
}
