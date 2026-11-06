package cli_test

import (
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"runtime"
	"strings"
	"testing"
)

// Acceptance 1: init in an empty folder, per stack.
func TestInitEveryStackMakeHelpListsTargets(t *testing.T) {
	skipOnWindows(t, "usa make")
	for _, stack := range stackIDs {
		t.Run(stack, func(t *testing.T) {
			d := scratch(t)
			mustOK(t, initq(t, d, "--stack", stack))
			help := sh(t, d, "make", "help")
			if help.code != 0 {
				t.Fatal(help.stderr)
			}
			targets := 0
			for _, l := range strings.Split(help.stdout, "\n") {
				if regexp.MustCompile(`^\s+\S`).MatchString(l) {
					targets++
				}
			}
			if targets < 25 {
				t.Fatalf("only %d targets", targets)
			}
		})
	}
}

func TestInitWritesLayoutAgentsMdAndManifest(t *testing.T) {
	d := scratch(t)
	r := initq(t, d, "--stack", "nextjs", "--context", "Reservas de canchas de pádel")
	mustOK(t, r)
	for _, f := range []string{
		".git", ".agents/README.md", ".agents/zones/orchestrator.txt", ".agents/roles/data-writer.md",
		".agents/zones/data-writer.txt", ".agents/skills/zone-check/SKILL.md", ".agents/skills/nextjs-verify/SKILL.md",
		"Makefile", ".make/stack.mk", "Dockerfile", "compose.yaml", "compose.dev.yaml", "compose.prod.yaml",
		".env.example", ".dockerignore", "README.md", "docs/PROJECT.md", "docs/ARCHITECTURE.md",
		"docs/RUNBOOK.md", "docs/AUTH.md", "docs/specs/.gitkeep",
	} {
		if !exists(d, f) {
			t.Errorf("missing %s", f)
		}
	}
	for _, f := range []string{"GLOSSARY.md", "docs/adr", ".agents/zones/orchestrator.stack.txt"} {
		if exists(d, f) {
			t.Errorf("%s must not be created", f)
		}
	}

	agents := read(t, d, "AGENTS.md")
	match(t, agents, `^<!-- basekit:start v0\.1\.0 `)
	match(t, agents, `<!-- basekit:end -->\n$`)
	match(t, agents, `\*\*Next\.js \(App Router\) \+ TypeScript\*\*`)
	match(t, agents, "\\| `nextjs-scaffold` \\| Feature nueva: estructura \\+ contrato \\|")
	noMatch(t, agents, `\{\{[A-Z_]+\}\}`)
	match(t, read(t, d, "docs/PROJECT.md"), `Reservas de canchas de pádel`)
	match(t, read(t, d, "README.md"), `^# `+regexp.QuoteMeta(filepath.Base(d)))

	common, err := os.ReadFile("../../plantillas/core/zones.orchestrator.common.txt")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(read(t, d, ".agents/zones/orchestrator.txt"), string(common)) {
		t.Error("orchestrator zones must start with the common zones")
	}
	if runtime.GOOS != "windows" { // Windows has no permission bits
		info, _ := os.Stat(filepath.Join(d, ".agents/skills/zone-check/scripts/check_zones.sh"))
		if info.Mode().Perm() != 0o755 {
			t.Errorf("check_zones.sh mode %o", info.Mode().Perm())
		}
	}
	match(t, read(t, d, ".gitignore"), `# >>> basekit\n(?s:.*)# <<< basekit\n`)

	m := manifest(t, d)
	if m["kit"] != "basekit" || m["version"] != "0.1.0" || m["stack"] != "nextjs" {
		t.Errorf("manifest %v", m)
	}
	if !reflect.DeepEqual(m["agents"], []any{"claude-code", "opencode"}) {
		t.Errorf("agents %v", m["agents"])
	}
	opts := m["options"].(map[string]any)
	if opts["db"] != "auto" || opts["redis"] != false {
		t.Errorf("options %v", opts)
	}
	if m["externalSkills"].(map[string]any)["ref"] != "5beb35b040e3de97e54fa9e386d7e42d15c295c2" {
		t.Error("externalSkills.ref")
	}
	if m["createdAt"] == nil || m["updatedAt"] == nil {
		t.Error("timestamps")
	}
	match(t, r.stdout, `Lee AGENTS\.md y ejecuta el bootstrap\. El proyecto trata de: Reservas de canchas de pádel`)
	match(t, r.stdout, `make new-app`)
}

