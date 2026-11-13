package cli_test

import (
	"strings"
	"testing"
)

// springEngines is the usage error for an engine spring's stack.json does not list (S6), byte-identical to Node.
const springEngines = "ERROR: --db 'sqlite' no aplica a spring: motores admitidos: postgres, mysql (o auto, none)\n"

// dropMakefileKey turns a project into a legacy one: a manifest written before options.makefile existed.
func dropMakefileKey(t *testing.T, d string) {
	t.Helper()
	m := read(t, d, ".agents/manifest.json")
	legacy := strings.Replace(m, `"makefile": true,`, "", 1)
	if legacy == m {
		t.Fatal("the manifest has no makefile: true to drop")
	}
	write(t, d, ".agents/manifest.json", legacy)
}

// S5: a legacy manifest (Docker, no makefile key) is not asked the Makefile question on an interactive rerun: it
// means a Makefile, as in sync and doctor.
func TestLegacyManifestKeepsMakefileWithoutAsking(t *testing.T) {
	d := scratch(t)
	mustOK(t, initq(t, d, "--stack", "django"))
	dropMakefileKey(t, d)

	r := runTTY(t, "", "init", d, "--no-external-skills", "--no-git")
	mustOK(t, r)
	noMatch(t, r.stdout, regexpQuote(askDocker)+`|`+regexpQuote(askMakefile))
	noMatch(t, r.stdout, regexpQuote(noMakefile))
	if o := options(t, d); o["docker"] != true || o["makefile"] != true {
		t.Fatalf("options %v", o)
	}
	for _, f := range []string{"Makefile", ".make/stack.mk"} {
		if !exists(d, f) {
			t.Fatalf("%s missing", f)
		}
	}

	dropMakefileKey(t, d)
	mustOK(t, run(t, "sync", d, "--no-external-skills"))
	if options(t, d)["makefile"] != true || !exists(d, "Makefile") {
		t.Fatalf("sync: %v", options(t, d))
	}

	dropMakefileKey(t, d)
	doc := run(t, "doctor", d)
	noMatch(t, doc.stdout, regexpQuote(doctorNoMk))
	for _, label := range []string{`make`, `Makefile`, `\.make/stack\.mk`} {
		match(t, doc.stdout, `(?m)^(OK|FALTA) +`+label+`( |$)`)
	}
}

// S6: the engine menu offers only the stack's engines, in the S1 order, with PostgreSQL as the default.
func TestEngineMenuFollowsStackEngines(t *testing.T) {
	skipOnWindows(t, "infra-setup es un script bash")
	d := scratch(t)
	r := ttyInit(t, d, "\n\n\n\n", "spring")
	mustOK(t, r)
	match(t, r.stdout, regexpQuote(askEngine)+`\n  1\) MySQL\n  2\) PostgreSQL\n[^ ]`)
	noMatch(t, r.stdout, `SQLite|MongoDB`)
	if got := options(t, d)["db"]; got != "postgres" {
		t.Fatalf("default db %v", got)
	}
	match(t, r.stdout, `(?m)^  infra OK: db=postgres `)

	d = scratch(t)
	r = ttyInit(t, d, "\n\n\n1\n", "spring")
	mustOK(t, r)
	if got := options(t, d)["db"]; got != "mysql" {
		t.Fatalf("db by number %v", got)
	}
}

// S6: an explicit --db the stack does not accept is a usage error before anything is written; in a combined project
// the backend's engines decide.
func TestDBFlagOutsideStackEngines(t *testing.T) {
	for _, spec := range []string{"spring", "spring+react"} {
		d := scratch(t)
		r := initq(t, d, "--stack", spec, "--db", "sqlite")
		if r.code != 2 {
			t.Fatalf("%s: exit %d", spec, r.code)
		}
		if r.stderr != springEngines {
			t.Fatalf("%s: stderr %q", spec, r.stderr)
		}
		if r.stdout != "" {
			t.Fatalf("%s: stdout %q", spec, r.stdout)
		}
		if len(entries(t, d)) != 0 {
			t.Fatalf("%s: wrote %v", spec, entries(t, d))
		}
	}
	// The S6 check comes before the "necesitan Docker" one (as in Node).
	d0 := scratch(t)
	nd := initq(t, d0, "--stack", "spring", "--db", "sqlite", "--no-docker")
	if nd.code != 2 || nd.stderr != springEngines || len(entries(t, d0)) != 0 {
		t.Fatalf("--no-docker: exit %d stderr %q", nd.code, nd.stderr)
	}
	// The interactive path rejects it the same way, before any question.
	d := scratch(t)
	r := ttyInit(t, d, "", "spring", "--db", "sqlite")
	if r.code != 2 || r.stderr != springEngines || len(entries(t, d)) != 0 {
		t.Fatalf("tty: exit %d stderr %q entries %v", r.code, r.stderr, entries(t, d))
	}
	noMatch(t, r.stdout, regexpQuote(askDocker))
}

