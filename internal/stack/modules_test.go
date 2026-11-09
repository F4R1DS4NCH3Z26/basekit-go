package stack

import (
	"archive/tar"
	"bytes"
	"flag"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"sort"
	"testing"
	"time"
)

var update = flag.Bool("update", false, "rewrite modules.tar from plantillas/ (make templates-pack)")

const plantillasDir = "../../plantillas"

// packModules builds modules.tar: every file under a folder of plantillas/ that holds its own go.mod (a nested Go
// module, which go:embed cannot include), sorted, with fixed metadata so the archive only changes with the files.
func packModules(t *testing.T) []byte {
	t.Helper()
	var roots, files []string
	err := filepath.WalkDir(plantillasDir, func(p string, d fs.DirEntry, err error) error {
		if err == nil && !d.IsDir() && d.Name() == "go.mod" {
			roots = append(roots, filepath.Dir(p))
		}
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, root := range roots {
		err := filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
			if err == nil && d.Type().IsRegular() && d.Name() != ".DS_Store" {
				rel, _ := filepath.Rel(plantillasDir, p)
				files = append(files, filepath.ToSlash(rel))
			}
			return err
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	sort.Strings(files)
	var buf bytes.Buffer
	w := tar.NewWriter(&buf)
	for _, f := range files {
		data, err := os.ReadFile(filepath.Join(plantillasDir, filepath.FromSlash(f)))
		if err != nil {
			t.Fatal(err)
		}
		hdr := &tar.Header{Name: f, Mode: 0o644, Size: int64(len(data)), ModTime: time.Unix(0, 0), Format: tar.FormatPAX}
		if err := w.WriteHeader(hdr); err != nil {
			t.Fatal(err)
		}
		if _, err := w.Write(data); err != nil {
			t.Fatal(err)
		}
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

// TestModulesArchiveIsCurrent fails when modules.tar no longer matches plantillas/. With -update it only writes the
// archive and returns: the embedded copy in this test binary is the old one, so comparing against it would always
// fail. `make templates-pack` runs it with -update, then once more without it (a fresh build that embeds the new
// archive) to prove the archive is current.
func TestModulesArchiveIsCurrent(t *testing.T) {
	want := packModules(t)
	if *update {
		if err := os.WriteFile("modules.tar", want, 0o644); err != nil {
			t.Fatal(err)
		}
		t.Log("modules.tar rewritten from plantillas/; run the test again without -update to check it")
		return
	}
	if !bytes.Equal(modulesTar, want) {
		t.Fatal("internal/stack/modules.tar is out of date with plantillas/: run make templates-pack")
	}
}

func TestNestedModuleFilesAreServed(t *testing.T) {
	for _, f := range []string{"go.mod", "cmd/api/main.go"} {
		for _, dir := range []string{"stacks/gin/starter", "stacks/go-native/starter"} {
			if !Exists(path.Join(dir, f)) {
				t.Errorf("%s/%s is not served", dir, f)
			}
		}
	}
	if files := Walk("stacks/gin/starter"); len(files) < 5 {
		t.Errorf("Walk sees %d starter files: %v", len(files), files)
	}
}
