package scaffold

import (
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"
)

// quietUI answers nothing: uninstall runs with Yes.
type quietUI struct{ lines []string }

func (q *quietUI) Log(line string)                                        { q.lines = append(q.lines, line) }
func (q *quietUI) Interactive() bool                                      { return false }
func (q *quietUI) Select(string, []Option, []string) (int, string, error) { return 0, "", ErrNoAnswer }
func (q *quietUI) ChooseMany(string, []string, []int) ([]int, error)      { return nil, nil }
func (q *quietUI) ChooseNames(string, []string, []int) ([]int, error)     { return nil, nil }
func (q *quietUI) Confirm(string, bool) (bool, error)                     { return false, nil }
func (q *quietUI) Text(string) (string, error)                            { return "", nil }

type windowsCalls struct {
	userPath []string
	later    []string // "<cwd>|<command>" of each delete-after-exit
}

// asWindows runs uninstall as on Windows with exe as the running (and invoked) basekit, recording the PATH and
// delete-after-exit calls instead of touching the registry or starting cmd.
func asWindows(t *testing.T, exe string) *windowsCalls {
	t.Helper()
	calls := &windowsCalls{}
	prevGOOS, prevExe, prevInvoked, prevPath, prevLater := goos, Executable, Invoked, removeUserPath, removeAfterExit
	goos = "windows"
	Executable = func() (string, error) { return exe, nil }
	Invoked = func() (string, error) { return exe, nil }
	removeUserPath = func(dir string, _ func(string)) error { calls.userPath = append(calls.userPath, dir); return nil }
	removeAfterExit = func(cwd, command string) error {
		calls.later = append(calls.later, cwd+"|"+command)
		return nil
	}
	t.Cleanup(func() {
		goos, Executable, Invoked, removeUserPath, removeAfterExit = prevGOOS, prevExe, prevInvoked, prevPath, prevLater
	})
	tmp := t.TempDir()
	t.Setenv("TMPDIR", tmp)
	t.Setenv("TMP", tmp)
	t.Setenv("TEMP", tmp)
	t.Setenv("PATH", "")
	t.Setenv("BASEKIT_INSTALL_DIR", "")
	return calls
}

