package cli_test

import (
	"reflect"
	"regexp"
	"strings"
	"testing"
)

// Optional choices (odd/tasks/optional-choices.md): the external skills are a list (S1) and the agents are optional (S2).

const askExternal = "¿Qué skills externas instalo? (usan npx y red)"

// pinnedWith is the pinned `npx skills add` call for some skills and agents.
func pinnedWith(skills, agents []string) string {
	base := strings.SplitN(pinned, " -s ", 2)[0]
	return "ARGS " + base + " -s " + strings.Join(skills, " ") + " -a " + strings.Join(agents, " ") + " -y"
}

func externalOption(t *testing.T, dir string) (any, bool) {
	t.Helper()
	v, ok := manifest(t, dir)["options"].(map[string]any)["externalSkills"]
	return v, ok
}

// S1 · The yes/no question is now a numbered list of the four pinned skills, after «Skills incluidas».
func TestExternalSkillsQuestionIsAList(t *testing.T) {
	npx := fakeNpx(t)
	d := scratch(t)
	// stack, external skills: Enter (all), agents, docker, git, context.
	r := runTTY(t, "fastapi\n\ncodex\n\n\n\n", "init", d)
	mustOK(t, r)
	match(t, r.stdout, `Skills incluidas: [^\n]+\n`+regexp.QuoteMeta(askExternal)+`\n  1\) grill-me\n  2\) grill-with-docs\n  3\) grilling\n  4\) domain-modeling\n`+
		regexp.QuoteMeta(`Números o nombres [grill-me,grill-with-docs,grilling,domain-modeling]: `))
	noMatch(t, r.stdout, `¿Instalar también`)
	if got := argsCalls(npx()); !reflect.DeepEqual(got, []string{pinnedWith(externalSkills, []string{"codex"})}) {
		t.Fatalf("%q", got)
	}
	match(t, r.stdout, `(?m)^\+ skills externas  \(grill-me, grill-with-docs, grilling, domain-modeling\)$`)
	if v, _ := externalOption(t, d); !reflect.DeepEqual(v, []any{"grill-me", "grill-with-docs", "grilling", "domain-modeling"}) {
		t.Fatalf("options.externalSkills %v", v)
	}
}

// S1 · Numbers or names separated by commas choose a subset: only those are installed and persisted.
func TestExternalSkillsSubsetByNumbersOrNames(t *testing.T) {
	for _, answer := range []string{"3,4", "grilling, Domain-Modeling"} {
		npx := fakeNpx(t)
		d := scratch(t)
		r := runTTY(t, "fastapi\n"+answer+"\ncodex\n\n\n\n", "init", d)
		mustOK(t, r)
		chosen := []string{"grilling", "domain-modeling"}
		if got := argsCalls(npx()); !reflect.DeepEqual(got, []string{pinnedWith(chosen, []string{"codex"})}) {
			t.Fatalf("%q: %q", answer, got)
		}
		match(t, r.stdout, `(?m)^\+ skills externas  \(grilling, domain-modeling\)$`)
		for _, s := range []string{"grill-me", "grill-with-docs"} {
			if exists(d, ".agents/skills/"+s) {
				t.Fatalf("%q: %s installed without being chosen", answer, s)
			}
		}
		if v, _ := externalOption(t, d); !reflect.DeepEqual(v, []any{"grilling", "domain-modeling"}) {
			t.Fatalf("%q: options.externalSkills %v", answer, v)
		}
	}
}

// S1 · An unknown answer is named and the list asked again; the agents question keeps its own format.
func TestExternalSkillsReasksUnknownAnswer(t *testing.T) {
	npx := fakeNpx(t)
	d := scratch(t)
	r := runTTY(t, "fastapi\n1,foo,bar\ngrilling\ncodex\n\n\n\n", "init", d)
	mustOK(t, r)
	match(t, r.stdout, `  No reconozco: foo, bar\. Usa los números o los nombres de la lista\.\n`)
	if got := argsCalls(npx()); !reflect.DeepEqual(got, []string{pinnedWith([]string{"grilling"}, []string{"codex"})}) {
		t.Fatalf("%q", got)
	}
	match(t, r.stdout, `¿Con qué herramientas vas a trabajar\?:\n`)
	match(t, r.stdout, regexp.QuoteMeta(`Elige uno o varios separados por coma [1,2]: `))
}

