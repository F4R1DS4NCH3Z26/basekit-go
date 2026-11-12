package scaffold

import (
	"os"
	"path/filepath"
	"reflect"
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
	later    []string
}

// asWindows runs uninstall as on Windows with exe as the running basekit, recording the PATH and delete-after-exit
// calls instead of touching the registry or starting cmd.
func asWindows(t *testing.T, exe string) *windowsCalls {
	t.Helper()
	calls := &windowsCalls{}
	prevGOOS, prevExe, prevPath, prevLater := goos, Executable, removeUserPath, removeAfterExit
	goos = "windows"
	Executable = func() (string, error) { return exe, nil }
	removeUserPath = func(dir string, _ func(string)) error { calls.userPath = append(calls.userPath, dir); return nil }
	removeAfterExit = func(target string, isDir bool) error {
		kind := "file"
		if isDir {
			kind = "dir"
		}
		calls.later = append(calls.later, kind+" "+target)
		return nil
	}
	t.Cleanup(func() { goos, Executable, removeUserPath, removeAfterExit = prevGOOS, prevExe, prevPath, prevLater })
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
	if err := Uninstall(UninstallOptions{Yes: true}, &quietUI{}); err != nil {
		t.Fatal(err)
	}
}

func TestWindowsInstallerFolderLeavesPathAndIsDeletedAfterExit(t *testing.T) {
	local := t.TempDir()
	exe := fakeExe(t, filepath.Join(local, "Programs", "basekit"), ".basekit.exe.tmp.123")
	calls := asWindows(t, exe)
	t.Setenv("LOCALAPPDATA", local)
	uninstallYes(t)
	dir := filepath.Dir(exe)
	if !reflect.DeepEqual(calls.userPath, []string{dir}) || !reflect.DeepEqual(calls.later, []string{"dir " + dir}) {
		t.Fatalf("%+v", calls)
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
	if !reflect.DeepEqual(calls.userPath, []string{dir}) || !reflect.DeepEqual(calls.later, []string{"dir " + dir}) {
		t.Fatalf("%+v", calls)
	}
}

func TestWindowsSharedInstallerFolderKeepsTheOtherFiles(t *testing.T) {
	local := t.TempDir()
	exe := fakeExe(t, filepath.Join(local, "Programs", "basekit"), "other-tool.exe")
	calls := asWindows(t, exe)
	t.Setenv("LOCALAPPDATA", local)
	uninstallYes(t)
	if !reflect.DeepEqual(calls.userPath, []string{filepath.Dir(exe)}) || !reflect.DeepEqual(calls.later, []string{"file " + exe}) {
		t.Fatalf("%+v", calls)
	}
}

func TestWindowsOutsideTheInstallerFolderOnlyDeletesTheExe(t *testing.T) {
	exe := fakeExe(t, filepath.Join(t.TempDir(), "bin"))
	calls := asWindows(t, exe)
	t.Setenv("LOCALAPPDATA", t.TempDir())
	uninstallYes(t)
	if calls.userPath != nil || !reflect.DeepEqual(calls.later, []string{"file " + exe}) {
		t.Fatalf("%+v", calls)
	}
}

func TestWindowsLooksForBasekitExeInPath(t *testing.T) {
	exe := fakeExe(t, filepath.Join(t.TempDir(), "bin"))
	asWindows(t, exe)
	other := fakeExe(t, filepath.Join(t.TempDir(), "other"))
	t.Setenv("LOCALAPPDATA", t.TempDir())
	t.Setenv("PATH", filepath.Dir(exe)+string(os.PathListSeparator)+filepath.Dir(other))
	ui := &quietUI{}
	if err := Uninstall(UninstallOptions{Yes: true}, ui); err != nil {
		t.Fatal(err)
	}
	want := "! otra copia de basekit en el PATH: " + other + " (no se borra)"
	if len(ui.lines) < 2 || ui.lines[len(ui.lines)-2] != want {
		t.Fatalf("%q", ui.lines)
	}
}
