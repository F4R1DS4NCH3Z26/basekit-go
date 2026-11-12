package cli_test

import (
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"

	"github.com/F4R1DS4NCH3Z26/basekit-go/internal/scaffold"
)

const (
	planHeader     = "Se va a desinstalar:\n"
	askUninstall   = "¿Desinstalar basekit? [s/N]: "
	nothingRemoved = "No se desinstaló nada.\n"
	uninstalled    = "basekit desinstalado. Los proyectos que ya creaste no cambian.\n"
	dryRunNote     = "(dry-run: no se borra nada)\n"
	noGlobalSkills = "= skills globales  (no hay instalación global)\n"
	needsConfirm   = "ERROR: basekit uninstall necesita confirmación: usa -y (o --dry-run para ver qué borraría)\n"
	keepsYours     = "  (solo los archivos de basekit; se conservan los tuyos)"
	goRunRefusal   = "ERROR: basekit uninstall: este basekit corre desde una compilación temporal (go run); no hay nada que desinstalar\n"
)

// fakeBasekit is a fake basekit executable in a scratch folder: the uninstaller is pointed at it (never at the real
// test binary), HOME is a scratch folder, TMPDIR is moved so the fake does not look like a `go run` build, and PATH
// holds only the fake's folder. It returns the fake's real path (symlinks resolved), as the plan prints it.
func fakeBasekit(t *testing.T) string {
	t.Helper()
	fakeHome(t)
	bin := scratch(t)
	exe := filepath.Join(bin, "basekit")
	if err := os.WriteFile(exe, []byte("fake basekit\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	pointAt(t, exe)
	tmp := scratch(t)
	t.Setenv("TMPDIR", tmp)
	t.Setenv("TMP", tmp)
	t.Setenv("TEMP", tmp)
	t.Setenv("PATH", bin)
	real, err := filepath.EvalSymlinks(exe)
	if err != nil {
		t.Fatal(err)
	}
	return real
}

// pointAt makes the uninstaller see exe as the running executable, invoked by that same path.
func pointAt(t *testing.T, exe string) {
	t.Helper()
	prev := scaffold.Executable
	scaffold.Executable = func() (string, error) { return exe, nil }
	t.Cleanup(func() { scaffold.Executable = prev })
	invoke(t, exe)
}

// invoke makes the uninstaller see path as the basekit the user typed (the PATH lookup of os.Args[0]).
func invoke(t *testing.T, path string) {
	t.Helper()
	prev := scaffold.Invoked
	scaffold.Invoked = func() (string, error) { return path, nil }
	t.Cleanup(func() { scaffold.Invoked = prev })
}

func fileExists(path string) bool {
	_, err := os.Lstat(path)
	return err == nil
}

func TestUninstallHelpLine(t *testing.T) {
	r := run(t, "help")
	mustOK(t, r)
	match(t, r.stdout, regexpQuote("\n  basekit uninstall [--skills] [-y] [--dry-run]   Desinstala este basekit (con --skills, también las skills globales)\n"))
}

func TestUninstallTakesNoDirectory(t *testing.T) {
	exe := fakeBasekit(t)
	r := run(t, "uninstall", "x", "-y")
	if r.code != 2 || r.stderr != "ERROR: Argumentos de más: x\n" || !fileExists(exe) {
		t.Fatalf("%+v", r)
	}
}

func TestUninstallAnswerNoKeepsEverything(t *testing.T) {
	for name, answer := range map[string]string{"no": "n\n", "default": "\n"} {
		t.Run(name, func(t *testing.T) {
			exe := fakeBasekit(t)
			r := runTTY(t, answer, "uninstall")
			mustOK(t, r)
			want := planHeader + "  - " + exe + "\n" + askUninstall + nothingRemoved
			if r.stdout != want || r.stderr != "" {
				t.Fatalf("stdout:\n%q\nwant:\n%q\nstderr: %q", r.stdout, want, r.stderr)
			}
			if !fileExists(exe) {
				t.Fatal("answering no must keep the executable")
			}
		})
	}
}

func TestUninstallYesRemovesTheExecutable(t *testing.T) {
	skipOnWindows(t, "un .exe en ejecución se borra al salir")
	exe := fakeBasekit(t)
	r := run(t, "uninstall", "-y")
	mustOK(t, r)
	if want := planHeader + "  - " + exe + "\n" + uninstalled; r.stdout != want || r.stderr != "" {
		t.Fatalf("stdout:\n%q\nwant:\n%q\nstderr: %q", r.stdout, want, r.stderr)
	}
	if fileExists(exe) {
		t.Fatal("-y must remove the executable")
	}
}

func TestUninstallAnswerYesRemovesTheExecutable(t *testing.T) {
	skipOnWindows(t, "un .exe en ejecución se borra al salir")
	exe := fakeBasekit(t)
	r := runTTY(t, "s\n", "uninstall")
	mustOK(t, r)
	if want := planHeader + "  - " + exe + "\n" + askUninstall + uninstalled; r.stdout != want {
		t.Fatalf("stdout:\n%q\nwant:\n%q", r.stdout, want)
	}
	if fileExists(exe) {
		t.Fatal("answering yes must remove the executable")
	}
}

func TestUninstallDryRunChangesNothing(t *testing.T) {
	exe := fakeBasekit(t)
	for _, args := range [][]string{{"uninstall", "--dry-run"}, {"uninstall", "--dry-run", "-y"}} {
		r := run(t, args...)
		mustOK(t, r)
		if want := planHeader + "  - " + exe + "\n" + dryRunNote; r.stdout != want || r.stderr != "" {
			t.Fatalf("%v stdout:\n%q\nwant:\n%q\nstderr: %q", args, r.stdout, want, r.stderr)
		}
		if !fileExists(exe) {
			t.Fatal("--dry-run must keep the executable")
		}
	}
}

func TestUninstallWithoutTerminalNeedsYes(t *testing.T) {
	exe := fakeBasekit(t)
	r := run(t, "uninstall")
	if r.code != 2 || r.stderr != needsConfirm || r.stdout != "" {
		t.Fatalf("%+v", r)
	}
	if !fileExists(exe) {
		t.Fatal("a usage error must remove nothing")
	}
}

func TestUninstallRefusesAGoRunBuild(t *testing.T) {
	t.Run("go-build folder", func(t *testing.T) {
		fakeBasekit(t)
		exe := filepath.Join(scratch(t), "go-build1234", "b001", "exe", "basekit")
		write(t, filepath.Dir(exe), "basekit", "fake\n")
		pointAt(t, exe)
		r := run(t, "uninstall", "-y")
		if r.code != 2 || r.stderr != goRunRefusal || r.stdout != "" || !fileExists(exe) {
			t.Fatalf("%+v", r)
		}
	})
	t.Run("temp folder", func(t *testing.T) {
		exe := fakeBasekit(t)
		t.Setenv("TMPDIR", filepath.Dir(exe))
		t.Setenv("TMP", filepath.Dir(exe))
		t.Setenv("TEMP", filepath.Dir(exe))
		r := run(t, "uninstall", "-y")
		if r.code != 2 || r.stderr != goRunRefusal || r.stdout != "" || !fileExists(exe) {
			t.Fatalf("%+v", r)
		}
	})
}

func TestUninstallSkillsRemovesOnlyWhatBasekitInstalled(t *testing.T) {
	skipOnWindows(t, "crea symlinks")
	home := fakeHome(t)
	mustOK(t, run(t, "init", "--global", "-y", "--no-external-skills", "--agents", "claude-code,opencode,codex"))
	var m map[string]any
	readJSON(t, home, ".agents/basekit.json", &m)
	var listed []string
	skills := map[string]bool{}
	for f := range m["files"].(map[string]any) {
		listed = append(listed, f)
		skills[strings.Split(f, "/")[2]] = true
	}
	if len(skills) == 0 {
		t.Fatal("the global manifest lists no files")
	}
	// The user's own things: a skill, its link, a file added inside a kit skill and a link basekit did not make.
	write(t, home, ".agents/skills/mine/SKILL.md", "mine\n")
	if err := os.Symlink("../../.agents/skills/mine", filepath.Join(home, ".claude/skills/mine")); err != nil {
		t.Fatal(err)
	}
	write(t, home, ".agents/skills/zone-check/notes.md", "mine\n")
	foreign := filepath.Join(home, ".config/opencode/skills/zone-check")
	write(t, home, "elsewhere/zone-check/SKILL.md", "other\n")
	if err := os.Remove(foreign); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(home, "elsewhere/zone-check"), foreign); err != nil {
		t.Fatal(err)
	}

	exe := filepath.Join(scratch(t), "basekit")
	write(t, filepath.Dir(exe), "basekit", "fake\n")
	pointAt(t, exe)
	tmp := scratch(t)
	t.Setenv("TMPDIR", tmp)
	t.Setenv("PATH", filepath.Dir(exe))
	realExe, _ := filepath.EvalSymlinks(exe)

	r := run(t, "uninstall", "--skills", "-y")
	mustOK(t, r)
	var plan strings.Builder
	plan.WriteString(planHeader + "  - " + realExe + "\n")
	var names []string
	for s := range skills {
		names = append(names, s)
	}
	slices.Sort(names)
	for _, s := range names {
		line := "  - " + filepath.Join(home, ".agents/skills", s)
		if s == "zone-check" {
			line += keepsYours // notes.md is the user's
		}
		plan.WriteString(line + "\n")
	}
	for _, dir := range []string{".claude/skills", ".config/opencode/skills"} {
		for _, s := range names {
			if s == "zone-check" {
				continue // its folder keeps notes.md, so its links stay (and opencode's is not basekit's any more)
			}
			plan.WriteString("  - " + filepath.Join(home, dir, s) + "\n")
		}
	}
	plan.WriteString("  - " + filepath.Join(home, ".agents/basekit.json") + "\n")
	if want := plan.String() + uninstalled; r.stdout != want {
		t.Fatalf("stdout:\n%s\nwant:\n%s", r.stdout, want)
	}

	for _, f := range listed {
		if fileExists(filepath.Join(home, f)) {
			t.Errorf("%s must be removed", f)
		}
	}
	for _, s := range names {
		if s == "zone-check" {
			if !fileExists(filepath.Join(home, ".claude/skills", s)) {
				t.Error("link .claude/skills/zone-check must stay: its folder keeps the user's notes.md")
			}
		} else if fileExists(filepath.Join(home, ".claude/skills", s)) {
			t.Errorf("link .claude/skills/%s must be removed", s)
		}
		if s != "zone-check" && fileExists(filepath.Join(home, ".agents/skills", s)) {
			t.Errorf("empty skill folder %s must be removed", s)
		}
	}
	for _, keep := range []string{".agents/skills/mine/SKILL.md", ".claude/skills/mine", ".agents/skills/zone-check/notes.md", ".config/opencode/skills/zone-check", "elsewhere/zone-check/SKILL.md"} {
		if !fileExists(filepath.Join(home, keep)) {
			t.Errorf("%s is not basekit's and must be kept", keep)
		}
	}
	if fileExists(filepath.Join(home, ".agents/basekit.json")) {
		t.Error("the global manifest must be removed")
	}
	if fileExists(exe) {
		t.Error("the executable must be removed")
	}
}

func TestUninstallSkillsDryRunChangesNothing(t *testing.T) {
	skipOnWindows(t, "crea symlinks")
	home := fakeHome(t)
	mustOK(t, run(t, "init", "--global", "-y", "--no-external-skills", "--agents", "claude-code"))
	before := read(t, home, ".agents/basekit.json")
	exe := fakeBasekitIn(t, home)
	r := run(t, "uninstall", "--skills", "--dry-run")
	mustOK(t, r)
	match(t, r.stdout, regexpQuote("  - "+filepath.Join(home, ".claude/skills/zone-check")+"\n"))
	if !strings.HasSuffix(r.stdout, dryRunNote) {
		t.Fatal(r.stdout)
	}
	if read(t, home, ".agents/basekit.json") != before || !exists(home, ".claude/skills/zone-check/SKILL.md") || !fileExists(exe) {
		t.Fatal("--dry-run must change nothing")
	}
}

// fakeBasekitIn is fakeBasekit keeping the HOME already set.
func fakeBasekitIn(t *testing.T, home string) string {
	t.Helper()
	exe := fakeBasekit(t)
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	return exe
}

func TestUninstallSkillsWithoutGlobalManifest(t *testing.T) {
	exe := fakeBasekit(t)
	r := run(t, "uninstall", "--skills", "--dry-run")
	mustOK(t, r)
	if want := planHeader + "  - " + exe + "\n" + noGlobalSkills + dryRunNote; r.stdout != want {
		t.Fatalf("stdout:\n%q\nwant:\n%q", r.stdout, want)
	}
	if runtime.GOOS == "windows" {
		return
	}
	r = run(t, "uninstall", "--skills", "-y")
	mustOK(t, r)
	if want := planHeader + "  - " + exe + "\n" + noGlobalSkills + uninstalled; r.stdout != want {
		t.Fatalf("stdout:\n%q\nwant:\n%q", r.stdout, want)
	}
	if fileExists(exe) {
		t.Fatal("the executable must be removed")
	}
}

func TestUninstallWarnsAboutOtherCopiesInPath(t *testing.T) {
	skipOnWindows(t, "crea symlinks")
	exe := fakeBasekit(t)
	other := filepath.Join(scratch(t), "basekit")
	write(t, filepath.Dir(other), "basekit", "other\n")
	if err := os.Chmod(other, 0o755); err != nil {
		t.Fatal(err)
	}
	linkDir := scratch(t)
	if err := os.Symlink(other, filepath.Join(linkDir, "basekit")); err != nil {
		t.Fatal(err)
	}
	notExecutable := scratch(t)
	write(t, notExecutable, "basekit", "data\n")
	sep := string(os.PathListSeparator)
	t.Setenv("PATH", strings.Join([]string{filepath.Dir(exe), filepath.Dir(other), linkDir, notExecutable, ""}, sep))
	realOther, _ := filepath.EvalSymlinks(other)

	r := run(t, "uninstall", "-y")
	mustOK(t, r)
	want := planHeader + "  - " + exe + "\n" + "! otra copia de basekit en el PATH: " + realOther + " (no se borra)\n" + uninstalled
	if r.stdout != want {
		t.Fatalf("stdout:\n%q\nwant:\n%q", r.stdout, want)
	}
	if fileExists(exe) || !fileExists(other) || !fileExists(filepath.Join(linkDir, "basekit")) {
		t.Fatal("only the running basekit is removed")
	}
}

func TestUninstallThroughALinkRemovesOnlyTheLink(t *testing.T) {
	skipOnWindows(t, "crea symlinks")
	exe := fakeBasekit(t)
	bin := scratch(t)
	link := filepath.Join(bin, "basekit")
	if err := os.Symlink(exe, link); err != nil {
		t.Fatal(err)
	}
	invoke(t, link)
	t.Setenv("PATH", bin)
	plan := planHeader + "  - " + link + "  (enlace; se conserva " + exe + ")\n"

	r := run(t, "uninstall", "--dry-run")
	mustOK(t, r)
	if want := plan + dryRunNote; r.stdout != want || r.stderr != "" {
		t.Fatalf("stdout:\n%q\nwant:\n%q\nstderr: %q", r.stdout, want, r.stderr)
	}
	r = run(t, "uninstall", "-y")
	mustOK(t, r)
	if want := plan + uninstalled; r.stdout != want || r.stderr != "" {
		t.Fatalf("stdout:\n%q\nwant:\n%q\nstderr: %q", r.stdout, want, r.stderr)
	}
	if fileExists(link) {
		t.Fatal("the link must be removed")
	}
	if !fileExists(exe) {
		t.Fatal("the link's target must be kept")
	}
}

func TestUninstallThroughALinkToAnotherFileRemovesTheRunningBinary(t *testing.T) {
	skipOnWindows(t, "crea symlinks")
	exe := fakeBasekit(t)
	other := filepath.Join(scratch(t), "basekit")
	write(t, filepath.Dir(other), "basekit", "other\n")
	link := filepath.Join(scratch(t), "basekit")
	if err := os.Symlink(other, link); err != nil {
		t.Fatal(err)
	}
	invoke(t, link)
	r := run(t, "uninstall", "-y")
	mustOK(t, r)
	if want := planHeader + "  - " + exe + "\n" + uninstalled; r.stdout != want {
		t.Fatalf("stdout:\n%q\nwant:\n%q", r.stdout, want)
	}
	if fileExists(exe) || !fileExists(link) || !fileExists(other) {
		t.Fatal("only the running binary is removed")
	}
}

func TestUninstallSkillsFailureKeepsTheManifestAndTheCLI(t *testing.T) {
	skipOnWindows(t, "usa permisos de Unix")
	if os.Geteuid() == 0 {
		t.Skip("root ignora los permisos")
	}
	home := fakeHome(t)
	mustOK(t, run(t, "init", "--global", "-y", "--no-external-skills", "--agents", "claude-code"))
	var m map[string]any
	readJSON(t, home, ".agents/basekit.json", &m)
	manifestBefore := read(t, home, ".agents/basekit.json")
	exe := fakeBasekitIn(t, home)
	locked := filepath.Join(home, ".agents/skills/zone-check")
	if err := os.Chmod(locked, 0o500); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(locked, 0o755) })
	var stuck []string
	for f := range m["files"].(map[string]any) {
		if p := filepath.Join(home, filepath.FromSlash(f)); filepath.Dir(p) == locked {
			stuck = append(stuck, p)
		}
	}
	if len(stuck) == 0 {
		t.Fatal("zone-check lists no top-level file")
	}
	slices.Sort(stuck)
	var failures strings.Builder
	for _, p := range stuck {
		failures.WriteString("! no se pudo borrar " + p + ": permission denied\n")
	}

	r := run(t, "uninstall", "--skills", "-y")
	if r.code != 1 || r.stderr != "" || !strings.HasPrefix(r.stdout, planHeader+"  - "+exe+"\n") || !strings.HasSuffix(r.stdout, failures.String()) {
		t.Fatalf("code %d\nstdout:\n%s\nwant suffix:\n%s\nstderr: %q", r.code, r.stdout, failures.String(), r.stderr)
	}
	if strings.Contains(r.stdout, uninstalled) {
		t.Fatal("a failed uninstall must not report success")
	}
	if !fileExists(exe) {
		t.Fatal("the CLI must be kept so the user can retry")
	}
	if read(t, home, ".agents/basekit.json") != manifestBefore {
		t.Fatal("the global manifest must be kept")
	}
	for _, p := range stuck {
		if !fileExists(p) {
			t.Errorf("%s could not be removed and must still be there", p)
		}
	}
	if fileExists(filepath.Join(home, ".claude/skills/zone-check")) {
		t.Error("the links that could be removed are removed")
	}
}

