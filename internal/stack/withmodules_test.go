package stack

import (
	"archive/tar"
	"bytes"
	"errors"
	"io/fs"
	"strings"
	"testing"
	"testing/fstest"
)

// tarOf packs regular files (and one directory entry, which withModules must skip).
func tarOf(t *testing.T, files map[string]string, order ...string) []byte {
	t.Helper()
	var buf bytes.Buffer
	w := tar.NewWriter(&buf)
	if err := w.WriteHeader(&tar.Header{Name: "dir/", Typeflag: tar.TypeDir, Mode: 0o755}); err != nil {
		t.Fatal(err)
	}
	for _, name := range order {
		data := files[name]
		if err := w.WriteHeader(&tar.Header{Name: name, Mode: 0o644, Size: int64(len(data)), Typeflag: tar.TypeReg}); err != nil {
			t.Fatal(err)
		}
		if _, err := w.Write([]byte(data)); err != nil {
			t.Fatal(err)
		}
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func readAll(t *testing.T, fsys fs.FS) map[string]string {
	t.Helper()
	out := map[string]string{}
	err := fs.WalkDir(fsys, ".", func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		if info.Mode().Perm() != 0o444 {
			t.Errorf("%s: mode %v, want read-only", p, info.Mode().Perm())
		}
		data, err := fs.ReadFile(fsys, p)
		out[p] = string(data)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	return out
}

func mustPanic(t *testing.T, want string, f func()) {
	t.Helper()
	defer func() {
		r := recover()
		if r == nil {
			t.Fatal("no panic")
		}
		if msg, _ := r.(string); !strings.HasPrefix(msg, want) {
			t.Fatalf("panic %v, want prefix %q", r, want)
		}
	}()
	f()
}

func TestWithModulesMergesBaseAndArchive(t *testing.T) {
	base := fstest.MapFS{"a.txt": {Data: []byte("A")}, "sub/b.txt": {Data: []byte("B")}, "over.txt": {Data: []byte("base")}}
	archive := tarOf(t, map[string]string{"mod/go.mod": "module x\n", "over.txt": "archive"}, "mod/go.mod", "over.txt")
	got := readAll(t, withModules(base, archive))
	want := map[string]string{"a.txt": "A", "sub/b.txt": "B", "mod/go.mod": "module x\n", "over.txt": "archive"}
	if len(got) != len(want) {
		t.Fatalf("files %v, want %v", got, want)
	}
	for k, v := range want {
		if got[k] != v {
			t.Errorf("%s = %q, want %q", k, got[k], v)
		}
	}
}

func TestWithModulesWithoutArchiveIsTheBase(t *testing.T) {
	base := fstest.MapFS{"a.txt": {Data: []byte("A")}}
	for _, archive := range [][]byte{nil, tarOf(t, nil)} {
		if got := readAll(t, withModules(base, archive)); len(got) != 1 || got["a.txt"] != "A" {
			t.Errorf("%v", got)
		}
	}
}

func TestWithModulesPanicsOnACorruptArchive(t *testing.T) {
	archive := tarOf(t, map[string]string{"f.txt": strings.Repeat("x", 2000)}, "f.txt")
	// Cut inside the file's data: the dir header, the file header and 100 bytes of its 2000.
	mustPanic(t, "plantillas embebidas ilegibles: ", func() { withModules(fstest.MapFS{}, archive[:512+512+100]) })
	mustPanic(t, "plantillas embebidas ilegibles: ", func() {
		withModules(fstest.MapFS{}, []byte("not a tar archive at all, but long enough to be read as a header block"))
	})
}

// failingFS lists a file it cannot read.
type failingFS struct{ fstest.MapFS }

func (f failingFS) Open(name string) (fs.File, error) {
	if name == "broken.txt" {
		return nil, errors.New("disk on fire")
	}
	return f.MapFS.Open(name)
}

func (f failingFS) ReadFile(name string) ([]byte, error) {
	if name == "broken.txt" {
		return nil, errors.New("disk on fire")
	}
	return f.MapFS.ReadFile(name)
}

func TestWithModulesPanicsWhenTheBaseCannotBeRead(t *testing.T) {
	base := failingFS{fstest.MapFS{"a.txt": {Data: []byte("A")}, "broken.txt": {Data: []byte("B")}}}
	mustPanic(t, "plantillas embebidas ilegibles: disk on fire", func() { withModules(base, nil) })
}

func TestEmbeddedTemplatesIncludeTheModules(t *testing.T) {
	if !Exists("core/Makefile") || !Exists("stacks") {
		t.Fatal("base templates missing")
	}
	r := tar.NewReader(bytes.NewReader(modulesTar))
	n := 0
	for hdr, err := r.Next(); err == nil; hdr, err = r.Next() {
		if hdr.Typeflag == tar.TypeReg && !Exists(hdr.Name) {
			t.Errorf("%s from modules.tar is not served", hdr.Name)
		}
		n++
	}
	if n == 0 {
		t.Fatal("modules.tar is empty")
	}
}