// fakeExe writes a fake basekit.exe (and extra files) in dir and returns its real path.
func fakeExe(t *testing.T, dir string, extra ...string) string {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, name := range append([]string{"basekit.exe"}, extra...) {
		if err := os.WriteFile(filepath.Join(dir, name), []byte("fake\n"), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	real, err := filepath.EvalSymlinks(filepath.Join(dir, "basekit.exe"))
	if err != nil {
		t.Fatal(err)
	}
	return real
}

func uninstallYes(t *testing.T) {
	t.Helper()
	if code, err := Uninstall(UninstallOptions{Yes: true}, &quietUI{}); code != 0 || err != nil {
		t.Fatal(code, err)
	}
}

// later is the recorded delete-after-exit of command, run from the parent of target.
func later(target, command string) string { return filepath.Dir(target) + "|" + command }

func TestWindowsInstallerFolderLeavesPathAndIsDeletedAfterExit(t *testing.T) {
	local := t.TempDir()
	exe := fakeExe(t, filepath.Join(local, "Programs", "basekit"), ".basekit.exe.tmp.123")
	calls := asWindows(t, exe)
	t.Setenv("LOCALAPPDATA", local)
	uninstallYes(t)
	dir := filepath.Dir(exe)
	// Only the files seen when the plan was made, then a non-recursive rmdir: anything added later keeps the folder.
	command := `del /f /q "` + exe + `" & del /f /q "` + filepath.Join(dir, ".basekit.exe.tmp.123") + `" & rmdir "` + dir + `"`
	if !reflect.DeepEqual(calls.userPath, []string{dir}) || !reflect.DeepEqual(calls.later, []string{later(dir, command)}) {
		t.Fatalf("%+v", calls)
	}
	if strings.Contains(command, "rmdir /s") {
		t.Fatal("the folder must not be removed recursively")
	}
	if _, err := os.Stat(exe); err != nil {
		t.Fatal("a running .exe is deleted after exit, not by basekit itself")
	}
}

func TestWindowsInstallDirFromEnvironment(t *testing.T) {
	exe := fakeExe(t, filepath.Join(t.TempDir(), "tools"))
	calls := asWindows(t, exe)
	t.Setenv("LOCALAPPDATA", t.TempDir())
	t.Setenv("BASEKIT_INSTALL_DIR", strings.ToUpper(filepath.Dir(exe)[:1])+filepath.Dir(exe)[1:]+string(filepath.Separator))
	uninstallYes(t)
	dir := filepath.Dir(exe)
	command := `del /f /q "` + exe + `" & rmdir "` + dir + `"`
	if !reflect.DeepEqual(calls.userPath, []string{dir}) || !reflect.DeepEqual(calls.later, []string{later(dir, command)}) {
		t.Fatalf("%+v", calls)
	}
}

func TestWindowsSharedInstallerFolderKeepsTheOtherFilesAndThePath(t *testing.T) {
	local := t.TempDir()
	exe := fakeExe(t, filepath.Join(local, "Programs", "basekit"), "other-tool.exe")
	calls := asWindows(t, exe)
	t.Setenv("LOCALAPPDATA", local)
	uninstallYes(t)
	if calls.userPath != nil || !reflect.DeepEqual(calls.later, []string{later(exe, `del /f /q "`+exe+`"`)}) {
		t.Fatalf("%+v", calls)
	}
}

func TestWindowsOutsideTheInstallerFolderOnlyDeletesTheExe(t *testing.T) {
	exe := fakeExe(t, filepath.Join(t.TempDir(), "bin"))
	calls := asWindows(t, exe)
	t.Setenv("LOCALAPPDATA", t.TempDir())
	uninstallYes(t)
	if calls.userPath != nil || !reflect.DeepEqual(calls.later, []string{later(exe, `del /f /q "`+exe+`"`)}) {
		t.Fatalf("%+v", calls)
	}
}

func TestWindowsRefusesToScheduleAPathWithPercent(t *testing.T) {
	for name, extra := range map[string][]string{"installer folder": nil, "shared folder": {"other-tool.exe"}} {
		t.Run(name, func(t *testing.T) {
			local := filepath.Join(t.TempDir(), "100%USERNAME%")
			exe := fakeExe(t, filepath.Join(local, "Programs", "basekit"), extra...)
			calls := asWindows(t, exe)
			t.Setenv("LOCALAPPDATA", local)
			target := filepath.Dir(exe)
			if extra != nil {
				target = exe
			}
			for _, o := range []UninstallOptions{{Yes: true}, {DryRun: true}} {
				ui := &quietUI{}
				_, err := Uninstall(o, ui)
				want := "basekit uninstall: no se puede programar el borrado de " + target + ": cmd expandiría el '%' de la ruta; bórralo a mano"
				if err == nil || err.Error() != want || ui.lines != nil {
					t.Fatalf("%+v: %v %q", o, err, ui.lines)
				}
			}
			if calls.userPath != nil || calls.later != nil {
				t.Fatalf("%+v", calls)
			}
			if _, err := os.Stat(exe); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestWindowsThroughALinkRemovesOnlyTheLink(t *testing.T) {
	local := t.TempDir()
	exe := fakeExe(t, filepath.Join(local, "Programs", "basekit"))
	calls := asWindows(t, exe)
	t.Setenv("LOCALAPPDATA", local)
	link := filepath.Join(t.TempDir(), "basekit.exe")
	if err := os.Symlink(exe, link); err != nil {
		t.Skip(err)
	}
	Invoked = func() (string, error) { return link, nil }
	uninstallYes(t)
	if calls.userPath != nil || calls.later != nil {
		t.Fatalf("a link is removed right away and its target keeps its folder and PATH: %+v", calls)
	}
	if _, err := os.Lstat(link); err == nil {
		t.Fatal("the link must be removed")
	}
	if _, err := os.Stat(exe); err != nil {
		t.Fatal(err)
	}
}

func TestWindowsLooksForBasekitExeInPath(t *testing.T) {
	exe := fakeExe(t, filepath.Join(t.TempDir(), "bin"))
	asWindows(t, exe)
	other := fakeExe(t, filepath.Join(t.TempDir(), "other"))
	t.Setenv("LOCALAPPDATA", t.TempDir())
	t.Setenv("PATH", filepath.Dir(exe)+string(os.PathListSeparator)+filepath.Dir(other))
	ui := &quietUI{}
	if _, err := Uninstall(UninstallOptions{Yes: true}, ui); err != nil {
		t.Fatal(err)
	}
	want := "! otra copia de basekit en el PATH: " + other + " (no se borra)"
	if len(ui.lines) < 2 || ui.lines[len(ui.lines)-2] != want {
		t.Fatalf("%q", ui.lines)
	}
}

func TestInvokedPathFollowsArgsZero(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows: PATHEXT")
	}
	prev := os.Args
	t.Cleanup(func() { os.Args = prev })
	bin := t.TempDir()
	if err := os.WriteFile(filepath.Join(bin, "basekit"), []byte("fake\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin)

	os.Args = []string{"basekit"}
	if got, err := invokedPath(); err != nil || got != filepath.Join(bin, "basekit") {
		t.Fatalf("a bare name is looked up in PATH: %q %v", got, err)
	}
	os.Args = []string{filepath.Join(bin, ".", "basekit")}
	if got, err := invokedPath(); err != nil || got != filepath.Join(bin, "basekit") {
		t.Fatalf("a path is used as is: %q %v", got, err)
	}
}
