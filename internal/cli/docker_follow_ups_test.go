package cli_test

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
)

// The Docker follow-ups (odd docker-follow-ups S1–S5), with Node's wording.
const (
	leftoverNote    = "queda de cuando el proyecto usaba Docker: sin Docker no se usa; bórralo si ya no lo necesitas"
	droppedDBLine   = "! .agents/manifest.json  (sin Docker no se usan la base de datos ni Redis guardados: quedan db none y redis false)"
	needsDockerLine = "ERROR: --db y --redis necesitan Docker: no se pueden usar con --no-docker ni sin Docker en el manifest\n"
	bothDockerLine  = "ERROR: Usa --docker o --no-docker, no los dos\n"
)

// flaskDockerFiles are the files a Docker init of flask writes for Docker and Make, in path order.
var flaskDockerFiles = []string{".dockerignore", ".env.example", ".make/stack.mk", "Dockerfile", "Makefile", "compose.dev.yaml", "compose.prod.yaml", "compose.yaml"}

// leftovers returns the paths of the S1b warnings in out, in the order printed.
func leftovers(out string) []string {
	paths := []string{}
	for _, l := range actionLines(out, "!") {
		if path, ok := strings.CutSuffix(strings.TrimPrefix(l, "! "), "  ("+leftoverNote+")"); ok {
			paths = append(paths, path)
		}
	}
	return paths
}

// setOptions rewrites options in the manifest of d: a nil value removes the key.
func setOptions(t *testing.T, d string, opts map[string]any) {
	t.Helper()
	text := read(t, d, ".agents/manifest.json")
	m := manifest(t, d)
	o := m["options"].(map[string]any)
	for k, v := range opts {
		if v == nil {
			delete(o, k)
		} else {
			o[k] = v
		}
	}
	write(t, d, ".agents/manifest.json", toJSONText(t, m))
	if read(t, d, ".agents/manifest.json") == text && len(opts) > 0 {
		t.Fatal("manifest unchanged")
	}
}

// S1a: a manifest without `docker` (written before the option existed) is Docker on and never asked; sync and doctor agree.
func TestLegacyManifestWithoutDockerIsNotAsked(t *testing.T) {
	d := scratch(t)
	mustOK(t, initq(t, d, "--stack", "flask"))
	setOptions(t, d, map[string]any{"docker": nil})
	r := ttyInit(t, d, "n\nn\nn\n", "flask")
	mustOK(t, r)
	noMatch(t, r.stdout, `¿Usar Docker\?`)
	if o := options(t, d); o["docker"] != true || o["makefile"] != true {
		t.Fatalf("options %v", o)
	}
	for _, f := range flaskDockerFiles {
		if !exists(d, f) {
			t.Fatalf("%s missing", f)
		}
	}
	noMatch(t, r.stdout, `queda de cuando`)

	setOptions(t, d, map[string]any{"docker": nil})
	mustOK(t, run(t, "sync", d, "--no-external-skills"))
	if options(t, d)["docker"] != true {
		t.Fatalf("sync: %v", options(t, d))
	}
	setOptions(t, d, map[string]any{"docker": nil})
	doc := run(t, "doctor", d)
	noMatch(t, doc.stdout, `sin Docker`)
	match(t, doc.stdout, `(?m)^(OK|FALTA) +docker( |$)`)
	match(t, doc.stdout, `(?m)^(OK|FALTA) +Dockerfile( |$)`)
}

// Touched behavior: a first interactive run still asks the Docker question, and no keeps Docker off.
func TestFirstRunStillAsksDocker(t *testing.T) {
	d := scratch(t)
	r := ttyInit(t, d, "n\n", "flask")
	mustOK(t, r)
	match(t, r.stdout, regexpQuote(askDocker))
	if o := options(t, d); o["docker"] != false || o["makefile"] != false {
		t.Fatalf("options %v", o)
	}
	if got := leftovers(r.stdout); len(got) != 0 {
		t.Fatalf("first run warned %v", got)
	}
}

