package cli_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

// The Docker follow-ups (odd docker-follow-ups S1–S10), with Node's wording.
const (
	leftoverNote    = "queda de cuando el proyecto usaba Docker: sin Docker no se usa; bórralo si ya no lo necesitas"
	keptDBLine      = "! .agents/manifest.json  (sin Docker no se usan la base de datos ni Redis guardados; se conservan para cuando vuelvas a usar Docker)"
	noDockerLine    = "! Makefile  (sin Docker: no se crean Makefile, Dockerfile ni compose; las reglas de AGENTS.md asumen Docker y make, adáptalas)"
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
	// docker follow-ups S9: Makefile is reported once, by the "sin Docker" line.
	want := []string{".dockerignore", ".env.example", ".make/db.mk", ".make/stack.mk", "Dockerfile", "compose.db.yaml", "compose.dev.yaml", "compose.prod.yaml", "compose.redis.yaml", "compose.yaml"}
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

// makefileLines returns the `!` lines of out for the root Makefile.
func makefileLines(out string) []string {
	lines := []string{}
	for _, l := range actionLines(out, "!") {
		if strings.HasPrefix(l, "! Makefile  (") {
			lines = append(lines, l)
		}
	}
	return lines
}

// indexOf returns the position of line in the `!` lines of out, or -1.
func indexOf(out, line string) int {
	for i, l := range actionLines(out, "!") {
		if l == line {
			return i
		}
	}
	return -1
}

// docker follow-ups S5: a database or Redis saved in the manifest is kept when Docker resolves off, with one warning
// printed after the "sin Docker" line and before the S1b list; infra-setup does not run.
func TestNoDockerKeepsSavedDatabase(t *testing.T) {
	d := scratch(t)
	mustOK(t, initq(t, d, "--stack", "flask"))
	setOptions(t, d, map[string]any{"db": "postgres", "redis": true})
	write(t, d, "compose.db.yaml", "overlay\n")
	r := initq(t, d, "--no-docker")
	mustOK(t, r)
	if n := strings.Count(r.stdout, keptDBLine+"\n"); n != 1 {
		t.Fatalf("warning %d times:\n%s", n, r.stdout)
	}
	noMatch(t, r.stdout, `ejecutaría|falló|infra OK`)
	if read(t, d, "compose.db.yaml") != "overlay\n" {
		t.Fatal("overlay changed")
	}
	if o := options(t, d); o["db"] != "postgres" || o["redis"] != true || o["docker"] != false {
		t.Fatalf("options %v", o)
	}
	want := []string{".dockerignore", ".env.example", ".make/stack.mk", "Dockerfile", "compose.db.yaml", "compose.dev.yaml", "compose.prod.yaml", "compose.yaml"}
	if got := leftovers(r.stdout); !reflect.DeepEqual(got, want) {
		t.Fatalf("warnings %v\nwant %v", got, want)
	}
	first := indexOf(r.stdout, "! .dockerignore  ("+leftoverNote+")")
	if a, b := indexOf(r.stdout, noDockerLine), indexOf(r.stdout, keptDBLine); a < 0 || b != a+1 || first != b+1 {
		t.Fatalf("order: sin Docker %d, manifest %d, first leftover %d\n%s", a, b, first, r.stdout)
	}

	// Docker already off in the manifest: the saved database is kept and warned; the overlays get no S1b line, which
	// belongs only to the run that turns Docker off.
	d = scratch(t)
	mustOK(t, initq(t, d, "--stack", "flask", "--no-docker"))
	setOptions(t, d, map[string]any{"db": "mysql"})
	write(t, d, "compose.db.yaml", "overlay\n")
	r = initq(t, d)
	mustOK(t, r)
	if n := strings.Count(r.stdout, keptDBLine+"\n"); n != 1 {
		t.Fatalf("warning %d times:\n%s", n, r.stdout)
	}
	if got := leftovers(r.stdout); len(got) != 0 {
		t.Fatalf("warnings %v", got)
	}
	if o := options(t, d); o["db"] != "mysql" || o["redis"] != false || o["docker"] != false {
		t.Fatalf("options %v", o)
	}

	// A saved auto or none and redis false are kept, without a warning.
	for _, db := range []string{"auto", "none"} {
		d = scratch(t)
		mustOK(t, initq(t, d, "--stack", "flask", "--no-docker", "--db", db))
		r = initq(t, d)
		mustOK(t, r)
		noMatch(t, r.stdout, regexpQuote(keptDBLine))
		if o := options(t, d); o["db"] != db || o["redis"] != false {
			t.Fatalf("%s: options %v", db, o)
		}
	}
}

// docker follow-ups S5: each value is decided on its own: an explicit --db auto is stored and the saved Redis is kept
// (and warned).
func TestNoDockerExplicitDBAutoKeepsSavedRedis(t *testing.T) {
	d := scratch(t)
	mustOK(t, initq(t, d, "--stack", "flask"))
	setOptions(t, d, map[string]any{"db": "postgres", "redis": true})
	r := initq(t, d, "--no-docker", "--db", "auto")
	mustOK(t, r)
	if n := strings.Count(r.stdout, keptDBLine+"\n"); n != 1 {
		t.Fatalf("warning %d times:\n%s", n, r.stdout)
	}
	if o := options(t, d); o["db"] != "auto" || o["redis"] != true || o["docker"] != false {
		t.Fatalf("options %v", o)
	}
	// An explicit --db with nothing saved to keep prints no warning.
	d = scratch(t)
	mustOK(t, initq(t, d, "--stack", "flask"))
	setOptions(t, d, map[string]any{"db": "postgres"})
	r = initq(t, d, "--no-docker", "--db", "none")
	mustOK(t, r)
	noMatch(t, r.stdout, regexpQuote(keptDBLine))
	if o := options(t, d); o["db"] != "none" || o["redis"] != false {
		t.Fatalf("options %v", o)
	}
}

// docker follow-ups S5: back to Docker with a saved engine or Redis, infra-setup runs with them, as with an explicit --db.
func TestDockerBackRunsInfraWithSavedDatabase(t *testing.T) {
	d := scratch(t)
	mustOK(t, initq(t, d, "--stack", "flask", "--no-docker"))
	setOptions(t, d, map[string]any{"db": "postgres", "redis": true})
	r := initq(t, d, "--docker", "--dry-run")
	mustOK(t, r)
	match(t, r.stdout, `(?m)^· ejecutaría: bash \.agents/skills/infra-setup/scripts/infra\.sh --db postgres --redis$`)

	// A saved none with Redis sets up Redis alone.
	d = scratch(t)
	mustOK(t, initq(t, d, "--stack", "flask", "--no-docker"))
	setOptions(t, d, map[string]any{"db": "none", "redis": true})
	r = initq(t, d, "--docker", "--dry-run")
	mustOK(t, r)
	match(t, r.stdout, `(?m)^· ejecutaría: bash \.agents/skills/infra-setup/scripts/infra\.sh --db none --redis$`)

	// A saved auto is never passed (infra.sh has no auto), with or without Redis.
	for _, redis := range []bool{false, true} {
		d = scratch(t)
		mustOK(t, initq(t, d, "--stack", "flask", "--no-docker"))
		if redis {
			setOptions(t, d, map[string]any{"redis": true})
		}
		r = initq(t, d, "--docker", "--dry-run")
		mustOK(t, r)
		noMatch(t, r.stdout, `ejecutaría: bash`)
		// docker follow-ups S11: with Redis saved, the warning says how to set it up with an engine.
		if n := strings.Count(r.stdout, autoRedisLine(infraPath)+"\n"); (n == 1) != redis {
			t.Fatalf("redis=%v: auto+Redis warning %d times:\n%s", redis, n, r.stdout)
		}
	}

	// Docker kept on: a rerun that takes the db from the manifest does not touch the infrastructure.
	d = scratch(t)
	mustOK(t, initq(t, d, "--stack", "flask"))
	setOptions(t, d, map[string]any{"db": "postgres"})
	r = initq(t, d, "--docker", "--dry-run")
	mustOK(t, r)
	noMatch(t, r.stdout, `ejecutaría: bash`)
}

// docker follow-ups S5: --no-docker then --docker keeps the engine and brings back the overlays deleted after the warning.
func TestDockerRoundTripRestoresDatabase(t *testing.T) {
	skipOnWindows(t, "infra-setup es un script bash")
	d := scratch(t)
	mustOK(t, initq(t, d, "--stack", "flask", "--db", "postgres"))
	overlay := read(t, d, "compose.db.yaml")
	mustOK(t, initq(t, d, "--no-docker"))
	if o := options(t, d); o["db"] != "postgres" || o["docker"] != false {
		t.Fatalf("off: options %v", o)
	}
	for _, f := range []string{"compose.db.yaml", ".make/db.mk"} {
		if err := os.Remove(filepath.Join(d, f)); err != nil {
			t.Fatal(err)
		}
	}
	r := initq(t, d, "--docker")
	mustOK(t, r)
	match(t, r.stdout, `infra OK: db=postgres redis=0`)
	if o := options(t, d); o["db"] != "postgres" || o["redis"] != false || o["docker"] != true || o["makefile"] != true {
		t.Fatalf("on: options %v", o)
	}
	if read(t, d, "compose.db.yaml") != overlay || !exists(d, ".make/db.mk") {
		t.Fatal("overlays not restored")
	}
}

const infraPath = ".agents/skills/infra-setup/scripts/infra.sh"

func autoRedisLine(script string) string {
	return "! " + script + "  (Redis guardado con base de datos auto: infra.sh necesita un motor; ejecútalo con --db <motor> --redis)"
}

// docker follow-ups S11: a failed restore prints the exact command to retry it.
func TestDockerRestoreFailurePrintsRetry(t *testing.T) {
	skipOnWindows(t, "infra-setup es un script bash")
	d := scratch(t)
	mustOK(t, initq(t, d, "--stack", "flask", "--db", "postgres", "--redis"))
	mustOK(t, initq(t, d, "--no-docker"))
	// infra.sh rewrites .env.example; a folder in its place makes it fail.
	if err := os.Remove(filepath.Join(d, ".env.example")); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(d, ".env.example"), 0o755); err != nil {
		t.Fatal(err)
	}
	r := initq(t, d, "--docker")
	want := "! " + infraPath + "  (falló; reinténtalo con: bash " + infraPath + " --db postgres --redis)\n"
	if !strings.Contains(r.stdout, want) {
		t.Fatalf("missing %q in:\n%s", want, r.stdout)
	}
}