// S1 · `ninguna`/`ninguno` (any case) chooses none: no npx, the `=` report and an empty persisted list.
func TestExternalSkillsNoneChosen(t *testing.T) {
	for _, answer := range []string{"ninguna", "NINGUNO"} {
		npx := fakeNpx(t)
		d := scratch(t)
		r := runTTY(t, "fastapi\n"+answer+"\ncodex\n\n\n\n", "init", d)
		mustOK(t, r)
		if got := npx(); got != nil {
			t.Fatalf("%q: npx ran %q", answer, got)
		}
		match(t, r.stdout, `(?m)^= skills externas  \(ninguna elegida\)$`)
		if v, ok := externalOption(t, d); !ok || !reflect.DeepEqual(v, []any{}) {
			t.Fatalf("%q: options.externalSkills %v (present %v)", answer, v, ok)
		}
	}
}

// S1 · Reruns of init and sync use the persisted choice: no question again, and only the chosen names are checked
// and passed to -s.
func TestExternalSkillsPersistedChoiceDrivesReruns(t *testing.T) {
	npx := fakeNpx(t)
	d := scratch(t)
	mustOK(t, runTTY(t, "fastapi\ngrilling\ncodex\n\n\n\n", "init", d))
	again := runTTY(t, "", "init", d)
	mustOK(t, again)
	noMatch(t, again.stdout, regexp.QuoteMeta(askExternal))
	match(t, again.stdout, `(?m)^= skills externas  \(grilling\)$`)
	if n := len(argsCalls(npx())); n != 1 {
		t.Fatalf("%d npx calls: the chosen skill was already installed", n)
	}

	// sync adding an agent reinstalls only the chosen skill.
	mustOK(t, run(t, "sync", d, "--agents", "codex,claude-code"))
	calls := argsCalls(npx())
	if len(calls) != 2 || calls[1] != pinnedWith([]string{"grilling"}, []string{"claude-code", "codex"}) {
		t.Fatalf("%q", calls)
	}
	if v, _ := externalOption(t, d); !reflect.DeepEqual(v, []any{"grilling"}) {
		t.Fatalf("options.externalSkills %v", v)
	}
}

// S1 · A persisted empty choice is never asked again and installs nothing, in init and sync.
func TestExternalSkillsPersistedNoneIsKept(t *testing.T) {
	npx := fakeNpx(t)
	d := scratch(t)
	mustOK(t, runTTY(t, "fastapi\nninguna\ncodex\n\n\n\n", "init", d))
	again := runTTY(t, "", "init", d)
	mustOK(t, again)
	noMatch(t, again.stdout, regexp.QuoteMeta(askExternal))
	match(t, again.stdout, `(?m)^= skills externas  \(ninguna elegida\)$`)
	s := run(t, "sync", d, "--agents", "codex,opencode")
	mustOK(t, s)
	match(t, s.stdout, `(?m)^= skills externas  \(ninguna elegida\)$`)
	if got := npx(); got != nil {
		t.Fatalf("npx ran %q", got)
	}
}

// S1 · --no-external-skills skips this run and keeps the persisted choice.
func TestNoExternalSkillsKeepsPersistedChoice(t *testing.T) {
	npx := fakeNpx(t)
	d := scratch(t)
	mustOK(t, runTTY(t, "fastapi\n2\ncodex\n\n\n\n", "init", d))
	mustOK(t, run(t, "init", d, "-y", "--no-external-skills"))
	mustOK(t, run(t, "sync", d, "--no-external-skills", "--agents", "codex,claude-code"))
	if v, _ := externalOption(t, d); !reflect.DeepEqual(v, []any{"grill-with-docs"}) {
		t.Fatalf("options.externalSkills %v", v)
	}
	if n := len(argsCalls(npx())); n != 1 {
		t.Fatalf("%d npx calls", n)
	}
}

