package cli_test

import (
	"fmt"
	"reflect"
	"regexp"
	"slices"
	"strings"
	"testing"
	"unicode/utf8"
)

// The three steps of the stack question (S1b), with Node's wording.
const (
	askKind      = "¿Qué tipo de app es?"
	askLanguage  = "¿En qué lenguaje?"
	askFramework = "¿Con qué framework?"
)

// Question headers, in the order the project needs them (S11, S1b).
var questionOrder = []string{askKind, askLanguage, askFramework, "Skills", "Agentes", "Docker + Makefile", "git init", "De qué trata"}

func assertOrder(t *testing.T, out string, headers ...string) {
	t.Helper()
	last := -1
	for _, h := range headers {
		i := strings.Index(out, h)
		if i < 0 {
			t.Fatalf("question %q not asked:\n%s", h, out)
		}
		if i < last {
			t.Fatalf("question %q out of order:\n%s", h, out)
		}
		last = i
	}
}

// languagesOf are the languages of a kind's stacks, sorted (the order of the language menu).
func languagesOf(kind string) []string {
	var langs []string
	for _, s := range stacksOf(kind) {
		if !slices.Contains(langs, s.Language) {
			langs = append(langs, s.Language)
		}
	}
	slices.Sort(langs)
	return langs
}

func TestWizardAsksEverythingInDependencyOrder(t *testing.T) {
	npx := fakeNpx(t)
	d := scratch(t)
	// kind, language, framework, external skills: no, agents: 1, docker: yes, git: yes, context.
	r := runTTY(t, "1\nPython\nflask\nn\n1\n\n\nReservas de canchas\n", "init", d)
	mustOK(t, r)
	assertOrder(t, r.stdout, questionOrder...)
	for _, s := range []string{"project-bootstrap", "infra-setup", "zone-check"} {
		match(t, r.stdout, `Skills incluidas: .*`+s)
	}
	m := manifest(t, d)
	if m["stack"] != "flask" || !reflect.DeepEqual(m["agents"], []any{"claude-code"}) {
		t.Fatal(m)
	}
	if m["options"].(map[string]any)["docker"] != true {
		t.Fatal(m["options"])
	}
	for _, f := range []string{".git", "Makefile", "Dockerfile"} {
		if !exists(d, f) {
			t.Fatal(f)
		}
	}
	match(t, read(t, d, "docs/PROJECT.md"), `Reservas de canchas`)
	if calls := npx(); calls != nil {
		t.Fatalf("external skills declined but npx ran: %v", calls)
	}
}

// Each step is numbered and filtered by the previous answers; the labels come from stack.json.
func TestWizardStackStepsAreNumberedAndFiltered(t *testing.T) {
	fakeNpx(t)
	r := runTTY(t, "1\nPython\nflask\n", "init", scratch(t), "--no-external-skills", "--agents", "codex", "--no-git", "--context", "x")
	mustOK(t, r)
	match(t, r.stdout, `¿Qué tipo de app es\?\n  1\) Backend +API o servicio, con base de datos\n  2\) Frontend +SPA que corre en el navegador, sin base de datos\n  3\) Fullstack +Una app integrada, o un backend y un frontend combinados\nNúmero o nombre: `)
	for i, l := range languagesOf("backend") {
		match(t, r.stdout, fmt.Sprintf(`(?m)^  %d\) %s\b`, i+1, regexp.QuoteMeta(l)))
	}
	python := stacksOf("backend", "Python")
	width := 0
	for _, s := range python {
		width = max(width, utf8.RuneCountInString(s.Name))
	}
	for i, s := range python {
		pad := strings.Repeat(" ", width-utf8.RuneCountInString(s.Name))
		match(t, r.stdout, fmt.Sprintf(`(?m)^  %d\) %s%s  %s$`, i+1, regexp.QuoteMeta(s.Name), pad, s.ID))
	}
	for _, s := range allStacks {
		if s.Kind != "backend" || s.Language != "Python" {
			noMatch(t, r.stdout, regexp.QuoteMeta(s.Name)+` +`+s.ID+`\n`)
		}
	}
}

func TestWizardAcceptsNumbersAtEveryStep(t *testing.T) {
	fakeNpx(t)
	d := scratch(t)
	lang := slices.Index(languagesOf("backend"), "Go") + 1
	mustOK(t, runTTY(t, fmt.Sprintf("1\n%d\n2\n", lang), "init", d, "--no-external-skills", "--agents", "codex", "--no-git", "--context", "x"))
	if got := manifest(t, d)["stack"]; got != stacksOf("backend", "Go")[1].ID {
		t.Fatal(got)
	}
}