// mongo-support S1: spring no longer lists mongo (JPA/Flyway), so --db mongo is a usage error before anything is written.
func TestSpringRejectsMongo(t *testing.T) {
	d := scratch(t)
	r := initq(t, d, "--stack", "spring", "--db", "mongo")
	want := "ERROR: --db 'mongo' no aplica a spring: motores admitidos: postgres, mysql (o auto, none)\n"
	if r.code != 2 || r.stderr != want || r.stdout != "" || len(entries(t, d)) != 0 {
		t.Fatalf("exit %d stderr %q stdout %q entries %v", r.code, r.stderr, r.stdout, entries(t, d))
	}
}

// S6: an engine the stack lists is accepted, and so are auto and none.
func TestDBFlagInsideStackEngines(t *testing.T) {
	skipOnWindows(t, "infra-setup es un script bash")
	d := scratch(t)
	r := initq(t, d, "--stack", "spring", "--db", "mysql")
	mustOK(t, r)
	if got := options(t, d)["db"]; got != "mysql" {
		t.Fatalf("db %v", got)
	}
	if read(t, d, "compose.db.yaml") != read(t, "../../plantillas/skills/infra-setup/assets", "compose.mysql.yaml") {
		t.Fatal("compose.db.yaml is not the mysql overlay")
	}
	for _, db := range []string{"auto", "none"} {
		d := scratch(t)
		mustOK(t, initq(t, d, "--stack", "spring", "--db", db))
		if got := options(t, d)["db"]; got != db {
			t.Fatalf("%s: db %v", db, got)
		}
	}
	// A stack without a dbs list still accepts sqlite.
	d = scratch(t)
	mustOK(t, initq(t, d, "--stack", "fastapi", "--db", "sqlite"))
	if got := options(t, d)["db"]; got != "sqlite" {
		t.Fatalf("fastapi db %v", got)
	}
}

// S6 (Q7): an engine already saved in the manifest is kept as is, even one the stack no longer lists.
func TestSavedEngineIsNotRevalidated(t *testing.T) {
	d := scratch(t)
	mustOK(t, initq(t, d, "--stack", "spring"))
	m := read(t, d, ".agents/manifest.json")
	saved := strings.Replace(m, `"db": "auto"`, `"db": "sqlite"`, 1)
	if saved == m {
		t.Fatal("the manifest has no db auto to replace")
	}
	write(t, d, ".agents/manifest.json", saved)

	r := initq(t, d)
	mustOK(t, r)
	if got := options(t, d)["db"]; got != "sqlite" {
		t.Fatalf("-y rerun db %v", got)
	}
	r = runTTY(t, "", "init", d, "--no-external-skills", "--no-git")
	mustOK(t, r)
	noMatch(t, r.stdout, regexpQuote(askDB))
	if got := options(t, d)["db"]; got != "sqlite" {
		t.Fatalf("tty rerun db %v", got)
	}
	mustOK(t, run(t, "sync", d, "--no-external-skills"))
	if got := options(t, d)["db"]; got != "sqlite" {
		t.Fatalf("sync db %v", got)
	}
}

// S5 step 2: a combined project always has the Makefile, whatever the manifest says (init, sync and doctor alike).
func TestCombinedAlwaysHasMakefile(t *testing.T) {
	skipOnWindows(t, "infra-setup es un script bash")
	d := scratch(t)
	mustOK(t, initq(t, d, "--stack", "fastapi+react", "--db", "none"))
	off := func() {
		m := read(t, d, ".agents/manifest.json")
		edited := strings.Replace(m, `"makefile": true,`, `"makefile": false,`, 1)
		if edited == m {
			t.Fatal("the manifest has no makefile: true")
		}
		write(t, d, ".agents/manifest.json", edited)
	}
	off()
	mustOK(t, run(t, "sync", d, "--no-external-skills"))
	if options(t, d)["makefile"] != true {
		t.Fatalf("sync: %v", options(t, d))
	}
	off()
	doc := run(t, "doctor", d)
	noMatch(t, doc.stdout, regexpQuote(doctorNoMk))
	match(t, doc.stdout, `(?m)^(OK|FALTA) +Makefile \(raíz\)`)
	mustOK(t, initq(t, d)) // doctor is read-only: the manifest still says false
	if options(t, d)["makefile"] != true || !exists(d, "Makefile") {
		t.Fatalf("init: %v", options(t, d))
	}
}
