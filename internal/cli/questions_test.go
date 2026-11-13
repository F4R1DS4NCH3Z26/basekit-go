package cli_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// The Docker, Makefile and database questions (S1, S3), with Node's wording.
const (
	askDocker   = "¿Usar Docker? Todo corre en contenedores (Dockerfile y compose) [S/n]: "
	askMakefile = "¿Usar Makefile? Es la interfaz para correr los comandos en los contenedores [S/n]: "
	askDB       = "¿Integrar base de datos? [S/n]: "
	askEngine   = "¿Qué base de datos?"
	noMakefile  = "sin Makefile: usa docker compose directo, crea .env desde .env.example (reemplaza __GENERATE__) y adapta las reglas de AGENTS.md que usan make"
	doctorNoMk  = "·     sin Makefile (manifest): se omiten make y el Makefile"
)

// quiet are the flags that answer every other question, so stdin only answers Docker, Makefile and the database.
func quiet(stack string, extra ...string) []string {
	return append([]string{"--stack", stack, "--no-external-skills", "--agents", "codex", "--no-git", "--context", "x"}, extra...)
}

func ttyInit(t *testing.T, d, stdin, stack string, extra ...string) result {
	t.Helper()
	return runTTY(t, stdin, append([]string{"init", d}, quiet(stack, extra...)...)...)
}

func options(t *testing.T, d string) map[string]any {
	t.Helper()
	return manifest(t, d)["options"].(map[string]any)
}

// S1: yes to the database question offers the stack's engines in order; the chosen one runs infra-setup like --db
// does. mongo-support S1: a stack without dbs offers no MongoDB, and typing it asks again.
func TestDBQuestionYesChoosesEngine(t *testing.T) {
	skipOnWindows(t, "infra-setup es un script bash")
	d := scratch(t)
	r := ttyInit(t, d, "\n\n\nMongoDB\nMySQL\n", "fastapi")
	mustOK(t, r)
	assertOrder(t, r.stdout, askDocker, askMakefile, askDB, askEngine)
	match(t, r.stdout, regexpQuote(askEngine)+`\n  1\) MySQL\n  2\) SQLite\n  3\) PostgreSQL\n[^ ]`)
	noMatch(t, r.stdout, `MongoDB\n`)
	match(t, r.stdout, `«MongoDB» no es una opción válida\.`)
	if got := options(t, d)["db"]; got != "mysql" {
		t.Fatalf("db %v", got)
	}
	if read(t, d, "compose.db.yaml") != read(t, "../../plantillas/skills/infra-setup/assets", "compose.mysql.yaml") {
		t.Fatal("compose.db.yaml is not the mysql overlay")
	}
	match(t, read(t, d, ".make/db.mk"), `mysql -u`)
	match(t, read(t, d, ".env.example"), `(?m)^DB_PASSWORD=__GENERATE__$`)
	match(t, r.stdout, `(?m)^  infra OK: db=mysql redis=0`)

	// A rerun takes the engine from the manifest: no question, and infra-setup does not run again.
	again := runTTY(t, "", "init", d, "--no-external-skills", "--no-git")
	mustOK(t, again)
	noMatch(t, again.stdout, regexpQuote(askDB)+`|infra OK`)
	if got := options(t, d)["db"]; got != "mysql" {
		t.Fatalf("rerun db %v", got)
	}
}

// S1: the default engine is PostgreSQL, and every offered engine is accepted by number or by id.
func TestDBQuestionDefaultAndAnswers(t *testing.T) {
	skipOnWindows(t, "infra-setup es un script bash")
	for answer, want := range map[string]string{"": "postgres", "1": "mysql", "sqlite": "sqlite", "3": "postgres", "mysql": "mysql"} {
		d := scratch(t)
		r := ttyInit(t, d, "\n\n\n"+answer+"\n", "fastapi")
		mustOK(t, r)
		if got := options(t, d)["db"]; got != want {
			t.Fatalf("%q: db %v", answer, got)
		}
		match(t, r.stdout, `(?m)^  infra OK: db=`+want+` `)
	}
}

// S1: no to the database question is `--db none`.
func TestDBQuestionNoMeansNone(t *testing.T) {
	skipOnWindows(t, "infra-setup es un script bash")
	d := scratch(t)
	r := ttyInit(t, d, "\n\nn\n", "fastapi")
	mustOK(t, r)
	match(t, r.stdout, regexpQuote(askDB))
	noMatch(t, r.stdout, regexpQuote(askEngine))
	if got := options(t, d)["db"]; got != "none" {
		t.Fatalf("db %v", got)
	}
	if exists(d, "compose.db.yaml") || exists(d, ".make/db.mk") {
		t.Fatal("database files written after answering no")
	}
	match(t, r.stdout, `(?m)^  infra OK: db=none `)
}