func TestWizardPicksASingleOptionByItself(t *testing.T) {
	fakeNpx(t)
	langs := languagesOf("frontend")
	if len(langs) != 1 {
		t.Fatalf("frontend languages %v", langs)
	}
	d := scratch(t)
	r := runTTY(t, "2\nvue\n", "init", d, "--no-external-skills", "--agents", "codex", "--no-git", "--context", "x")
	mustOK(t, r)
	match(t, r.stdout, `(?m)Lenguaje: `+langs[0]+` \(única opción\)$`)
	noMatch(t, r.stdout, regexp.QuoteMeta(askLanguage))
	if manifest(t, d)["stack"] != "vue" {
		t.Fatal(manifest(t, d)["stack"])
	}

	htmx := stacksOf("fullstack", "Python")
	if len(htmx) != 1 {
		t.Fatalf("fullstack Python stacks %v", idsOf(htmx))
	}
	d = scratch(t)
	r = runTTY(t, "3\nPython\n", "init", d, "--no-external-skills", "--agents", "codex", "--no-git", "--context", "x")
	mustOK(t, r)
	match(t, r.stdout, `(?m)Framework: `+regexp.QuoteMeta(htmx[0].Name)+` \(única opción\)$`)
	noMatch(t, r.stdout, regexp.QuoteMeta(askFramework))
	noMatch(t, r.stdout, `\) Go\b|\) Java\b`) // fullstack only offers its own languages
	if manifest(t, d)["stack"] != htmx[0].ID {
		t.Fatal(manifest(t, d)["stack"])
	}
}

func TestWizardDefaultsInstallExternalSkillsForChosenAgents(t *testing.T) {
	npx := fakeNpx(t)
	d := scratch(t)
	// A stack id typed at the first step skips the other two.
	r := runTTY(t, "fastapi\n\n\n\n\n\n", "init", d)
	mustOK(t, r)
	noMatch(t, r.stdout, regexp.QuoteMeta(askLanguage))
	calls := argsCalls(npx())
	if len(calls) != 1 {
		t.Fatal(calls)
	}
	match(t, calls[0], ` -a claude-code opencode -y$`)
	if manifest(t, d)["stack"] != "fastapi" {
		t.Fatal(manifest(t, d)["stack"])
	}
}

func TestWizardTypedStackIDIgnoresCase(t *testing.T) {
	fakeNpx(t)
	d := scratch(t)
	r := runTTY(t, "Spring\n", "init", d, "--no-external-skills", "--agents", "codex", "--no-git", "--context", "x")
	mustOK(t, r)
	noMatch(t, r.stdout, `no es una opción válida|`+regexp.QuoteMeta(askLanguage))
	if manifest(t, d)["stack"] != "spring" {
		t.Fatal(manifest(t, d)["stack"])
	}
}

func TestWizardReasksInvalidAnswers(t *testing.T) {
	fakeNpx(t)
	d := scratch(t)
	// Invalid or empty answers at every stack step, then an invalid agent.
	r := runTTY(t, "foo\n9\n\nbackend\nCobol\npython\n7\nflask\nn\n9\n2\n\n\n\n", "init", d)
	mustOK(t, r)
	for _, bad := range []string{"foo", "9", "Cobol", "7"} {
		match(t, r.stdout, `  «`+bad+`» no es una opción válida\.`)
	}
	match(t, r.stdout, `(?m)  Hace falta elegir una opción\.$`)
	match(t, r.stdout, `Opción no válida: 9`) // the agents question keeps its own message
	m := manifest(t, d)
	if m["stack"] != "flask" || !reflect.DeepEqual(m["agents"], []any{"opencode"}) {
		t.Fatal(m)
	}
}

func TestWizardFailsWhenInputEndsAtAnyStep(t *testing.T) {
	for _, answers := range []string{"", "foo\n", "1\n", "1\nPython\n"} {
		d := scratch(t)
		r := runTTY(t, answers, "init", d)
		if r.code != 2 {
			t.Fatalf("%q: exit %d", answers, r.code)
		}
		match(t, r.stderr, `^ERROR: No se eligió un stack\. Usa --stack <id>\n$`)
		if len(entries(t, d)) != 0 {
			t.Fatalf("%q: wrote %v", answers, entries(t, d))
		}
	}
}