func TestInitCopiesStarterOnlyWhenAppMarkerMissing(t *testing.T) {
	d := scratch(t)
	mustOK(t, initq(t, d, "--stack", "fastapi"))
	if !exists(d, "src/main.py") || !exists(d, "pyproject.toml") {
		t.Fatal("starter not copied")
	}
	d2 := scratch(t)
	write(t, d2, "pyproject.toml", "[project]\nname = \"mine\"\ndependencies = [\"fastapi\"]\n")
	mustOK(t, initq(t, d2, "--stack", "fastapi"))
	if exists(d2, "src/main.py") {
		t.Fatal("starter copied over an existing app")
	}
	match(t, read(t, d2, "pyproject.toml"), `name = "mine"`)
}

// Acceptance 2: init twice in a row.
func TestInitTwiceOnlyKeepsEverything(t *testing.T) {
	d := scratch(t)
	mustOK(t, initq(t, d, "--stack", "nextjs"))
	gitCommitAll(t, d)
	before := manifest(t, d)["updatedAt"]

	r := initq(t, d, "--stack", "nextjs")
	mustOK(t, r)
	if len(actionLines(r.stdout, "+~=!")) == 0 {
		t.Fatal("no action lines")
	}
	if changed := actionLines(r.stdout, "+~!"); len(changed) > 0 {
		t.Fatalf("second run changed: %v", changed)
	}
	match(t, r.stdout, `al día`)
	gitClean(t, d)
	if manifest(t, d)["updatedAt"] != before {
		t.Fatal("updatedAt moved")
	}
}

func TestInitDoesNotRerunInfraWithoutDbFlag(t *testing.T) {
	skipOnWindows(t, "usa scripts bash")
	d := scratch(t)
	first := initq(t, d, "--stack", "nextjs", "--db", "postgres")
	mustOK(t, first)
	match(t, first.stdout, `infra OK`)
	r := initq(t, d)
	mustOK(t, r)
	noMatch(t, r.stdout, `infra OK|ejecutaría`)
	match(t, r.stdout, `al día`)
	if manifest(t, d)["options"].(map[string]any)["db"] != "postgres" {
		t.Fatal("db not kept")
	}
}

func TestInitRerunReusesManifest(t *testing.T) {
	d := scratch(t)
	mustOK(t, initq(t, d, "--stack", "spring", "--agents", "claude-code"))
	gitCommitAll(t, d)
	mustOK(t, initq(t, d))
	gitClean(t, d)
}

// Acceptance 3: project with its own AGENTS.md and Makefile.
func TestInitKeepsUserAgentsMdAndMakefile(t *testing.T) {
	d := scratch(t)
	ownMake := "all:\n\techo hola\n"
	write(t, d, "AGENTS.md", "# Mis reglas\n\nNo borrar esto.\n")
	write(t, d, "Makefile", ownMake)
	r := initq(t, d, "--stack", "laravel")
	mustOK(t, r)
	if !strings.HasPrefix(read(t, d, "AGENTS.md"), "# Mis reglas\n\nNo borrar esto.\n\n<!-- basekit:start") {
		t.Fatal(read(t, d, "AGENTS.md")[:120])
	}
	if read(t, d, "Makefile") != ownMake {
		t.Fatal("Makefile changed")
	}
	match(t, read(t, d, "Makefile.basekit"), `generado por basekit`)
	match(t, r.stdout, `(?m)^! .*Makefile\.basekit`)
}

func TestInitReplacesOnlyTheManagedBlock(t *testing.T) {
	d := scratch(t)
	mustOK(t, initq(t, d, "--stack", "nextjs"))
	original := read(t, d, "AGENTS.md")
	write(t, d, "AGENTS.md", "# Arriba\n\n"+strings.Replace(original, "## Reglas globales", "## CAMBIADO", 1)+"\n## Abajo\n")
	r := initq(t, d)
	mustOK(t, r)
	if got := read(t, d, "AGENTS.md"); got != "# Arriba\n\n"+original+"\n## Abajo\n" {
		t.Fatal(got)
	}
	match(t, r.stdout, `(?m)^~ AGENTS\.md`)
}