// S1 · Previous behavior: a manifest without options.externalSkills means all four skills, and a non-interactive
// init does not add the key.
func TestManifestWithoutExternalChoiceMeansAllFour(t *testing.T) {
	npx := fakeNpx(t)
	d := scratch(t)
	mustOK(t, initq(t, d, "--stack", "nextjs", "--agents", "codex"))
	if _, ok := externalOption(t, d); ok {
		t.Fatal("a non-interactive init must not persist options.externalSkills")
	}
	r := run(t, "sync", d, "--agents", "codex,opencode")
	mustOK(t, r)
	if got := argsCalls(npx()); !reflect.DeepEqual(got, []string{pinnedWith(externalSkills, []string{"opencode", "codex"})}) {
		t.Fatalf("%q", got)
	}
	match(t, r.stdout, `(?m)^\+ skills externas  \(grill-me, grill-with-docs, grilling, domain-modeling\)$`)
}

// S2 · `ninguno`/`ninguna` in the agents question: no adapters or skill links, AGENTS.md and .agents/ as usual,
// `agents: []` in the manifest, and the external skills are not installed (they need -a).
func TestAgentsNoneChosenInTheQuestion(t *testing.T) {
	for _, answer := range []string{"ninguno", "Ninguna"} {
		npx := fakeNpx(t)
		d := scratch(t)
		// stack, external skills: Enter, agents: none, docker, git, context.
		r := runTTY(t, "fastapi\n\n"+answer+"\n\n\n\n", "init", d)
		mustOK(t, r)
		assertNoAgentProject(t, d, r.stdout)
		if got := npx(); got != nil {
			t.Fatalf("%q: npx ran %q", answer, got)
		}
	}
}

func assertNoAgentProject(t *testing.T, d, out string) {
	t.Helper()
	if got, ok := manifest(t, d)["agents"]; !ok || !reflect.DeepEqual(got, []any{}) {
		t.Fatalf("agents %v (present %v)", got, ok)
	}
	for _, f := range []string{"AGENTS.md", ".agents/roles", ".agents/zones", ".agents/skills/zone-check/SKILL.md", ".agents/manifest.json", "docs", "Makefile"} {
		if !exists(d, f) {
			t.Fatalf("%s missing", f)
		}
	}
	for _, f := range []string{".claude", "CLAUDE.md", ".opencode", "opencode.json", ".gemini", ".hermes"} {
		if exists(d, f) {
			t.Fatalf("%s created without agents", f)
		}
	}
	match(t, out, `(?m)^! skills externas  \(sin agentes: se instalan cuando agregues uno con basekit sync --agents <a,b>\)$`)
}

// S2 · `--agents none` in init, and a rerun treats `agents: []` as decided: no detection, no question.
func TestAgentsNoneFlagAndRerunKeepsIt(t *testing.T) {
	npx := fakeNpx(t)
	d := scratch(t)
	r := run(t, "init", d, "-y", "--stack", "fastapi", "--agents", "none")
	mustOK(t, r)
	assertNoAgentProject(t, d, r.stdout)
	write(t, d, ".cursor/rules.md", "x\n") // detection would pick cursor
	again := runTTY(t, "\n", "init", d)
	mustOK(t, again)
	noMatch(t, again.stdout, `¿Con qué herramientas vas a trabajar\?`)
	if got := manifest(t, d)["agents"]; !reflect.DeepEqual(got, []any{}) {
		t.Fatalf("agents %v", got)
	}
	if got := npx(); got != nil {
		t.Fatalf("npx ran %q", got)
	}

	// sync --agents <a,b> later adds them and installs the external skills for them.
	s := run(t, "sync", d, "--agents", "claude-code")
	mustOK(t, s)
	if !exists(d, ".claude/agents/data-writer.md") || !exists(d, ".claude/skills/zone-check/SKILL.md") {
		t.Fatal("sync did not add the claude-code adapter and links")
	}
	if got := argsCalls(npx()); !reflect.DeepEqual(got, []string{pinnedWith(externalSkills, []string{"claude-code"})}) {
		t.Fatalf("%q", got)
	}
}

