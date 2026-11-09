package stack

import (
	"bufio"
	"crypto/sha1"
	"encoding/hex"
	"fmt"
	"io/fs"
	"os"
	"path"
	"strings"
	"testing"
)

// Parity with the Node implementation (ADR 0001): the embedded templates must be byte-identical to Node's
// plantillas/, except for how generated text calls the CLI (`npx --no-install basekit` there, `basekit` here).
//
// Choice: a committed manifest (testdata/node-plantillas.tree, the output of
// `git -C <basekit Node> ls-tree -r <rev> plantillas`) instead of reading the Node checkout, so the check is
// hermetic and runs in CI, where the Node repository does not exist. Regenerate it after porting a template change:
//
//	{ echo "# git -C <basekit Node> ls-tree -r <rev> plantillas"; git -C ../guiade-project ls-tree -r <rev> plantillas; } \
//	  > internal/stack/testdata/node-plantillas.tree

// cliText lists, per file, the only allowed difference: the Go text and the Node text it stands for.
var cliText = map[string][2]string{
	"core/AGENTS.block.md":                                            {"usa `basekit sync`", "usa `npx --no-install basekit sync`"},
	"core/agents-README.md":                                           {"con `basekit sync`", "con `npx --no-install basekit sync`"},
	"stacks/django-htmx/files/.make/stack.mk":                         {"(ejecuta basekit init en la raíz del proyecto)", "(ejecuta npx --no-install basekit init en la raíz del proyecto)"},
	"stacks/django/files/.make/stack.mk":                              {"(ejecuta basekit init en la raíz del proyecto)", "(ejecuta npx --no-install basekit init en la raíz del proyecto)"},
	"stacks/express/files/.make/stack.mk":                             {"ejecuta basekit init en", "ejecuta npx --no-install basekit init en"},
	"stacks/fastapi/files/.make/stack.mk":                             {"(ejecuta basekit init en la raíz del proyecto)", "(ejecuta npx --no-install basekit init en la raíz del proyecto)"},
	"stacks/flask/files/.make/stack.mk":                               {"(ejecuta basekit init en la raíz del proyecto)", "(ejecuta npx --no-install basekit init en la raíz del proyecto)"},
	"stacks/gin/files/.make/stack.mk":                                 {"(ejecuta basekit init en la raíz del proyecto)", "(ejecuta npx --no-install basekit init en la raíz del proyecto)"},
	"stacks/go-native/files/.make/stack.mk":                           {"(ejecuta basekit init en la raíz del proyecto)", "(ejecuta npx --no-install basekit init en la raíz del proyecto)"},
	"stacks/node-native/files/.make/stack.mk":                         {"ejecuta basekit init en", "ejecuta npx --no-install basekit init en"},
	"stacks/php-native/files/.make/stack.mk":                          {"(ejecuta basekit init en la raíz del proyecto)", "(ejecuta npx --no-install basekit init en la raíz del proyecto)"},
	"stacks/php-native/skills/phpnative-scaffold/scripts/scaffold.sh": {"¿corriste basekit init?", "¿corriste npx --no-install basekit init?"},
	"stacks/python-native/files/.make/stack.mk":                       {"(ejecuta basekit init en la raíz del proyecto)", "(ejecuta npx --no-install basekit init en la raíz del proyecto)"},
}

// gitBlob is the id git gives to a file's content.
func gitBlob(data []byte) string {
	h := sha1.New()
	fmt.Fprintf(h, "blob %d\x00", len(data))
	h.Write(data)
	return hex.EncodeToString(h.Sum(nil))
}

func nodeTree(t *testing.T) map[string]string {
	t.Helper()
	f, err := os.Open("testdata/node-plantillas.tree")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	blobs := map[string]string{}
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := sc.Text()
		if strings.HasPrefix(line, "#") || line == "" {
			continue
		}
		meta, file, ok := strings.Cut(line, "\t")
		fields := strings.Fields(meta)
		if !ok || len(fields) != 3 || fields[1] != "blob" {
			t.Fatalf("bad tree line: %q", line)
		}
		blobs[strings.TrimPrefix(file, "plantillas/")] = fields[2]
	}
	if err := sc.Err(); err != nil {
		t.Fatal(err)
	}
	return blobs
}

func TestTemplatesMatchNodeExceptCLIText(t *testing.T) {
	node := nodeTree(t)
	seen := map[string]bool{}
	for _, root := range []string{"core", "skills", "stacks"} {
		err := fs.WalkDir(templates, root, func(p string, d fs.DirEntry, err error) error {
			if err != nil || d.IsDir() || path.Base(p) == ".DS_Store" {
				return err
			}
			seen[p] = true
			data, err := fs.ReadFile(templates, p)
			if err != nil {
				return err
			}
			text := string(data)
			if strings.Contains(text, "npx --no-install basekit") {
				t.Errorf("%s calls the CLI as Node does: use `basekit`", p)
			}
			if diff, ok := cliText[p]; ok {
				if n := strings.Count(text, diff[0]); n != 1 {
					t.Errorf("%s: expected the CLI text %q once, found %d", p, diff[0], n)
				}
				text = strings.Replace(text, diff[0], diff[1], 1)
			}
			want, ok := node[p]
			switch {
			case !ok:
				t.Errorf("%s is not in Node's plantillas/", p)
			case gitBlob([]byte(text)) != want:
				t.Errorf("%s differs from Node's plantillas/ (beyond the allowed CLI text)", p)
			}
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	for p := range node {
		if !seen[p] {
			t.Errorf("%s is in Node's plantillas/ but not embedded here", p)
		}
	}
	for p := range cliText {
		if !seen[p] {
			t.Errorf("allowed difference for a missing file: %s", p)
		}
	}
}