// Acceptance 4: --dry-run.
func TestDryRunWritesNothing(t *testing.T) {
	d := scratch(t)
	r := initq(t, d, "--stack", "nextjs", "--db", "postgres", "--dry-run")
	mustOK(t, r)
	if len(entries(t, d)) != 0 {
		t.Fatalf("wrote %v", entries(t, d))
	}
	match(t, r.stdout, `(?m)^\+ AGENTS\.md`)
}

func TestDryRunDoesNotCreateTargetDir(t *testing.T) {
	d := filepath.Join(scratch(t), "nuevo")
	mustOK(t, initq(t, d, "--stack", "nextjs", "--dry-run"))
	if exists(d, "") {
		t.Fatal("created target dir")
	}
}

// Acceptance 5: invalid values.
func TestInvalidValuesFailClearly(t *testing.T) {
	for _, c := range [][2]string{{"--stack", "foo"}, {"--agents", "x"}, {"--db", "mongo"}} {
		t.Run(c[0], func(t *testing.T) {
			d := scratch(t)
			r := initq(t, d, "--stack", "nextjs", c[0], c[1])
			if r.code == 0 {
				t.Fatal("expected failure")
			}
			match(t, r.stderr, c[0]+`.*`+c[1]+`|`+c[1]+`.*`+c[0])
			if len(entries(t, d)) != 0 {
				t.Fatal("wrote files")
			}
		})
	}
}

func TestRejectsUnknownOptions(t *testing.T) {
	r := run(t, "init", scratch(t), "--nope")
	if r.code != 2 {
		t.Fatalf("exit %d", r.code)
	}
	match(t, r.stderr, `--nope`)
}

// Acceptance 6: zone-check right after init.
func TestZoneCheckNoOrphansAfterInit(t *testing.T) {
	skipOnWindows(t, "usa scripts bash")
	for _, stack := range stackIDs {
		t.Run(stack, func(t *testing.T) {
			d := scratch(t)
			mustOK(t, initq(t, d, "--stack", stack))
			r := sh(t, d, "bash", ".agents/skills/zone-check/scripts/check_zones.sh", "--owners")
			if r.code != 0 {
				t.Fatal(r.stdout + r.stderr)
			}
			match(t, r.stdout, ` 0 sin dueño`)
		})
	}
}

// Acceptance 7: make env.
func TestMakeEnvGeneratesDistinctSecrets(t *testing.T) {
	skipOnWindows(t, "usa make")
	d := scratch(t)
	mustOK(t, initq(t, d, "--stack", "nextjs", "--db", "postgres", "--redis"))
	if r := sh(t, d, "make", "env"); r.code != 0 {
		t.Fatal(r.stdout + r.stderr)
	}
	env := read(t, d, ".env")
	noMatch(t, env, `__GENERATE`)
	info, _ := os.Stat(filepath.Join(d, ".env"))
	if info.Mode().Perm() != 0o600 {
		t.Fatalf(".env mode %o", info.Mode().Perm())
	}
	seen := map[string]bool{}
	for _, l := range strings.Split(env, "\n") {
		if regexp.MustCompile(`^[A-Z_]*(SECRET|PASSWORD)[A-Z_]*=`).MatchString(l) {
			v := strings.SplitN(l, "=", 2)[1]
			if seen[v] {
				t.Fatalf("repeated secret in %s", env)
			}
			seen[v] = true
		}
	}
	if len(seen) < 2 {
		t.Fatal(env)
	}
}

// Acceptance 8: make -n includes the existing overlays.
func TestMakeDryRunIncludesOverlays(t *testing.T) {
	skipOnWindows(t, "usa make")
	d := scratch(t)
	mustOK(t, initq(t, d, "--stack", "nextjs", "--db", "postgres", "--redis"))
	if !exists(d, "compose.db.yaml") || !exists(d, "compose.redis.yaml") {
		t.Fatal("overlays missing")
	}
	write(t, d, ".env", "X=1\n")
	write(t, d, ".env.prod", "X=1\n")
	for _, args := range [][]string{{"dev"}, {"verify", "SCOPE=src"}, {"migrate"}, {"prod-up"}} {
		n := sh(t, d, "make", append([]string{"-n"}, args...)...)
		if n.code != 0 {
			t.Fatalf("%v: %s%s", args, n.stdout, n.stderr)
		}
		match(t, n.stdout, `-f compose\.db\.yaml`)
		match(t, n.stdout, `-f compose\.redis\.yaml`)
	}
}