// S2 · `sync --agents none` drops the agents and writes no tool files.
func TestSyncAgentsNone(t *testing.T) {
	npx := fakeNpx(t)
	d := scratch(t)
	mustOK(t, initq(t, d, "--stack", "fastapi", "--agents", "none"))
	r := run(t, "sync", d, "--agents", "none")
	mustOK(t, r)
	if got := manifest(t, d)["agents"]; !reflect.DeepEqual(got, []any{}) {
		t.Fatalf("agents %v", got)
	}
	match(t, r.stdout, `(?m)^! skills externas  \(sin agentes: se instalan cuando agregues uno con basekit sync --agents <a,b>\)$`)
	if exists(d, ".claude") || exists(d, ".opencode") {
		t.Fatal("tool files created without agents")
	}
	if got := npx(); got != nil {
		t.Fatalf("npx ran %q", got)
	}
}

// S2 · `none` mixed with agent ids is a usage error, before writing anything.
func TestAgentsNoneMixedIsUsageError(t *testing.T) {
	d := scratch(t)
	r := initq(t, d, "--stack", "fastapi", "--agents", "none,codex")
	if r.code != 2 {
		t.Fatalf("init exit %d", r.code)
	}
	match(t, r.stderr, `^ERROR: --agents none no se combina con otros agentes\n$`)
	if len(entries(t, d)) != 0 {
		t.Fatalf("wrote %v", entries(t, d))
	}
	mustOK(t, initq(t, d, "--stack", "fastapi", "--agents", "codex"))
	s := run(t, "sync", d, "--agents", "codex,none")
	if s.code != 2 {
		t.Fatalf("sync exit %d", s.code)
	}
	match(t, s.stderr, `^ERROR: --agents none no se combina con otros agentes\n$`)
}

// S2 · Previous behavior: Enter in the agents question keeps the defaults.
func TestAgentsEnterKeepsDefaults(t *testing.T) {
	fakeNpx(t)
	d := scratch(t)
	mustOK(t, runTTY(t, "fastapi\nninguna\n\n\n\n\n", "init", d))
	if got := manifest(t, d)["agents"]; !reflect.DeepEqual(got, []any{"claude-code", "opencode"}) {
		t.Fatalf("agents %v", got)
	}
}

// S2 · Previous behavior: --agents <id> still works, and non-interactive init without --agents uses the defaults.
func TestAgentsFlagAndDefaultsStillWork(t *testing.T) {
	d := scratch(t)
	mustOK(t, initq(t, d, "--stack", "fastapi", "--agents", "codex"))
	if got := manifest(t, d)["agents"]; !reflect.DeepEqual(got, []any{"codex"}) {
		t.Fatalf("agents %v", got)
	}
	d = scratch(t)
	mustOK(t, initq(t, d, "--stack", "fastapi"))
	if got := manifest(t, d)["agents"]; !reflect.DeepEqual(got, []any{"claude-code", "opencode"}) {
		t.Fatalf("agents %v", got)
	}
}

// S2 · A global install honors `--agents none`: kit skills without links, external skills not installed.
func TestGlobalAgentsNone(t *testing.T) {
	npx := fakeNpx(t)
	home := fakeHome(t)
	r := run(t, "init", "--global", "-y", "--agents", "none")
	mustOK(t, r)
	if !exists(home, ".agents/skills/zone-check/SKILL.md") {
		t.Fatal("kit skill missing")
	}
	for _, dir := range []string{".claude", ".config/opencode"} {
		if exists(home, dir) {
			t.Fatalf("%s created without agents", dir)
		}
	}
	match(t, r.stdout, `(?m)^! skills externas  \(sin agentes: se instalan cuando agregues uno con basekit init --global --agents <a,b>\)$`)
	if got := npx(); got != nil {
		t.Fatalf("npx ran %q", got)
	}
}

