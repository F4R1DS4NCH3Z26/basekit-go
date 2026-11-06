package basekit_test

import (
	"reflect"
	"regexp"
	"strings"
	"testing"
)

// Question headers, in the order the project needs them (S11).
var questionOrder = []string{"Lenguaje", "Template", "Skills", "Agentes", "Docker + Makefile", "git init", "De qué trata"}

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

func TestWizardAsksEverythingInDependencyOrder(t *testing.T) {
	npx := fakeNpx(t)
	d := scratch(t)
	// language, (template auto), external skills: no, agents: 1, docker: yes, git: yes, context.
	r := runTTY(t, "TypeScript\nn\n1\n\n\nReservas de canchas\n", "init", d)
	mustOK(t, r)
	assertOrder(t, r.stdout, questionOrder...)
	match(t, r.stdout, `Template: nextjs`)
	for _, s := range []string{"project-bootstrap", "infra-setup", "zone-check", "nextjs-patterns", "nextjs-verify"} {
		match(t, r.stdout, `Skills incluidas: .*`+s)
	}
	m := manifest(t, d)
	if m["stack"] != "nextjs" || !reflect.DeepEqual(m["agents"], []any{"claude-code"}) {
		t.Fatal(m)
	}
	if m["options"].(map[string]any)["docker"] != true {
		t.Fatal(m["options"])
	}
	for _, f := range []string{".git", "Makefile", "Dockerfile", ".claude/agents/data-writer.md"} {
		if !exists(d, f) {
			t.Fatal(f)
		}
	}
	match(t, read(t, d, "docs/PROJECT.md"), `Reservas de canchas`)
	if calls := npx(); calls != nil {
		t.Fatalf("external skills declined but npx ran: %v", calls)
	}
}

func TestWizardDefaultsInstallExternalSkillsForChosenAgents(t *testing.T) {
	npx := fakeNpx(t)
	d := scratch(t)
	mustOK(t, runTTY(t, "Python\n\n\n\n\n\n", "init", d))
	calls := argsCalls(npx())
	if len(calls) != 1 {
		t.Fatal(calls)
	}
	match(t, calls[0], ` -a claude-code opencode -y$`)
	if manifest(t, d)["stack"] != "fastapi" {
		t.Fatal(manifest(t, d)["stack"])
	}
}

func TestWizardReasksInvalidAnswers(t *testing.T) {
	fakeNpx(t)
	d := scratch(t)
	r := runTTY(t, "Cobol\n9\n3\nn\n2\n\n\n\n", "init", d)
	mustOK(t, r)
	if n := strings.Count(r.stdout, "Opción no válida"); n != 2 {
		t.Fatalf("%d re-asks:\n%s", n, r.stdout)
	}
	m := manifest(t, d)
	if m["stack"] != "fastapi" || !reflect.DeepEqual(m["agents"], []any{"opencode"}) {
		t.Fatal(m)
	}
}

func TestWizardSkipsWhatFlagsDetectionAndGitAnswer(t *testing.T) {
	fakeNpx(t)
	d := scratch(t)
	write(t, d, "next.config.ts", "export default {};\n")
	sh(t, d, "git", "init", "-q")
	r := runTTY(t, "n\n\n", "init", d, "--agents", "opencode", "--context", "x")
	mustOK(t, r)
	for _, h := range []string{"Lenguaje", "Template", "Agentes", "git init", "De qué trata"} {
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
	for _, h := range questionOrder {
		if strings.Contains(r.stdout, h+":") || strings.Contains(r.stdout, h+"?") {
			t.Fatalf("asked %q with --yes", h)
		}
	}
}

func TestWizardWithoutDockerAndGit(t *testing.T) {
	fakeNpx(t)
	d := scratch(t)
	r := runTTY(t, "Java\nn\n1\nn\nn\n\n", "init", d)
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