func TestModifiedKitSkillKeptUnlessForce(t *testing.T) {
	d := scratch(t)
	mustOK(t, initq(t, d, "--stack", "nextjs"))
	skill := ".agents/skills/zone-check/SKILL.md"
	original := read(t, d, skill)
	write(t, d, skill, "mío\n")

	r := initq(t, d)
	mustOK(t, r)
	if read(t, d, skill) != "mío\n" {
		t.Fatal("overwritten without --force")
	}
	match(t, r.stdout, `(?m)^! `+regexp.QuoteMeta(skill)+`.*--force`)

	mustOK(t, initq(t, d, "--force"))
	if read(t, d, skill) != original {
		t.Fatal("not restored with --force")
	}
}

func TestContextNeverBecomesAPathOrCommand(t *testing.T) {
	base := scratch(t)
	if err := os.Mkdir(filepath.Join(base, "a"), 0o755); err != nil {
		t.Fatal(err)
	}
	d := filepath.Join(base, "a", "b")
	mustOK(t, initq(t, d, "--stack", "nextjs", "--context", "../../evil $(touch pwned)"))
	if got := entries(t, base); !reflect.DeepEqual(got, []string{"a"}) {
		t.Fatal(got)
	}
	if exists(d, "pwned") {
		t.Fatal("command ran")
	}
}

func TestSymlinkedDirsCannotLeadWritesOutside(t *testing.T) {
	skipOnWindows(t, "crea symlinks")
	out := scratch(t)
	d := scratch(t)
	for _, dir := range []string{"docs", ".agents"} {
		if err := os.Symlink(out, filepath.Join(d, dir)); err != nil {
			t.Fatal(err)
		}
	}
	write(t, out, "victim.md", "# ajeno\n")
	if err := os.Symlink(filepath.Join(out, "victim.md"), filepath.Join(d, "AGENTS.md")); err != nil {
		t.Fatal(err)
	}
	r := initq(t, d, "--stack", "nextjs")
	mustOK(t, r)
	if got := entries(t, out); !reflect.DeepEqual(got, []string{"victim.md"}) {
		t.Fatalf("wrote outside the project: %v", got)
	}
	if read(t, out, "victim.md") != "# ajeno\n" {
		t.Fatal("followed AGENTS.md symlink outside the project")
	}
	match(t, r.stdout, `(?m)^! AGENTS\.md .*enlace simbólico`)
	match(t, r.stdout, `(?m)^! docs/PROJECT\.md .*enlace simbólico`)
}

func TestBlockWithoutEndMarkerIsLeftAlone(t *testing.T) {
	d := scratch(t)
	broken := "# Mío\n<!-- basekit:start roto\ntexto del usuario\n"
	write(t, d, "AGENTS.md", broken)
	for range 2 {
		r := initq(t, d, "--stack", "nextjs")
		mustOK(t, r)
		match(t, r.stdout, `(?m)^! AGENTS\.md .*marcador de inicio`)
	}
	if read(t, d, "AGENTS.md") != broken {
		t.Fatal(read(t, d, "AGENTS.md"))
	}
}

// A repository basekit creates gets .gitattributes (LF everywhere, so scripts and the
// Makefile survive Windows checkouts); an existing repository is left as it is.
func TestGitattributesOnlyForRepositoriesInitCreates(t *testing.T) {
	d := scratch(t)
	mustOK(t, initq(t, d, "--stack", "nextjs"))
	match(t, read(t, d, ".gitattributes"), `(?m)^\* text=auto eol=lf$`)
	if r := sh(t, d, "bash", ".agents/skills/zone-check/scripts/check_zones.sh", "--owners"); runtime.GOOS != "windows" {
		match(t, r.stdout, ` 0 sin dueño`)
	}

	existing := scratch(t)
	sh(t, existing, "git", "init", "-q")
	mustOK(t, initq(t, existing, "--stack", "nextjs"))
	if exists(existing, ".gitattributes") {
		t.Fatal(".gitattributes added to a repository basekit did not create")
	}
}