// S1: the database questions are skipped for a frontend stack, with -y, with --db, with a saved db and without Docker.
func TestDBQuestionSkipped(t *testing.T) {
	skipOnWindows(t, "infra-setup es un script bash")
	d := scratch(t)
	r := ttyInit(t, d, "\n\n", "react")
	mustOK(t, r)
	noMatch(t, r.stdout, regexpQuote(askDB))
	if got := options(t, d)["db"]; got != "none" {
		t.Fatalf("frontend db %v", got)
	}

	d = scratch(t)
	r = runWith(t, "", true, append([]string{"init", d, "-y"}, quiet("fastapi")...)...)
	mustOK(t, r)
	noMatch(t, r.stdout, regexpQuote(askDB))

	d = scratch(t)
	r = ttyInit(t, d, "\n\n", "fastapi", "--db", "sqlite")
	mustOK(t, r)
	noMatch(t, r.stdout, regexpQuote(askDB)+`|`+regexpQuote(askEngine))
	if got := options(t, d)["db"]; got != "sqlite" {
		t.Fatalf("--db db %v", got)
	}

	d = scratch(t)
	mustOK(t, initq(t, d, "--stack", "fastapi", "--db", "mysql"))
	r = runTTY(t, "", "init", d, "--no-external-skills", "--no-git")
	mustOK(t, r)
	noMatch(t, r.stdout, regexpQuote(askDB))
	if got := options(t, d)["db"]; got != "mysql" {
		t.Fatalf("saved db %v", got)
	}

	d = scratch(t)
	r = ttyInit(t, d, "n\n", "fastapi")
	mustOK(t, r)
	noMatch(t, r.stdout, regexpQuote(askMakefile)+`|`+regexpQuote(askDB))
	if o := options(t, d); o["docker"] != false || o["makefile"] != false || o["db"] != "auto" {
		t.Fatalf("no Docker: %v", o)
	}
}

