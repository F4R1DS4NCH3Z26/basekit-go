package scaffold

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
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

// Invoked returns the basekit the user typed: the PATH lookup of os.Args[0], or os.Args[0] itself when it is a path.
// Tests point it at a fake file or link.
var Invoked = invokedPath

// The platform hooks of uninstall; internal tests replace them so no test touches the registry or starts cmd.
var (
	goos = runtime.GOOS
	// removeUserPath removes dir from the user PATH (Windows only).
	removeUserPath = removeFromUserPath
	// removeAfterExit runs the cmd command once this process has exited, from the folder cwd (Windows only: a running
	// .exe cannot delete itself).
	removeAfterExit = deleteAfterExit
)

// UninstallOptions are the flags of `basekit uninstall`.
type UninstallOptions struct {
	Skills, Yes, DryRun bool
}

// uninstallPlan is everything uninstall removes, in the order it is shown.
type uninstallPlan struct {
	exe string // the running executable, symlinks resolved
	// link is the symbolic link the user invoked basekit through, when it resolves to exe: only the link goes.
	link string
	// On Windows: installDir is the installer's folder holding exe when it holds nothing but basekit, so it leaves the
	// user PATH and is deleted after exit. later is the cmd command that deletes after exit (run from laterCwd).
	installDir, later, laterCwd string
	// The global skills (--skills): the listed files that exist, the skill folders they live in, the links basekit
	// made to those skills, and the manifest. All are absolute.
	home       string
	files      []string
	skillDirs  []skillDir
	links      []string
	manifest   string
	noManifest bool
}

// skillDir is a skill folder holding listed files; keepsYours tells whether it also holds files the manifest does
// not list, which stay (and so does the folder).
type skillDir struct {
	path       string
	keepsYours bool
}

// Uninstall removes the running basekit and, with Skills, the global skills `init --global` installed. It never
// touches the projects basekit created. It returns 1 when some global skill could not be removed: the manifest and
// the CLI then stay, so a retry finds what is still listed.
func Uninstall(o UninstallOptions, ui Prompter) (int, error) {
	exe, err := runningExecutable()
	if err != nil {
		return 1, err
	}
	if !o.Yes && !o.DryRun && !ui.Interactive() {
		return 2, usage.Errorf("basekit uninstall necesita confirmación: usa -y (o --dry-run para ver qué borraría)")
	}
	plan := uninstallPlan{exe: exe, link: invokedLink(exe)}
	if goos == "windows" && plan.link == "" {
		if err := plan.scheduleWindows(); err != nil {
			return 1, err
		}
	}
	if o.Skills {
		if err := plan.addSkills(); err != nil {
			return 1, err
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
		return 0, nil
	}
	if !o.Yes {
		yes, err := ui.Confirm("¿Desinstalar basekit?", false)
		if err != nil {
			return 1, err
		}
		if !yes {
			ui.Log("No se desinstaló nada.")
			return 0, nil
		}
	}
	if !plan.removeSkills(ui.Log) {
		return 1, nil
	}
	if err := plan.removeCLI(ui.Log); err != nil {
		return 1, err
	}
	for _, other := range others {
		ui.Log("! otra copia de basekit en el PATH: " + other + " (no se borra)")
	}
	ui.Log("basekit desinstalado. Los proyectos que ya creaste no cambian.")
	return 0, nil
}

// invokedPath is os.Args[0] as a path: looked up in PATH when it is a bare name.
func invokedPath() (string, error) {
	arg := os.Args[0]
	if !strings.ContainsRune(arg, '/') && !strings.ContainsRune(arg, filepath.Separator) {
		found, err := exec.LookPath(arg)
		if err != nil {
			return "", err
		}
		arg = found
	}
	return filepath.Abs(arg)
}

// invokedLink returns the basekit the user invoked when it is a symbolic link that resolves to exe, or "".
func invokedLink(exe string) string {
	invoked, err := Invoked()
	if err != nil {
		return ""
	}
	info, err := os.Lstat(invoked)
	if err != nil || info.Mode()&fs.ModeSymlink == 0 {
		return ""
	}
	if real, err := filepath.EvalSymlinks(invoked); err != nil || !samePath(real, exe) {
		return ""
	}
	return filepath.Clean(invoked)
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
// %LOCALAPPDATA%\Programs\basekit) and it holds only basekit, so deleting it loses nothing else, with the paths of
// install.ps1's leftover temp files in it. Otherwise it returns "".
func installerFolder(exe string) (string, []string) {
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
		if !samePath(dir, abs) {
			continue
		}
		entries, err := os.ReadDir(dir)
		if err != nil {
			return "", nil
		}
		var temps []string
		for _, e := range entries {
			switch {
			case strings.EqualFold(e.Name(), filepath.Base(exe)):
			// install.ps1 stages its download as .basekit.exe.tmp.<pid> next to the target.
			case strings.HasPrefix(e.Name(), ".basekit.exe.tmp.") && e.Type().IsRegular():
				temps = append(temps, filepath.Join(dir, e.Name()))
			default:
				return "", nil
			}
		}
		return dir, temps
	}
	return "", nil
}

// scheduleWindows plans the delete-after-exit command: the installer's folder (only the files seen now, then a
// non-recursive rmdir, so anything written into it later keeps it) or just the .exe. cmd expands %…% even inside
// quotes, so a path with '%' is refused rather than escaped.
func (u *uninstallPlan) scheduleWindows() error {
	dir, temps := installerFolder(u.exe)
	target := u.exe
	u.later = `del /f /q "` + u.exe + `"`
	if dir != "" {
		target, u.installDir = dir, dir
		for _, t := range temps {
			u.later += ` & del /f /q "` + t + `"`
		}
		u.later += ` & rmdir "` + dir + `"`
	}
	if strings.Contains(u.later, "%") {
		return fmt.Errorf("basekit uninstall: no se puede programar el borrado de %s: cmd expandiría el '%%' de la ruta; bórralo a mano", target)
	}
	// Run from outside the folder being deleted.
	u.laterCwd = filepath.Dir(target)
	return nil
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
	listed := u.files
	u.files = nil
	for _, f := range listed {
		if _, err := os.Lstat(f); err == nil {
			u.files = append(u.files, f)
		}
	}
	// A folder is shown only while it holds a listed file; one with none left is not touched.
	for _, s := range skills {
		dir := filepath.Join(home, filepath.FromSlash(adapters.CanonicalSkillsDir), s)
		if slices.ContainsFunc(u.files, func(f string) bool { return within(f, dir) }) {
			u.skillDirs = append(u.skillDirs, skillDir{path: dir, keepsYours: holdsUnlisted(dir, listed)})
		}
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

// holdsUnlisted tells whether dir holds any file or link (at any depth) that listed does not name.
func holdsUnlisted(dir string, listed []string) bool {
	found := false
	_ = filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			found = true // what cannot be read is not known to be basekit's
			return filepath.SkipAll
		}
		if !d.IsDir() && !slices.Contains(listed, p) {
			found = true
			return filepath.SkipAll
		}
		return nil
	})
	return found
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
	first := u.exe
	if u.link != "" {
		first = u.link + "  (enlace; se conserva " + u.exe + ")"
	}
	items := []string{first}
	for _, d := range u.skillDirs {
		if d.keepsYours {
			items = append(items, d.path+"  (solo los archivos de basekit; se conservan los tuyos)")
		} else {
			items = append(items, d.path)
		}
	}
	items = append(items, u.links...)
	if u.manifest != "" {
		items = append(items, u.manifest)
	}
	return items
}

