package scaffold

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"runtime"
	"slices"
	"strings"

	"github.com/F4R1DS4NCH3Z26/basekit-go/internal/adapters"
	"github.com/F4R1DS4NCH3Z26/basekit-go/internal/fsx"
	"github.com/F4R1DS4NCH3Z26/basekit-go/internal/manifest"
	"github.com/F4R1DS4NCH3Z26/basekit-go/internal/usage"
)

// Executable returns the path of the running basekit; tests point it at a fake file.
var Executable = os.Executable

// The platform hooks of uninstall; internal tests replace them so no test touches the registry or starts cmd.
var (
	goos = runtime.GOOS
	// removeUserPath removes dir from the user PATH (Windows only).
	removeUserPath = removeFromUserPath
	// removeAfterExit deletes target once this process has exited (Windows only: a running .exe cannot delete itself);
	// isDir deletes a whole folder.
	removeAfterExit = deleteAfterExit
)

// UninstallOptions are the flags of `basekit uninstall`.
type UninstallOptions struct {
	Skills, Yes, DryRun bool
}

// uninstallPlan is everything uninstall removes, in the order it is shown.
type uninstallPlan struct {
	exe string // the running executable, symlinks resolved
	// installDir is the installer's folder holding exe on Windows: it leaves the user PATH, and it is deleted after
	// exit when deleteDir is true (it holds nothing but basekit).
	installDir string
	deleteDir  bool
	// The global skills (--skills): files listed in the global manifest, the skill folders they live in, the links
	// basekit made to those skills, and the manifest. All are absolute.
	home       string
	files      []string
	skillDirs  []string
	links      []string
	manifest   string
	noManifest bool
}

// Uninstall removes the running basekit and, with Skills, the global skills `init --global` installed. It never
// touches the projects basekit created.
func Uninstall(o UninstallOptions, ui Prompter) error {
	exe, err := runningExecutable()
	if err != nil {
		return err
	}
	if !o.Yes && !o.DryRun && !ui.Interactive() {
		return usage.Errorf("basekit uninstall necesita confirmación: usa -y (o --dry-run para ver qué borraría)")
	}
	plan := uninstallPlan{exe: exe}
	if goos == "windows" {
		plan.installDir, plan.deleteDir = installerFolder(exe)
	}
	if o.Skills {
		if err := plan.addSkills(); err != nil {
			return err
		}
	}
	others := otherCopies(exe)

	ui.Log("Se va a desinstalar:")
	for _, item := range plan.items() {
		ui.Log("  - " + item)
	}
	if plan.noManifest {
		ui.Log("= skills globales  (no hay instalación global)")
	}
	if o.DryRun {
		ui.Log("(dry-run: no se borra nada)")
		return nil
	}
	if !o.Yes {
		yes, err := ui.Confirm("¿Desinstalar basekit?", false)
		if err != nil {
			return err
		}
		if !yes {
			ui.Log("No se desinstaló nada.")
			return nil
		}
	}
	if err := plan.run(ui.Log); err != nil {
		return err
	}
	for _, other := range others {
		ui.Log("! otra copia de basekit en el PATH: " + other + " (no se borra)")
	}
	ui.Log("basekit desinstalado. Los proyectos que ya creaste no cambian.")
	return nil
}

// runningExecutable is the running basekit with symlinks resolved; a `go run` build is refused, as there is
// nothing installed to remove.
func runningExecutable() (string, error) {
	exe, err := Executable()
	if err != nil {
		return "", err
	}
	if exe, err = filepath.EvalSymlinks(exe); err != nil {
		return "", err
	}
	if temporaryBuild(exe) {
		return "", usage.Errorf("basekit uninstall: este basekit corre desde una compilación temporal (go run); no hay nada que desinstalar")
	}
	return exe, nil
}

// temporaryBuild tells whether exe is a `go run` build: under the temp folder or in a go-build folder (the build
// cache that newer Go versions run from).
func temporaryBuild(exe string) bool {
	for _, dir := range strings.Split(filepath.ToSlash(exe), "/") {
		if strings.HasPrefix(dir, "go-build") {
			return true
		}
	}
	tmp := filepath.Clean(os.TempDir())
	if real, err := filepath.EvalSymlinks(tmp); err == nil {
		tmp = real
	}
	return within(exe, tmp)
}

// within tells whether p is dir or lives under it (ignoring case on Windows).
func within(p, dir string) bool {
	p, dir = filepath.Clean(p), filepath.Clean(dir)
	if goos == "windows" {
		p, dir = strings.ToLower(p), strings.ToLower(dir)
	}
	return p == dir || strings.HasPrefix(p, strings.TrimSuffix(dir, string(filepath.Separator))+string(filepath.Separator))
}

// samePath tells whether a and b name the same path (ignoring case on Windows).
func samePath(a, b string) bool { return within(a, b) && within(b, a) }

// installerFolder returns exe's folder when it is the one install.ps1 uses (BASEKIT_INSTALL_DIR, or
// %LOCALAPPDATA%\Programs\basekit), and whether that folder holds only basekit, so deleting it loses nothing else.
func installerFolder(exe string) (string, bool) {
	dir := filepath.Dir(exe)
	var candidates []string
	if d := os.Getenv("BASEKIT_INSTALL_DIR"); d != "" {
		candidates = append(candidates, d)
	}
	if d := os.Getenv("LOCALAPPDATA"); d != "" {
		candidates = append(candidates, filepath.Join(d, "Programs", "basekit"))
	}
	for _, c := range candidates {
		abs, err := filepath.Abs(c)
		if err != nil {
			continue
		}
		if real, err := filepath.EvalSymlinks(abs); err == nil {
			abs = real
		}
		if samePath(dir, abs) {
			entries, err := os.ReadDir(dir)
			if err != nil {
				return dir, false
			}
			for _, e := range entries {
				// install.ps1 stages its download as .basekit.exe.tmp.<pid> next to the target.
				if !strings.EqualFold(e.Name(), filepath.Base(exe)) && !strings.HasPrefix(e.Name(), ".basekit.exe.tmp.") {
					return dir, false
				}
			}
			return dir, true
		}
	}
	return "", false
}