// docker follow-ups S11: a single stack with a saved auto and Redis switched to combined never calls infra.sh with auto.
func TestCarriedAutoRedisNeverPassesAuto(t *testing.T) {
	d := scratch(t)
	mustOK(t, initq(t, d, "--stack", "flask", "--redis"))
	r := initq(t, d, "--stack", "flask+react", "--dry-run")
	mustOK(t, r)
	noMatch(t, r.stdout+r.stderr, `--db auto|falló|infra OK`)
	if n := strings.Count(r.stdout, autoRedisLine("backend/"+infraPath)+"\n"); n != 1 {
		t.Fatalf("auto+Redis warning %d times:\n%s", n, r.stdout)
	}
}

// docker follow-ups S11: a frontend switch with --no-docker warns each overlay once, with the frontend reason.
func TestFrontendSwitchNoDockerWarnsOverlaysOnce(t *testing.T) {
	skipOnWindows(t, "infra-setup es un script bash")
	d := scratch(t)
	mustOK(t, initq(t, d, "--stack", "flask", "--db", "postgres", "--redis"))
	r := initq(t, d, "--stack", "react", "--no-docker")
	mustOK(t, r)
	for _, f := range []string{"compose.db.yaml", "compose.redis.yaml"} {
		var got []string
		for _, l := range strings.Split(r.stdout, "\n") {
			if strings.HasPrefix(l, "! "+f+"  (") {
				got = append(got, l)
			}
		}
		want := []string{"! " + f + "  (queda del stack anterior: un stack frontend no usa base de datos; bórralo si ya no lo necesitas)"}
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("%s lines: %q", f, got)
		}
	}
}

