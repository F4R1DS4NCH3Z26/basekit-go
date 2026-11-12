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

// pointAt makes the uninstaller see exe as the running executable.
func pointAt(t *testing.T, exe string) {
	t.Helper()
	prev := scaffold.Executable
	scaffold.Executable = func() (string, error) { return exe, nil }
	t.Cleanup(func() { scaffold.Executable = prev })
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
		plan.WriteString("  - " + filepath.Join(home, ".agents/skills", s) + "\n")
	}
	for _, dir := range []string{".claude/skills", ".config/opencode/skills"} {
		for _, s := range names {
			if dir == ".config/opencode/skills" && s == "zone-check" {
				continue // not basekit's link any more
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
		if fileExists(filepath.Join(home, ".claude/skills", s)) {
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