// S1 · The global wizard asks the same list and installs only the chosen skills.
func TestGlobalExternalSkillsList(t *testing.T) {
	npx := fakeNpx(t)
	fakeHome(t)
	r := runTTY(t, "1\n2\n", "init", "--global")
	mustOK(t, r)
	match(t, r.stdout, regexp.QuoteMeta(askExternal)+`\n  1\) grill-me\n`)
	calls := argsCalls(npx())
	if len(calls) != 1 || !strings.HasPrefix(calls[0], strings.TrimSuffix(pinnedWith([]string{"grill-me"}, []string{"opencode"}), " -y")) {
		t.Fatalf("%q", calls)
	}
}

// doctor checks only the chosen external skills, and none without agents (nothing could install them).
func TestDoctorFollowsExternalChoiceAndAgents(t *testing.T) {
	fakeNpx(t)
	d := scratch(t)
	mustOK(t, runTTY(t, "fastapi\ngrilling\ncodex\n\n\n\n", "init", d))
	r := run(t, "doctor", d)
	match(t, lineFor(r.stdout, regexp.MustCompile(`grilling/SKILL\.md \(externa\)`)), `^OK `)
	noMatch(t, r.stdout, `grill-me/SKILL\.md`)

	d = scratch(t)
	mustOK(t, initq(t, d, "--stack", "fastapi", "--agents", "none"))
	noMatch(t, run(t, "doctor", d).stdout, `\(externa\)`)
}

// S3 · The help documents `--agents none` (init and sync) and the none answers of the lists.
func TestHelpDocumentsOptionalChoices(t *testing.T) {
	r := run(t, "help")
	mustOK(t, r)
	if n := strings.Count(r.stdout, "--agents none"); n != 2 {
		t.Fatalf("--agents none documented %d times, want init and sync:\n%s", n, r.stdout)
	}
	match(t, r.stdout, `ninguna o ninguno`)
}

// S4 · --external-skills <a,b> in init skips the question, installs the chosen skills in pinned order and persists them.
func TestExternalSkillsFlagInInit(t *testing.T) {
	npx := fakeNpx(t)
	d := scratch(t)
	// stack, agents, docker, git, context: no external skills question.
	r := runTTY(t, "fastapi\ncodex\n\n\n\n", "init", d, "--external-skills", "domain-modeling, grilling")
	mustOK(t, r)
	noMatch(t, r.stdout, regexp.QuoteMeta(askExternal))
	chosen := []string{"grilling", "domain-modeling"}
	if got := argsCalls(npx()); !reflect.DeepEqual(got, []string{pinnedWith(chosen, []string{"codex"})}) {
		t.Fatalf("%q", got)
	}
	match(t, r.stdout, `(?m)^\+ skills externas  \(grilling, domain-modeling\)$`)
	if v, _ := externalOption(t, d); !reflect.DeepEqual(v, []any{"grilling", "domain-modeling"}) {
		t.Fatalf("options.externalSkills %v", v)
	}
}

// S4 · --external-skills none stores [] and installs nothing; a later --external-skills replaces it.
func TestExternalSkillsFlagNone(t *testing.T) {
	npx := fakeNpx(t)
	d := scratch(t)
	r := run(t, "init", d, "-y", "--stack", "fastapi", "--agents", "codex", "--external-skills", "none")
	mustOK(t, r)
	match(t, r.stdout, `(?m)^= skills externas  \(ninguna elegida\)$`)
	if v, ok := externalOption(t, d); !ok || !reflect.DeepEqual(v, []any{}) {
		t.Fatalf("options.externalSkills %v (present %v)", v, ok)
	}
	if got := npx(); got != nil {
		t.Fatalf("npx ran %q", got)
	}
	again := run(t, "init", d, "-y", "--external-skills", "grill-me")
	mustOK(t, again)
	if got := argsCalls(npx()); !reflect.DeepEqual(got, []string{pinnedWith([]string{"grill-me"}, []string{"codex"})}) {
		t.Fatalf("%q", got)
	}
	if v, _ := externalOption(t, d); !reflect.DeepEqual(v, []any{"grill-me"}) {
		t.Fatalf("options.externalSkills %v", v)
	}
}