// docker follow-ups S9: turning Docker off reports the root Makefile once, with the "sin Docker" line.
func TestDockerOffWarnsMakefileOnce(t *testing.T) {
	d := scratch(t)
	mustOK(t, initq(t, d, "--stack", "flask"))
	r := initq(t, d, "--no-docker")
	mustOK(t, r)
	if got := makefileLines(r.stdout); !reflect.DeepEqual(got, []string{noDockerLine}) {
		t.Fatalf("Makefile lines %q", got)
	}
}

// docker follow-ups S10: a combined project switched to a single stack with --no-docker warns the previous combined
// layout, once per path and in path order; the root Makefile only gets the "sin Docker" line (S9).
func TestCombinedToSingleNoDockerWarnsCombinedLayout(t *testing.T) {
	d := scratch(t)
	mustOK(t, initq(t, d, "--stack", "flask+react"))
	r := initq(t, d, "--stack", "flask", "--no-docker")
	mustOK(t, r)
	if got := makefileLines(r.stdout); !reflect.DeepEqual(got, []string{noDockerLine}) {
		t.Fatalf("Makefile lines %q", got)
	}
	var want []string
	for _, side := range []string{"backend", "frontend"} {
		files := []string{".dockerignore", ".env.example", ".make/stack.mk", "Dockerfile", "Makefile", "compose.dev.yaml", "compose.prod.yaml", "compose.yaml"}
		if side == "frontend" {
			files = append(files, "nginx.conf")
		}
		for _, f := range files {
			want = append(want, side+"/"+f)
		}
	}
	if got := leftovers(r.stdout); !reflect.DeepEqual(got, want) {
		t.Fatalf("warnings %v\nwant %v", got, want)
	}
	for _, f := range append(want, "Makefile") {
		if !exists(d, f) {
			t.Fatalf("%s deleted", f)
		}
	}
	if o := options(t, d); o["docker"] != false {
		t.Fatalf("options %v", o)
	}
}

// docker follow-ups S10: a combined manifest edited to docker false is a Docker project for sync and doctor.
func TestCombinedManifestDockerFalseIsDocker(t *testing.T) {
	d := scratch(t)
	mustOK(t, initq(t, d, "--stack", "flask+react"))
	setOptions(t, d, map[string]any{"docker": false})
	doc := run(t, "doctor", d)
	noMatch(t, doc.stdout, `sin Docker`)
	match(t, doc.stdout, `(?m)^(OK|FALTA) +docker( |$)`)
	mustOK(t, run(t, "sync", d, "--no-external-skills"))
	if o := options(t, d); o["docker"] != true || o["makefile"] != true {
		t.Fatalf("sync: options %v", o)
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