// S1: a rerun never asks for the database, even with a saved auto. The agent may have activated the database
// itself with infra.sh (which leaves options.db as auto); asking again would rerun infra-setup over its overlay.
func TestDBQuestionNotAskedOnRerun(t *testing.T) {
	skipOnWindows(t, "infra-setup es un script bash")
	d := scratch(t)
	mustOK(t, initq(t, d, "--stack", "fastapi"))
	if got := options(t, d)["db"]; got != "auto" {
		t.Fatalf("-y db %v", got)
	}
	cmd := exec.Command("bash", ".agents/skills/infra-setup/scripts/infra.sh", "--db", "postgres")
	cmd.Dir = d
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("infra.sh: %v\n%s", err, out)
	}
	overlay, err := os.ReadFile(filepath.Join(d, "compose.db.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	r := runTTY(t, "", "init", d, "--no-external-skills", "--no-git")
	mustOK(t, r)
	noMatch(t, r.stdout, regexpQuote(askDB)+`|`+regexpQuote(askEngine))
	noMatch(t, r.stdout+r.stderr, `(?m)infra OK|ya existe con otro contenido|^\s*[+~] compose\.db\.yaml`)
	if got := options(t, d)["db"]; got != "auto" {
		t.Fatalf("rerun db %v", got)
	}
	if now, _ := os.ReadFile(filepath.Join(d, "compose.db.yaml")); string(now) != string(overlay) {
		t.Fatal("the agent's overlay changed")
	}
}

// mongo-support S1: --db mongo is still a valid value, but a stack without mongo in its dbs rejects it with the S6
// usage error (exit 2, nothing written); in a combined project the backend decides. An unknown value still lists mongo.
func TestDBFlagMongoNeedsStackListing(t *testing.T) {
	const flaskMongo = "ERROR: --db 'mongo' no aplica a flask: motores admitidos: postgres, mysql, sqlite (o auto, none)\n"
	for _, spec := range []string{"flask", "flask+react"} {
		d := scratch(t)
		r := initq(t, d, "--stack", spec, "--db", "mongo")
		if r.code != 2 || r.stderr != flaskMongo || r.stdout != "" || len(entries(t, d)) != 0 {
			t.Fatalf("%s: exit %d stderr %q stdout %q entries %v", spec, r.code, r.stderr, r.stdout, entries(t, d))
		}
	}

	bad := initq(t, scratch(t), "--stack", "fastapi", "--db", "oracle")
	if bad.code != 2 {
		t.Fatalf("exit %d", bad.code)
	}
	if bad.stderr != "ERROR: --db 'oracle' no es válido. Valores: auto, postgres, mysql, sqlite, mongo, none\n" {
		t.Fatalf("stderr %q", bad.stderr)
	}
}

// S3: the Makefile question comes only after a yes to Docker; no means Docker without Makefile, and it is saved.
func TestMakefileQuestionAfterDocker(t *testing.T) {
	skipOnWindows(t, "infra-setup es un script bash")
	d := scratch(t)
	r := ttyInit(t, d, "\nn\nn\n", "django")
	mustOK(t, r)
	assertOrder(t, r.stdout, askDocker, askMakefile, askDB)
	noMatch(t, r.stdout, `Docker \+ Makefile`)
	assertDockerOnly(t, d, r)

	// The saved choice is reused: no question, still no Makefile.
	again := runTTY(t, "", "init", d, "--no-external-skills", "--no-git")
	mustOK(t, again)
	noMatch(t, again.stdout, regexpQuote(askDocker)+`|`+regexpQuote(askMakefile))
	if exists(d, "Makefile") || exists(d, ".make") || options(t, d)["makefile"] != false {
		t.Fatal("rerun ignored the saved makefile: false")
	}
	mustOK(t, initq(t, d))
	if exists(d, "Makefile") || options(t, d)["makefile"] != false {
		t.Fatal("-y rerun ignored the saved makefile: false")
	}
}

// assertDockerOnly checks a single-stack project with Docker and without the Makefile.
func assertDockerOnly(t *testing.T, d string, r result) {
	t.Helper()
	for _, f := range []string{"Makefile", "Makefile.basekit", ".make"} {
		if exists(d, f) {
			t.Fatalf("%s written without Makefile", f)
		}
	}
	for _, f := range []string{"Dockerfile", "compose.yaml", "compose.dev.yaml", "compose.prod.yaml", ".dockerignore", ".env.example"} {
		if !exists(d, f) {
			t.Fatalf("%s missing with Docker", f)
		}
	}
	match(t, r.stdout, `(?m)^! Makefile  \(`+regexpQuote(noMakefile)+`\)$`)
	match(t, r.stdout, `(?m)^Falta la app del framework: créala con las herramientas del stack \(uv\.lock\)\.$`)
	noMatch(t, r.stdout, `make new-app|sin Docker`)
	if o := options(t, d); o["docker"] != true || o["makefile"] != false {
		t.Fatalf("options %v", o)
	}
}

// S3: --no-makefile keeps the Docker files, drops the Makefile and .make/, and infra-setup still runs.
func TestNoMakefileFlag(t *testing.T) {
	d := scratch(t)
	r := initq(t, d, "--stack", "django", "--no-makefile")
	mustOK(t, r)
	assertDockerOnly(t, d, r)

	skipOnWindows(t, "infra-setup es un script bash")
	d = scratch(t)
	r = initq(t, d, "--stack", "django", "--no-makefile", "--db", "postgres")
	mustOK(t, r)
	assertDockerOnly(t, d, r)
	if !exists(d, "compose.db.yaml") {
		t.Fatal("infra-setup did not run without Makefile")
	}
	if exists(d, ".make/db.mk") {
		t.Fatal(".make/db.mk written without Makefile")
	}
}

// S3: a combined project needs the Makefile.
func TestNoMakefileRejectsCombined(t *testing.T) {
	d := scratch(t)
	r := initq(t, d, "--stack", "fastapi+react", "--no-makefile")
	if r.code != 2 {
		t.Fatalf("exit %d", r.code)
	}
	if r.stderr != "ERROR: un proyecto combinado (<backend>+<frontend>) necesita el Makefile: el Makefile raíz delega en cada lado; quita --no-makefile\n" {
		t.Fatalf("stderr %q", r.stderr)
	}
	if len(entries(t, d)) != 0 {
		t.Fatal("wrote files")
	}
	// A combined project never asks the Makefile question and always uses it.
	skipOnWindows(t, "infra-setup es un script bash")
	d = scratch(t)
	c := ttyInit(t, d, "\n", "fastapi+react", "--db", "none")
	mustOK(t, c)
	noMatch(t, c.stdout, regexpQuote(askDocker)+`|`+regexpQuote(askMakefile))
	if options(t, d)["makefile"] != true || !exists(d, "Makefile") {
		t.Fatal("combined without Makefile")
	}
}

// S3: sync keeps makefile false and writes no Makefile; a manifest without the key means true.
func TestSyncRespectsMakefile(t *testing.T) {
	d := scratch(t)
	mustOK(t, initq(t, d, "--stack", "django", "--no-makefile"))
	mustOK(t, run(t, "sync", d, "--no-external-skills"))
	if options(t, d)["makefile"] != false || exists(d, "Makefile") || exists(d, ".make") {
		t.Fatalf("sync changed makefile: %v", options(t, d))
	}

	d = scratch(t)
	mustOK(t, initq(t, d, "--stack", "django"))
	m := read(t, d, ".agents/manifest.json")
	write(t, d, ".agents/manifest.json", strings.Replace(m, `"makefile": true,`, "", 1))
	mustOK(t, run(t, "sync", d, "--no-external-skills"))
	if options(t, d)["makefile"] != true {
		t.Fatalf("missing makefile is not true: %v", options(t, d))
	}

	d = scratch(t)
	mustOK(t, initq(t, d, "--stack", "django", "--no-docker"))
	mustOK(t, run(t, "sync", d, "--no-external-skills"))
	if options(t, d)["makefile"] != false {
		t.Fatalf("no Docker means no Makefile: %v", options(t, d))
	}
}

// S3: doctor skips make, the Makefile and .make/ without a Makefile, and keeps the Docker checks.
func TestDoctorWithoutMakefile(t *testing.T) {
	d := scratch(t)
	mustOK(t, initq(t, d, "--stack", "django", "--no-makefile"))
	r := run(t, "doctor", d)
	match(t, r.stdout, `(?m)^`+regexpQuote(doctorNoMk)+`$`)
	for _, label := range []string{`make`, `Makefile`, `\.make/stack\.mk`} {
		noMatch(t, r.stdout, `(?m)^(OK|FALTA) +`+label+`( |$)`)
	}
	for _, label := range []string{`docker`, `Dockerfile`, `compose\.yaml`, `\.env\.example`, `\.env`} {
		match(t, r.stdout, `(?m)^(OK|FALTA) +`+label+`( |$)`)
	}
	noMatch(t, r.stdout, `sin Docker`)
	// Without a Makefile there is no `make new-app` to suggest (same hint as Node).
	match(t, r.stdout, `crea la app con las herramientas del framework`)
	noMatch(t, r.stdout, `make new-app`)

	// With the Makefile, doctor checks it as before.
	d = scratch(t)
	mustOK(t, initq(t, d, "--stack", "django"))
	r = run(t, "doctor", d)
	noMatch(t, r.stdout, regexp.QuoteMeta(doctorNoMk))
	for _, label := range []string{`make`, `Makefile`, `\.make/stack\.mk`} {
		match(t, r.stdout, `(?m)^(OK|FALTA) +`+label+`( |$)`)
	}
}

// Touched behavior: -y keeps its defaults (db auto, Docker and Makefile on) without asking.
func TestYesDefaultsKeepDockerAndMakefile(t *testing.T) {
	d := scratch(t)
	r := runWith(t, "", true, "init", d, "-y", "--no-external-skills", "--stack", "django")
	mustOK(t, r)
	noMatch(t, r.stdout, regexpQuote(askDocker)+`|`+regexpQuote(askMakefile)+`|`+regexpQuote(askDB)+`|infra OK`)
	if o := options(t, d); o["db"] != "auto" || o["docker"] != true || o["makefile"] != true {
		t.Fatalf("options %v", o)
	}
	for _, f := range []string{"Makefile", ".make/stack.mk", "Dockerfile", "compose.yaml"} {
		if !exists(d, f) {
			t.Fatal(f)
		}
	}
	match(t, r.stdout, "(?m)^Falta la app del framework: ejecuta `make new-app` \\(corre dentro de Docker\\)\\.$")
}

// Touched behavior: --no-docker still writes neither Docker nor Makefile, says so, and saves makefile false.
func TestNoDockerStillSkipsDockerAndMakefile(t *testing.T) {
	d := scratch(t)
	r := initq(t, d, "--stack", "django", "--no-docker")
	mustOK(t, r)
	for _, f := range []string{"Makefile", ".make", "Dockerfile", "compose.yaml", ".env.example"} {
		if exists(d, f) {
			t.Fatalf("%s written with --no-docker", f)
		}
	}
	match(t, r.stdout, `(?m)^! Makefile  \(sin Docker: no se crean Makefile, Dockerfile ni compose; las reglas de AGENTS.md asumen Docker y make, adáptalas\)$`)
	noMatch(t, r.stdout, regexpQuote(noMakefile))
	if o := options(t, d); o["docker"] != false || o["makefile"] != false {
		t.Fatalf("options %v", o)
	}
	doc := run(t, "doctor", d)
	match(t, doc.stdout, `(?m)^·     sin Docker \(manifest\): se omiten Docker, Makefile y \.env$`)
	noMatch(t, doc.stdout, regexpQuote(doctorNoMk))
}