// S4 · sync --external-skills replaces the persisted choice and installs the chosen missing skills; deselected
// skills already installed stay in place, and later reruns follow the new choice without asking.
func TestExternalSkillsFlagInSync(t *testing.T) {
	npx := fakeNpx(t)
	d := scratch(t)
	mustOK(t, runTTY(t, "fastapi\ngrilling\ncodex\n\n\n\n", "init", d))
	s := run(t, "sync", d, "--external-skills", "grill-me")
	mustOK(t, s)
	calls := argsCalls(npx())
	if len(calls) != 2 || calls[1] != pinnedWith([]string{"grill-me"}, []string{"codex"}) {
		t.Fatalf("%q", calls)
	}
	match(t, s.stdout, `(?m)^\+ skills externas  \(grill-me\)$`)
	if !exists(d, ".agents/skills/grilling/SKILL.md") {
		t.Fatal("a deselected skill already installed was removed")
	}
	if v, _ := externalOption(t, d); !reflect.DeepEqual(v, []any{"grill-me"}) {
		t.Fatalf("options.externalSkills %v", v)
	}
	again := runTTY(t, "", "init", d)
	mustOK(t, again)
	noMatch(t, again.stdout, regexp.QuoteMeta(askExternal))
	match(t, again.stdout, `(?m)^= skills externas  \(grill-me\)$`)
	if n := len(argsCalls(npx())); n != 2 {
		t.Fatalf("%d npx calls", n)
	}
}

// S4 · Without agents, sync --external-skills stores the choice but installs nothing (the skills CLI needs -a).
func TestExternalSkillsFlagInSyncWithoutAgents(t *testing.T) {
	npx := fakeNpx(t)
	d := scratch(t)
	mustOK(t, initq(t, d, "--stack", "fastapi", "--agents", "none"))
	s := run(t, "sync", d, "--external-skills", "grilling")
	mustOK(t, s)
	match(t, s.stdout, `(?m)^! skills externas  \(sin agentes: se instalan cuando agregues uno con basekit sync --agents <a,b>\)$`)
	if got := npx(); got != nil {
		t.Fatalf("npx ran %q", got)
	}
	if v, _ := externalOption(t, d); !reflect.DeepEqual(v, []any{"grilling"}) {
		t.Fatalf("options.externalSkills %v", v)
	}
}

// S4 · Bad --external-skills values are usage errors (exit 2) before anything is written, in init and sync.
func TestExternalSkillsFlagUsageErrors(t *testing.T) {
	cases := []struct {
		args []string
		msg  string
	}{
		{[]string{"--external-skills", "grill-me,foo,bar"}, `--external-skills 'foo,bar' no es una skill externa. Disponibles: grill-me, grill-with-docs, grilling, domain-modeling`},
		{[]string{"--external-skills", "none,grilling"}, `--external-skills none no se combina con otras skills`},
		{[]string{"--external-skills", "grilling", "--no-external-skills"}, `--external-skills y --no-external-skills no se combinan`},
	}
	for _, c := range cases {
		npx := fakeNpx(t)
		d := scratch(t)
		r := run(t, append([]string{"init", d, "-y", "--stack", "fastapi"}, c.args...)...)
		if r.code != 2 {
			t.Fatalf("%v: init exit %d", c.args, r.code)
		}
		match(t, r.stderr, `^ERROR: `+regexp.QuoteMeta(c.msg)+`\n$`)
		if len(entries(t, d)) != 0 {
			t.Fatalf("%v: wrote %v", c.args, entries(t, d))
		}

		mustOK(t, initq(t, d, "--stack", "fastapi", "--agents", "codex"))
		before := read(t, d, ".agents/manifest.json")
		s := run(t, append([]string{"sync", d}, c.args...)...)
		if s.code != 2 {
			t.Fatalf("%v: sync exit %d", c.args, s.code)
		}
		match(t, s.stderr, `^ERROR: `+regexp.QuoteMeta(c.msg)+`\n$`)
		if s.stdout != "" || read(t, d, ".agents/manifest.json") != before {
			t.Fatalf("%v: sync changed something:\n%s", c.args, s.stdout)
		}
		if got := npx(); got != nil {
			t.Fatalf("%v: npx ran %q", c.args, got)
		}
	}
}