// S1b: turning Docker off deletes nothing and warns once per leftover Docker/Make file, in path order.
func TestDockerOffWarnsLeftovers(t *testing.T) {
	d := scratch(t)
	mustOK(t, initq(t, d, "--stack", "flask"))
	for _, f := range []string{"compose.db.yaml", "compose.redis.yaml", ".make/db.mk"} {
		write(t, d, f, "overlay\n")
	}
	r := initq(t, d, "--no-docker")
	mustOK(t, r)
	want := []string{".dockerignore", ".env.example", ".make/db.mk", ".make/stack.mk", "Dockerfile", "Makefile", "compose.db.yaml", "compose.dev.yaml", "compose.prod.yaml", "compose.redis.yaml", "compose.yaml"}
	if got := leftovers(r.stdout); !reflect.DeepEqual(got, want) {
		t.Fatalf("warnings %v\nwant %v\n%s", got, want, r.stdout)
	}
	for _, f := range want {
		if !exists(d, f) {
			t.Fatalf("%s deleted", f)
		}
	}
	if o := options(t, d); o["docker"] != false || o["makefile"] != false {
		t.Fatalf("options %v", o)
	}
	// Keeping Docker off warns nothing.
	again := initq(t, d, "--no-docker")
	mustOK(t, again)
	if got := leftovers(again.stdout); len(got) != 0 {
		t.Fatalf("rerun warned %v", got)
	}
	again = initq(t, d)
	mustOK(t, again)
	if got := leftovers(again.stdout); len(got) != 0 {
		t.Fatalf("rerun from the manifest warned %v", got)
	}
}

// S1b: a user's own Makefile is not basekit's: only Makefile.basekit (listed in files) is warned.
func TestDockerOffKeepsOwnMakefileSilent(t *testing.T) {
	d := scratch(t)
	write(t, d, "Makefile", "own:\n\techo own\n")
	mustOK(t, initq(t, d, "--stack", "flask"))
	if !exists(d, "Makefile.basekit") {
		t.Fatal("Makefile.basekit not written")
	}
	r := initq(t, d, "--no-docker")
	mustOK(t, r)
	got := leftovers(r.stdout)
	want := []string{".dockerignore", ".env.example", ".make/stack.mk", "Dockerfile", "Makefile.basekit", "compose.dev.yaml", "compose.prod.yaml", "compose.yaml"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("warnings %v\nwant %v", got, want)
	}
	if read(t, d, "Makefile") != "own:\n\techo own\n" {
		t.Fatal("own Makefile changed")
	}
}

// S2: --docker and --no-docker together are a usage error before any question or write.
func TestDockerAndNoDockerConflict(t *testing.T) {
	d := scratch(t)
	r := runTTY(t, "", "init", d, "--docker", "--no-docker")
	if r.code != 2 || r.stderr != bothDockerLine || r.stdout != "" {
		t.Fatalf("exit %d stdout %q stderr %q", r.code, r.stdout, r.stderr)
	}
	if len(entries(t, d)) != 0 {
		t.Fatal("wrote files")
	}
	g := run(t, "init", "--global", "--docker")
	if g.code != 2 || g.stderr != "ERROR: --docker no aplica con --global: es de cada proyecto (usa `basekit init` dentro del proyecto)\n" {
		t.Fatalf("--global: exit %d stderr %q", g.code, g.stderr)
	}
	s := run(t, "sync", d, "--docker")
	if s.code != 2 || s.stderr != "ERROR: --docker no aplica a `sync`\n" {
		t.Fatalf("sync: exit %d stderr %q", s.code, s.stderr)
	}
}