func TestUninstallSkillsPlanShowsWhatHappensToEachFolder(t *testing.T) {
	skipOnWindows(t, "crea symlinks")
	home := fakeHome(t)
	mustOK(t, run(t, "init", "--global", "-y", "--no-external-skills", "--agents", "claude-code"))
	var m map[string]any
	readJSON(t, home, ".agents/basekit.json", &m)
	skills := map[string]bool{}
	for f := range m["files"].(map[string]any) {
		skills[strings.Split(f, "/")[2]] = true
		// project-bootstrap: every listed file is already gone and the user keeps a file in its folder.
		if strings.HasPrefix(f, ".agents/skills/project-bootstrap/") {
			if err := os.Remove(filepath.Join(home, filepath.FromSlash(f))); err != nil {
				t.Fatal(err)
			}
		}
	}
	// infra-setup: none of its listed files is left and its folder is empty.
	if err := os.RemoveAll(filepath.Join(home, ".agents/skills/infra-setup")); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(home, ".agents/skills/infra-setup"), 0o755); err != nil {
		t.Fatal(err)
	}
	for _, s := range []string{"infra-setup", "project-bootstrap", "zone-check"} {
		if !skills[s] {
			t.Fatalf("the global install has no %s: %v", s, skills)
		}
	}
	write(t, home, ".agents/skills/project-bootstrap/mine.md", "mine\n")
	write(t, home, ".agents/skills/zone-check/scripts/mine.sh", "mine\n")
	exe := fakeBasekitIn(t, home)

	r := run(t, "uninstall", "--skills", "--dry-run")
	mustOK(t, r)
	var names []string
	for s := range skills {
		names = append(names, s)
	}
	slices.Sort(names)
	var want strings.Builder
	want.WriteString(planHeader + "  - " + exe + "\n")
	for _, s := range names {
		switch s {
		case "infra-setup", "project-bootstrap": // nothing of basekit's is left in them
		case "zone-check":
			want.WriteString("  - " + filepath.Join(home, ".agents/skills", s) + keepsYours + "\n")
		default:
			want.WriteString("  - " + filepath.Join(home, ".agents/skills", s) + "\n")
		}
	}
	// A link goes only with its folder: the folders that stay (untouched or keeping the user's files) keep theirs.
	for _, s := range names {
		if s != "infra-setup" && s != "project-bootstrap" && s != "zone-check" {
			want.WriteString("  - " + filepath.Join(home, ".claude/skills", s) + "\n")
		}
	}
	want.WriteString("  - " + filepath.Join(home, ".agents/basekit.json") + "\n" + dryRunNote)
	if r.stdout != want.String() {
		t.Fatalf("stdout:\n%s\nwant:\n%s", r.stdout, want.String())
	}

	// The plan matches what happens.
	mustOK(t, run(t, "uninstall", "--skills", "-y"))
	for _, s := range names {
		dir := filepath.Join(home, ".agents/skills", s)
		switch s {
		case "infra-setup":
			if !fileExists(dir) {
				t.Error("a folder with none of its listed files left is not touched, even when empty")
			}
		case "project-bootstrap":
			if !exists(home, ".agents/skills/project-bootstrap/mine.md") {
				t.Error("the user's file must be kept")
			}
		case "zone-check":
			if !exists(home, ".agents/skills/zone-check/scripts/mine.sh") || exists(home, ".agents/skills/zone-check/SKILL.md") {
				t.Error("only basekit's files go; the user's stay")
			}
		default:
			if fileExists(dir) {
				t.Errorf("%s must be removed", dir)
			}
		}
		link := filepath.Join(home, ".claude/skills", s)
		_, err := os.Lstat(link)
		switch s {
		case "infra-setup", "project-bootstrap", "zone-check":
			if err != nil {
				t.Errorf("link %s must stay: its folder stays", link)
			}
		default:
			if err == nil {
				t.Errorf("link %s must be removed with its folder", link)
			}
		}
	}
}