// S4 · init --global accepts --external-skills for this run only: no question, only those installed, nothing persisted.
func TestGlobalExternalSkillsFlag(t *testing.T) {
	npx := fakeNpx(t)
	home := fakeHome(t)
	r := runTTY(t, "2\n", "init", "--global", "--external-skills", "grilling")
	mustOK(t, r)
	noMatch(t, r.stdout, regexp.QuoteMeta(askExternal))
	calls := argsCalls(npx())
	if len(calls) != 1 || !strings.HasPrefix(calls[0], strings.TrimSuffix(pinnedWith([]string{"grilling"}, []string{"opencode"}), " -y")) {
		t.Fatalf("%q", calls)
	}
	var m map[string]any
	readJSON(t, home, ".agents/basekit.json", &m)
	if _, ok := m["options"]; ok {
		t.Fatalf("global manifest has options: %v", m["options"])
	}
	bad := run(t, "init", "--global", "-y", "--external-skills", "grilling", "--no-external-skills")
	if bad.code != 2 {
		t.Fatalf("exit %d", bad.code)
	}
	match(t, bad.stderr, `^ERROR: --external-skills y --no-external-skills no se combinan\n$`)
}

// S4 · The help documents --external-skills for init and sync.
func TestHelpDocumentsExternalSkillsFlag(t *testing.T) {
	r := run(t, "help")
	mustOK(t, r)
	if n := strings.Count(r.stdout, "--external-skills <a,b>"); n != 2 {
		t.Fatalf("--external-skills <a,b> documented %d times, want init and sync:\n%s", n, r.stdout)
	}
}

// S5 · An answer made only of commas or spaces is not «none»: it is named and the list asked again.
func TestExternalSkillsOnlyCommasIsRejected(t *testing.T) {
	for _, answer := range []string{",", ", ,"} {
		npx := fakeNpx(t)
		d := scratch(t)
		r := runTTY(t, "fastapi\n"+answer+"\ngrilling\ncodex\n\n\n\n", "init", d)
		mustOK(t, r)
		match(t, r.stdout, `  No reconozco: `+regexp.QuoteMeta(answer)+`\. Usa los números o los nombres de la lista\.\n`+
			regexp.QuoteMeta(`Números o nombres [grill-me,grill-with-docs,grilling,domain-modeling]: `))
		if got := argsCalls(npx()); !reflect.DeepEqual(got, []string{pinnedWith([]string{"grilling"}, []string{"codex"})}) {
			t.Fatalf("%q: %q", answer, got)
		}
	}
}

// S5 · Previous behavior: the agents list already rejects a lone comma and asks again.
func TestAgentsOnlyCommasStillRejected(t *testing.T) {
	fakeNpx(t)
	d := scratch(t)
	r := runTTY(t, "fastapi\n\n,\ncodex\n\n\n\n", "init", d)
	mustOK(t, r)
	match(t, r.stdout, `Opción no válida: ,\n`)
	if got := manifest(t, d)["agents"]; !reflect.DeepEqual(got, []any{"codex"}) {
		t.Fatalf("agents %v", got)
	}
}