func TestWizardSkipsWhatFlagsDetectionAndGitAnswer(t *testing.T) {
	fakeNpx(t)
	d := scratch(t)
	write(t, d, "next.config.ts", "export default {};\n")
	sh(t, d, "git", "init", "-q")
	r := runTTY(t, "n\n\n", "init", d, "--agents", "opencode", "--context", "x")
	mustOK(t, r)
	for _, h := range []string{askKind, askLanguage, askFramework, "Agentes", "git init", "De qué trata"} {
		if strings.Contains(r.stdout, h) {
			t.Fatalf("asked %q although it was already answered:\n%s", h, r.stdout)
		}
	}
	assertOrder(t, r.stdout, "Skills", "Docker + Makefile")
}

func TestYesNeverAsks(t *testing.T) {
	fakeNpx(t)
	d := scratch(t)
	r := runWith(t, "", true, "init", d, "--yes", "--no-external-skills", "--stack", "spring")
	mustOK(t, r)
	for _, q := range []string{askKind, askLanguage, askFramework, "Número o nombre", "¿Instalar también", "Elige", "¿Usar Docker", "¿Inicializar git", "¿De qué trata"} {
		if strings.Contains(r.stdout, q) {
			t.Fatalf("asked %q with --yes", q)
		}
	}
}

func TestWizardWithoutDockerAndGit(t *testing.T) {
	fakeNpx(t)
	d := scratch(t)
	r := runTTY(t, "1\nJava\nn\n1\nn\nn\n\n", "init", d)
	mustOK(t, r)
	for _, f := range []string{".git", "Makefile", "Dockerfile", "compose.yaml", ".make/stack.mk", ".env.example"} {
		if exists(d, f) {
			t.Fatalf("%s created without Docker/git", f)
		}
	}
	for _, f := range []string{"AGENTS.md", ".agents/skills/java-verify/SKILL.md", "docs/PROJECT.md", ".gitignore"} {
		if !exists(d, f) {
			t.Fatal(f)
		}
	}
	match(t, r.stdout, `(?m)^! Makefile .*sin Docker`)
	match(t, r.stdout, `(?m)^! \.git .*git init`)
	noMatch(t, r.stdout, `make new-app`)
	if manifest(t, d)["options"].(map[string]any)["docker"] != false {
		t.Fatal(manifest(t, d)["options"])
	}

	// A rerun keeps the choice from the manifest without asking.
	again := runTTY(t, "", "init", d, "--no-git", "--no-external-skills")
	mustOK(t, again)
	if strings.Contains(again.stdout, "Docker + Makefile") || exists(d, "Makefile") {
		t.Fatal("rerun ignored the stored no-Docker choice")
	}
}

func TestNoDockerFlag(t *testing.T) {
	d := scratch(t)
	r := initq(t, d, "--stack", "laravel", "--no-docker")
	mustOK(t, r)
	if exists(d, "Makefile") || exists(d, "Dockerfile") {
		t.Fatal("Docker files created with --no-docker")
	}
	if !exists(d, ".git") {
		t.Fatal("--no-docker must not skip git")
	}
	doc := run(t, "doctor", d)
	for _, label := range []string{"Makefile", "Dockerfile", "compose.yaml", "docker"} {
		if strings.Contains(lineFor(doc.stdout, regexpFor(label)), "FALTA") {
			t.Fatalf("doctor demands %s without Docker:\n%s", label, doc.stdout)
		}
	}
}

func TestNoDockerRejectsDbAndRedis(t *testing.T) {
	for _, extra := range [][]string{{"--db", "postgres"}, {"--redis"}} {
		d := scratch(t)
		r := initq(t, d, append([]string{"--stack", "nextjs", "--no-docker"}, extra...)...)
		if r.code != 2 {
			t.Fatalf("%v: exit %d", extra, r.code)
		}
		match(t, r.stderr, `--no-docker`)
		if len(entries(t, d)) != 0 {
			t.Fatal("wrote files")
		}
	}
}

func TestNoGitFlag(t *testing.T) {
	d := scratch(t)
	r := initq(t, d, "--stack", "nextjs", "--no-git")
	mustOK(t, r)
	if exists(d, ".git") {
		t.Fatal("git init ran with --no-git")
	}
	match(t, r.stdout, `(?m)^! \.git `)
	if !exists(d, "Makefile") {
		t.Fatal("--no-git must not skip Docker")
	}
}

func regexpFor(label string) *regexp.Regexp { return regexp.MustCompile(regexp.QuoteMeta(label)) }