// S2: --docker turns Docker back on; the stored makefile false does not count, so the Makefile is asked again.
func TestDockerFlagReenablesDocker(t *testing.T) {
	d := scratch(t)
	mustOK(t, initq(t, d, "--stack", "flask", "--no-docker"))
	r := ttyInit(t, d, "n\n", "flask", "--docker")
	mustOK(t, r)
	match(t, r.stdout, regexpQuote(askMakefile))
	noMatch(t, r.stdout, regexpQuote(askDocker)+`|`+regexpQuote(askDB))
	if o := options(t, d); o["docker"] != true || o["makefile"] != false || o["db"] != "auto" {
		t.Fatalf("options %v", o)
	}
	for _, f := range []string{"Dockerfile", "compose.yaml", ".env.example"} {
		if !exists(d, f) {
			t.Fatalf("%s missing", f)
		}
	}
	if exists(d, "Makefile") || exists(d, ".make") {
		t.Fatal("Makefile written after no")
	}

	// Without a terminal the Makefile is yes.
	d = scratch(t)
	mustOK(t, initq(t, d, "--stack", "flask", "--no-docker"))
	r = initq(t, d, "--docker")
	mustOK(t, r)
	if o := options(t, d); o["docker"] != true || o["makefile"] != true {
		t.Fatalf("options %v", o)
	}
	for _, f := range flaskDockerFiles {
		if !exists(d, f) {
			t.Fatalf("%s missing", f)
		}
	}
	// --no-makefile still wins.
	d = scratch(t)
	mustOK(t, initq(t, d, "--stack", "flask", "--no-docker"))
	r = ttyInit(t, d, "", "flask", "--docker", "--no-makefile")
	mustOK(t, r)
	noMatch(t, r.stdout, regexpQuote(askMakefile))
	if o := options(t, d); o["docker"] != true || o["makefile"] != false {
		t.Fatalf("options %v", o)
	}
}

// S2: on a project that already uses Docker, --docker changes nothing.
func TestDockerFlagOnDockerProjectChangesNothing(t *testing.T) {
	d := scratch(t)
	mustOK(t, runWith(t, "", false, append([]string{"init", d}, quiet("flask", "--no-makefile")...)...))
	before := options(t, d)
	r := ttyInit(t, d, "", "flask", "--docker")
	mustOK(t, r)
	noMatch(t, r.stdout, regexpQuote(askMakefile))
	match(t, r.stdout, `(?m)^Todo al día: no había nada que cambiar\.$`)
	if after := options(t, d); !reflect.DeepEqual(before, after) {
		t.Fatalf("options %v -> %v", before, after)
	}
}

// S3: with Docker off by flag or manifest, --db/--redis fail before any question (the stack one included) or write.
func TestDBNeedsDockerBeforeAnyQuestion(t *testing.T) {
	for _, extra := range [][]string{{"--db", "postgres"}, {"--redis"}} {
		d := scratch(t)
		r := runTTY(t, "", append([]string{"init", d, "--no-docker"}, extra...)...)
		if r.code != 2 || r.stderr != needsDockerLine || r.stdout != "" {
			t.Fatalf("%v: exit %d stdout %q stderr %q", extra, r.code, r.stdout, r.stderr)
		}
		if len(entries(t, d)) != 0 {
			t.Fatal("wrote files")
		}

		d = scratch(t)
		mustOK(t, initq(t, d, "--stack", "flask", "--no-docker"))
		m := read(t, d, ".agents/manifest.json")
		r = runTTY(t, "", append([]string{"init", d}, extra...)...)
		if r.code != 2 || r.stderr != needsDockerLine || r.stdout != "" {
			t.Fatalf("manifest %v: exit %d stdout %q stderr %q", extra, r.code, r.stdout, r.stderr)
		}
		if read(t, d, ".agents/manifest.json") != m {
			t.Fatal("manifest rewritten")
		}
	}
}

// Touched behavior: a stack-specific --db error keeps its precedence over the Docker one.
func TestStackDBErrorStillFirst(t *testing.T) {
	d := scratch(t)
	r := initq(t, d, "--stack", "react", "--no-docker", "--db", "postgres")
	if r.code != 2 {
		t.Fatalf("exit %d", r.code)
	}
	match(t, r.stderr, `^ERROR: --db 'postgres' no aplica a react`)
}