// removeSkills removes the global skills the plan lists: links, files, the folders left empty and, only when all of
// that worked, the manifest. Each failure is logged; it returns whether everything was removed.
func (u *uninstallPlan) removeSkills(log func(string)) bool {
	ok := true
	remove := func(p string) {
		if err := os.Remove(p); err != nil && !errors.Is(err, fs.ErrNotExist) {
			var pe *fs.PathError
			if errors.As(err, &pe) {
				err = pe.Err
			}
			log("! no se pudo borrar " + p + ": " + err.Error())
			ok = false
		}
	}
	for _, link := range u.links {
		remove(link)
	}
	for _, f := range u.files {
		remove(f)
	}
	// A skill folder goes only once it is empty: whatever the user added inside it stays.
	canonical := filepath.Join(u.home, filepath.FromSlash(adapters.CanonicalSkillsDir))
	for _, f := range u.files {
		pruneEmpty(filepath.Dir(f), canonical)
	}
	for _, d := range u.skillDirs {
		if !d.keepsYours {
			removeEmptyDirs(d.path)
		}
	}
	if ok && u.manifest != "" {
		remove(u.manifest)
	}
	return ok
}

// removeCLI removes the running basekit: the link it was invoked through, the executable itself or, on Windows, the
// delete-after-exit command (and the installer folder's PATH entry when the whole folder goes).
func (u *uninstallPlan) removeCLI(log func(string)) error {
	if u.link != "" {
		return os.Remove(u.link)
	}
	if goos != "windows" {
		return os.Remove(u.exe)
	}
	if u.installDir != "" {
		if err := removeUserPath(u.installDir, log); err != nil {
			return err
		}
	}
	return removeAfterExit(u.laterCwd, u.later)
}

// removeEmptyDirs removes dir and the folders under it that are empty, deepest first; a folder that still holds
// something stays.
func removeEmptyDirs(dir string) {
	var dirs []string
	_ = filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
		if err == nil && d.IsDir() {
			dirs = append(dirs, p)
		}
		return nil
	})
	for i := len(dirs) - 1; i >= 0; i-- {
		_ = os.Remove(dirs[i])
	}
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