// addSkills plans the global skills listed in ~/.agents/basekit.json, the links basekit made to them in each
// agent's global skills folder, and the manifest. Nothing the manifest does not list is planned.
func (u *uninstallPlan) addSkills() error {
	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}
	u.home = home
	p := fsx.New(home, true, func(string) {})
	m, err := manifest.ReadAt(p, manifest.GlobalPath)
	if err != nil {
		return err
	}
	if m == nil {
		u.noManifest = true
		return nil
	}
	var skills []string
	for rel := range m.Files() {
		name, ok := skillOf(rel)
		if !ok || p.Escapes(rel) {
			continue
		}
		u.files = append(u.files, filepath.Join(home, filepath.FromSlash(rel)))
		if !slices.Contains(skills, name) {
			skills = append(skills, name)
		}
	}
	slices.Sort(u.files)
	slices.Sort(skills)
	for _, s := range skills {
		u.skillDirs = append(u.skillDirs, filepath.Join(home, filepath.FromSlash(adapters.CanonicalSkillsDir), s))
	}
	for _, a := range adapters.All {
		if a.GlobalSkillsDir == adapters.CanonicalSkillsDir {
			continue
		}
		for _, s := range skills {
			link := filepath.Join(home, filepath.FromSlash(a.GlobalSkillsDir), s)
			if linksTo(link, filepath.Join(home, filepath.FromSlash(adapters.CanonicalSkillsDir), s)) && !slices.Contains(u.links, link) {
				u.links = append(u.links, link)
			}
		}
	}
	u.manifest = filepath.Join(home, filepath.FromSlash(manifest.GlobalPath))
	return nil
}

// skillOf returns the skill a manifest path belongs to: .agents/skills/<skill>/<file…>.
func skillOf(rel string) (string, bool) {
	clean := path.Clean(rel)
	if clean != rel || !strings.HasPrefix(clean, adapters.CanonicalSkillsDir+"/") {
		return "", false
	}
	parts := strings.Split(strings.TrimPrefix(clean, adapters.CanonicalSkillsDir+"/"), "/")
	if len(parts) < 2 || slices.Contains(parts, "..") {
		return "", false
	}
	return parts[0], true
}

// linksTo tells whether link is a symbolic link that resolves to target.
func linksTo(link, target string) bool {
	info, err := os.Lstat(link)
	if err != nil || info.Mode()&fs.ModeSymlink == 0 {
		return false
	}
	a, err1 := filepath.EvalSymlinks(link)
	b, err2 := filepath.EvalSymlinks(target)
	return err1 == nil && err2 == nil && a == b
}

func (u *uninstallPlan) items() []string {
	items := []string{u.exe}
	items = append(items, u.skillDirs...)
	items = append(items, u.links...)
	if u.manifest != "" {
		items = append(items, u.manifest)
	}
	return items
}

// run removes what the plan lists: the global skills first, then the executable.
func (u *uninstallPlan) run(log func(string)) error {
	var errs []error
	for _, link := range u.links {
		errs = append(errs, removeIfPresent(link))
	}
	for _, f := range u.files {
		errs = append(errs, removeIfPresent(f))
	}
	// A skill folder goes only once it is empty: whatever the user added inside it stays.
	for _, f := range u.files {
		pruneEmpty(filepath.Dir(f), filepath.Join(u.home, filepath.FromSlash(adapters.CanonicalSkillsDir)))
	}
	if u.manifest != "" {
		errs = append(errs, removeIfPresent(u.manifest))
	}
	if err := errors.Join(errs...); err != nil {
		return err
	}
	if goos != "windows" {
		return os.Remove(u.exe)
	}
	if u.installDir != "" {
		if err := removeUserPath(u.installDir, log); err != nil {
			return err
		}
		if u.deleteDir {
			return removeAfterExit(u.installDir, true)
		}
	}
	return removeAfterExit(u.exe, false)
}

func removeIfPresent(p string) error {
	if err := os.Remove(p); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return fmt.Errorf("no se pudo borrar %s: %w", p, err)
	}
	return nil
}

// pruneEmpty removes dir and its parents while they are empty, stopping at stop (which stays).
func pruneEmpty(dir, stop string) {
	for within(dir, stop) && filepath.Clean(dir) != filepath.Clean(stop) {
		if os.Remove(dir) != nil {
			return
		}
		dir = filepath.Dir(dir)
	}
}

// otherCopies lists every other basekit executable in PATH, by real path and without duplicates.
func otherCopies(exe string) []string {
	name := "basekit"
	if goos == "windows" {
		name = "basekit.exe"
	}
	var found []string
	for _, dir := range filepath.SplitList(os.Getenv("PATH")) {
		if dir == "" {
			continue
		}
		real, err := filepath.EvalSymlinks(filepath.Join(dir, name))
		if err != nil || samePath(real, exe) || slices.ContainsFunc(found, func(f string) bool { return samePath(f, real) }) {
			continue
		}
		info, err := os.Stat(real)
		if err != nil || !info.Mode().IsRegular() || (goos != "windows" && info.Mode().Perm()&0o111 == 0) {
			continue
		}
		found = append(found, real)
	}
	return found
}