// S3: on a first run, --redis (or a --db engine) implies Docker: the Docker question is skipped, the Makefile one stays.
func TestRedisSkipsDockerQuestion(t *testing.T) {
	d := scratch(t)
	r := ttyInit(t, d, "\nn\n", "flask", "--redis")
	mustOK(t, r)
	noMatch(t, r.stdout, regexpQuote(askDocker))
	match(t, r.stdout, regexpQuote(askMakefile))
	if o := options(t, d); o["docker"] != true || o["makefile"] != true || o["redis"] != true {
		t.Fatalf("options %v", o)
	}
}

// S2, S4: the --docker line and the accurate --no-docker line, one right after the other.
func TestDockerHelpLines(t *testing.T) {
	r := run(t, "help")
	mustOK(t, r)
	want := "\n  --docker                Vuelve a usar Docker en un proyecto creado sin Docker\n" +
		"  --no-docker             Sin Docker ni Makefile (no compatible con --redis, un combinado ni --db distinto de auto/none)\n"
	if !strings.Contains(r.stdout, want) {
		t.Fatalf("help:\n%s", r.stdout)
	}
}

// S5: a database or Redis saved in the manifest is dropped when Docker resolves off, with one warning.
func TestNoDockerDropsSavedDatabase(t *testing.T) {
	d := scratch(t)
	mustOK(t, initq(t, d, "--stack", "flask"))
	setOptions(t, d, map[string]any{"db": "postgres", "redis": true})
	write(t, d, "compose.db.yaml", "overlay\n")
	r := initq(t, d, "--no-docker")
	mustOK(t, r)
	if n := strings.Count(r.stdout, droppedDBLine+"\n"); n != 1 {
		t.Fatalf("warning %d times:\n%s", n, r.stdout)
	}
	noMatch(t, r.stdout, `ejecutaría|falló`)
	if read(t, d, "compose.db.yaml") != "overlay\n" {
		t.Fatal("overlay changed")
	}
	if o := options(t, d); o["db"] != "none" || o["redis"] != false || o["docker"] != false {
		t.Fatalf("options %v", o)
	}
	if got := leftovers(r.stdout); !reflect.DeepEqual(got, []string{".dockerignore", ".env.example", ".make/stack.mk", "Dockerfile", "Makefile", "compose.db.yaml", "compose.dev.yaml", "compose.prod.yaml", "compose.yaml"}) {
		t.Fatalf("warnings %v", got)
	}

	// Docker already off in the manifest: the saved database is dropped and the overlays still get the warning.
	d = scratch(t)
	mustOK(t, initq(t, d, "--stack", "flask", "--no-docker"))
	setOptions(t, d, map[string]any{"db": "mysql"})
	write(t, d, "compose.db.yaml", "overlay\n")
	r = initq(t, d)
	mustOK(t, r)
	if n := strings.Count(r.stdout, droppedDBLine+"\n"); n != 1 {
		t.Fatalf("warning %d times:\n%s", n, r.stdout)
	}
	if got := leftovers(r.stdout); !reflect.DeepEqual(got, []string{"compose.db.yaml"}) {
		t.Fatalf("warnings %v", got)
	}
	if o := options(t, d); o["db"] != "none" || o["redis"] != false {
		t.Fatalf("options %v", o)
	}

	// A saved auto is kept, without a warning.
	d = scratch(t)
	mustOK(t, initq(t, d, "--stack", "flask", "--no-docker"))
	r = initq(t, d)
	mustOK(t, r)
	noMatch(t, r.stdout, regexpQuote(droppedDBLine))
	if o := options(t, d); o["db"] != "auto" {
		t.Fatalf("options %v", o)
	}
}

func toJSONText(t *testing.T, v any) string {
	t.Helper()
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	return string(b) + "\n"
}
