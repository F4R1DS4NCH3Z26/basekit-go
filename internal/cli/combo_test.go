package cli_test

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"slices"
	"strings"
	"testing"
)

// S16: a combined project, `--stack <backend>+<frontend>` (port of Node's tests/commands/init/combo.test.ts).

var ansi = regexp.MustCompile(`\x1b\[[0-9;]*m`)

// targetsOf are the targets `make help` lists (one indented row per target).
func targetsOf(help string) []string {
	var out []string
	for _, l := range strings.Split(ansi.ReplaceAllString(help, ""), "\n") {
		if regexp.MustCompile(`^\s+\S`).MatchString(l) {
			out = append(out, strings.Fields(l)[0])
		}
	}
	return out
}

var commonTargets = []string{"help", "env", "doctor", "dev", "up", "down", "restart", "logs", "ps", "build", "install", "test", "lint",
	"verify", "migrate", "migration", "db-shell", "new-app", "prod-build", "prod-up", "prod-down", "prod-logs", "prod-migrate", "clean"}

func repoFile(t *testing.T, rel string) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("../..", rel))
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func TestComboLayoutSharedCoreAndOneProjectPerSide(t *testing.T) {
	skipOnWindows(t, "usa make y scripts bash")
	d := scratch(t)
	r := initq(t, d, "--stack", "fastapi+react", "--context", "Reservas")
	mustOK(t, r)

	for _, f := range []string{
		".git", "AGENTS.md", "CLAUDE.md", "README.md", "Makefile", ".gitignore", ".gitattributes", "docs/PROJECT.md",
		".agents/README.md", ".agents/manifest.json", ".agents/zones/orchestrator.txt",
		".agents/skills/zone-check/SKILL.md", ".agents/skills/infra-setup/scripts/infra.sh",
		".agents/skills/py-verify/SKILL.md", ".agents/skills/react-verify/SKILL.md",
		".agents/roles/backend-data-writer.md", ".agents/roles/frontend-ui-writer.md",
		".agents/zones/backend-data-writer.txt", ".agents/zones/frontend-ui-writer.txt",
		".claude/agents/backend-data-writer.md", ".claude/agents/frontend-ui-writer.md",
		".opencode/agents/backend-web-writer.md", ".opencode/agents/frontend-qa-writer.md",
		".claude/skills/py-verify", ".claude/skills/react-verify",
	} {
		if !exists(d, f) {
			t.Errorf("missing %s", f)
		}
	}
	for _, f := range []string{"Dockerfile", "compose.yaml", ".make", ".env.example", "src", ".agents/roles/data-writer.md", ".claude/agents/ui-writer.md"} {
		if exists(d, f) {
			t.Errorf("%s must not be at the root", f)
		}
	}
	match(t, read(t, d, "Makefile"), `generado por basekit`)
	match(t, read(t, d, "Makefile"), `-C backend`)
	if read(t, d, "Makefile") != repoFile(t, "plantillas/core/combo/Makefile") {
		t.Error("the root Makefile is core/combo/Makefile")
	}

	core := repoFile(t, "plantillas/core/Makefile")
	for _, side := range [][3]string{{"backend", "fastapi", "py-verify"}, {"frontend", "react", "react-verify"}} {
		if read(t, d, side[0]+"/Makefile") != core {
			t.Errorf("%s/Makefile is not the core Makefile", side[0])
		}
		for _, f := range []string{".make/stack.mk", "Dockerfile", "compose.yaml", "compose.dev.yaml", "compose.prod.yaml", ".env.example", ".dockerignore",
			".agents/skills/" + side[2] + "/scripts/verify.sh"} {
			if !exists(d, side[0]+"/"+f) {
				t.Errorf("missing %s/%s", side[0], f)
			}
		}
		if read(t, d, side[0]+"/.make/stack.mk") != repoFile(t, "plantillas/stacks/"+side[1]+"/files/.make/stack.mk") {
			t.Errorf("%s/.make/stack.mk", side[0])
		}
		if exists(d, side[0]+"/AGENTS.md") {
			t.Errorf("%s/AGENTS.md: the core lives at the root", side[0])
		}
	}
	if !exists(d, "backend/src/main.py") {
		t.Error("the fastapi starter goes into backend/")
	}
	if !exists(d, "frontend/.agents/skills/react-scaffold/assets/app/src/lib/api.ts.tpl") {
		t.Error("frontend new-app reads its scaffold assets")
	}
	if exists(d, "frontend/package.json") {
		t.Error("react has no starter: make new-app creates the app")
	}

	agents := read(t, d, "AGENTS.md")
	for _, id := range []string{"fastapi", "react"} {
		if !strings.Contains(agents, strings.TrimRight(repoFile(t, "plantillas/stacks/"+id+"/AGENTS.section.md"), "\n")) {
			t.Errorf("AGENTS.md lacks the %s section", id)
		}
	}
	match(t, agents, "\\| `py-scaffold` \\|")
	match(t, agents, "\\| `react-scaffold` \\|")
	match(t, agents, `backend/`)
	match(t, agents, regexp.QuoteMeta("**"+stackByID(t, "fastapi").Name+" en backend/ + "+stackByID(t, "react").Name+" en frontend/**"))
	noMatch(t, agents, `\{\{[A-Z_]+\}\}`)
	if strings.Count(agents, "<!-- basekit:start") != 1 {
		t.Error("one managed block")
	}

	role := read(t, d, ".agents/roles/backend-data-writer.md")
	match(t, role, `(?m)^name: backend-data-writer$`)
	match(t, role, `\.agents/zones/backend-data-writer\.txt`)
	match(t, role, "`backend/`")
	if z := read(t, d, ".agents/zones/backend-data-writer.txt"); z != "backend/src/modules/*/models.py\nbackend/src/modules/*/schemas.py\n" {
		t.Errorf("backend-data-writer zones: %q", z)
	}
	match(t, read(t, d, ".agents/zones/frontend-ui-writer.txt"), `(?m)^frontend/src/features/\*/components/\*\*$`)
	match(t, read(t, d, ".claude/agents/frontend-ui-writer.md"), `(?m)^name: frontend-ui-writer$`)
	match(t, read(t, d, ".opencode/agents/backend-data-writer.md"), `(?m)^ {4}"backend/src/modules/\*/models\.py": allow$`)
	orch := read(t, d, ".agents/zones/orchestrator.txt")
	if !strings.HasPrefix(orch, repoFile(t, "plantillas/core/zones.orchestrator.common.txt")) {
		t.Error("orchestrator zones start with the common zones")
	}
	match(t, orch, `(?m)^backend/src/core/\*\*$`)
	match(t, orch, `(?m)^frontend/src/lib/\*\*$`)

	m := manifest(t, d)
	if m["stack"] != "fastapi+react" {
		t.Fatal(m["stack"])
	}
	if !reflect.DeepEqual(m["options"], map[string]any{"db": "auto", "redis": false, "docker": true, "makefile": true}) {
		t.Fatal(m["options"])
	}
	if files := m["files"].(map[string]any); files["backend/.agents/skills/py-verify/SKILL.md"] == nil {
		t.Error("side skills are tracked")
	}
	match(t, r.stdout, `Listo: base de `+regexp.QuoteMeta(stackByID(t, "fastapi").Name+" en backend/ + "+stackByID(t, "react").Name+" en frontend/"))
	match(t, r.stdout, `(?m)^Falta la app del framework en frontend/: ejecuta `+"`make -C frontend new-app`"+` \(corre dentro de Docker\)\.$`)
	noMatch(t, r.stdout, `make -C backend new-app`)
}

func TestComboRootMakefileDelegatesToBothSides(t *testing.T) {
	skipOnWindows(t, "usa make")
	d := scratch(t)
	mustOK(t, initq(t, d, "--stack", "fastapi+react"))
	help := sh(t, d, "make", "help")
	if help.code != 0 {
		t.Fatal(help.stderr)
	}
	targets := targetsOf(help.stdout)
	for _, target := range commonTargets {
		if !slices.Contains(targets, target) {
			t.Errorf("make help lacks %s", target)
		}
	}
	dev := sh(t, d, "make", "-n", "dev")
	if dev.code != 0 {
		t.Fatal(dev.stderr)
	}
	match(t, dev.stdout, `-p backend-dev-\d+ .*up -d --build`)
	match(t, dev.stdout, `-p frontend-dev-\d+ .*up -d --build`)
	for _, side := range []string{"backend", "frontend"} {
		alone := sh(t, d, "make", "-C", side, "-n", "dev")
		if alone.code != 0 {
			t.Fatal(alone.stderr)
		}
		match(t, alone.stdout, `-p `+side+`-dev-\d+ .*up -d --build`)
		if n := len(targetsOf(sh(t, d, "make", "-C", side, "help").stdout)); n < 25 {
			t.Errorf("%s make help: %d targets", side, n)
		}
	}
	test := sh(t, d, "make", "-n", "test")
	match(t, test.stdout, `pytest -q`)
	match(t, test.stdout, `vitest run`)
	migrate := sh(t, d, "make", "-n", "migrate")
	match(t, migrate.stdout, `backend-dev-\d+`)
	noMatch(t, migrate.stdout, `frontend-dev-\d+`)
	newApp := sh(t, d, "make", "-n", "new-app")
	if newApp.code != 0 {
		t.Fatal(newApp.stderr)
	}
	match(t, newApp.stdout, `create-vite`)
}

func TestComboComposeGetsAProjectNamePerSide(t *testing.T) {
	skipOnWindows(t, "usa make")
	d := filepath.Join(scratch(t), "Mi.Tienda")
	mustOK(t, initq(t, d, "--stack", "fastapi+react"))
	match(t, read(t, d, "backend/compose.yaml"), `(?m)^name: mitienda-backend$`)
	match(t, read(t, d, "frontend/compose.yaml"), `(?m)^name: mitienda-frontend$`)
	noMatch(t, sh(t, d, "make", "-C", "backend", "-n", "prod-up").stdout, ` -p `)
	single := scratch(t)
	mustOK(t, initq(t, single, "--stack", "fastapi"))
	if read(t, single, "compose.yaml") != repoFile(t, "plantillas/stacks/fastapi/files/compose.yaml") {
		t.Error("a single stack's compose.yaml is copied as is")
	}
}

func TestComboDbAndRedisLandOnlyInBackend(t *testing.T) {
	skipOnWindows(t, "usa make y scripts bash")
	d := scratch(t)
	r := initq(t, d, "--stack", "fastapi+react", "--db", "postgres", "--redis")
	mustOK(t, r)
	match(t, r.stdout, `infra OK: db=postgres redis=1`)
	for _, f := range []string{"compose.db.yaml", "compose.redis.yaml", ".make/db.mk"} {
		if !exists(d, "backend/"+f) || exists(d, "frontend/"+f) || exists(d, f) {
			t.Errorf("%s must exist only in backend/", f)
		}
	}
	match(t, read(t, d, "backend/.env.example"), `(?m)^DB_PASSWORD=__GENERATE__$`)
	match(t, read(t, d, "backend/.env.example"), `(?m)^REDIS_PASSWORD=__GENERATE__$`)
	noMatch(t, read(t, d, "frontend/.env.example"), `DB_|REDIS`)
	opts := manifest(t, d)["options"].(map[string]any)
	if opts["db"] != "postgres" || opts["redis"] != true {
		t.Fatal(opts)
	}
	match(t, sh(t, d, "make", "-C", "backend", "-n", "dev").stdout, `-f compose\.db\.yaml -f compose\.redis\.yaml`)
}

func TestComboInfraScriptWritesIntoBackendFromAnyFolder(t *testing.T) {
	skipOnWindows(t, "usa scripts bash")
	d := scratch(t)
	mustOK(t, initq(t, d, "--stack", "fastapi+react"))
	gitCommitAll(t, d)
	infra := "backend/.agents/skills/infra-setup/scripts/infra.sh"
	fromRoot := sh(t, d, "bash", infra, "--db", "mysql", "--redis")
	if fromRoot.code != 0 {
		t.Fatal(fromRoot.stdout + fromRoot.stderr)
	}
	match(t, fromRoot.stdout, `infra OK: db=mysql redis=1`)
	for _, f := range []string{"compose.db.yaml", "compose.redis.yaml", ".make/db.mk"} {
		if !exists(d, "backend/"+f) || exists(d, f) || exists(d, "frontend/"+f) {
			t.Errorf("%s must exist only in backend/", f)
		}
	}
	inside := sh(t, filepath.Join(d, "backend"), "bash", ".agents/skills/infra-setup/scripts/infra.sh", "--db", "mysql", "--redis")
	if inside.code != 0 {
		t.Fatal(inside.stdout + inside.stderr)
	}
	match(t, inside.stdout, `(?m)^\s*= compose\.db\.yaml$`)
	outside := sh(t, scratch(t), "bash", filepath.Join(d, infra), "--db", "mysql", "--redis")
	if outside.code != 0 {
		t.Fatal(outside.stdout + outside.stderr)
	}
	match(t, outside.stdout, `(?m)^\s*= compose\.db\.yaml$`)
}

func TestComboClaudeAllowlistNamesTheSideScripts(t *testing.T) {
	d := scratch(t)
	mustOK(t, initq(t, d, "--stack", "fastapi+react", "--agents", "claude-code"))
	var settings struct {
		Permissions struct{ Allow, Ask []string } `json:"permissions"`
	}
	readJSON(t, d, ".claude/settings.json", &settings)
	scaffold := "bash backend/.agents/skills/py-scaffold/scripts/scaffold.sh"
	if !exists(d, strings.TrimPrefix(scaffold, "bash ")) {
		t.Error("the allowlisted script exists")
	}
	if !slices.Contains(settings.Permissions.Allow, "Bash("+scaffold+":*)") {
		t.Errorf("allow: %v", settings.Permissions.Allow)
	}
	if slices.Contains(settings.Permissions.Allow, "Bash(bash .agents/skills/py-scaffold/scripts/scaffold.sh:*)") {
		t.Error("the root copy is not allowlisted")
	}
	if !slices.Contains(settings.Permissions.Ask, "Bash(bash backend/.agents/skills/infra-setup/scripts/infra.sh:*)") {
		t.Errorf("ask: %v", settings.Permissions.Ask)
	}
	agents := read(t, d, "AGENTS.md")
	match(t, agents, "`bash backend/\\.agents/skills/<skill>/scripts/scaffold\\.sh <nombre>`")
	match(t, agents, "`bash backend/\\.agents/skills/infra-setup/scripts/infra\\.sh --db postgres`")
}

func TestComboOpencodeAllowlistCoversTheSideScripts(t *testing.T) {
	d := scratch(t)
	mustOK(t, initq(t, d, "--stack", "fastapi+react", "--agents", "opencode"))
	var oc struct {
		Permission struct {
			Bash map[string]string `json:"bash"`
		} `json:"permission"`
	}
	readJSON(t, d, "opencode.json", &oc)
	for _, p := range []string{"bash .agents/skills/*", "bash backend/.agents/skills/*", "bash frontend/.agents/skills/*"} {
		if oc.Permission.Bash[p] != "allow" {
			t.Errorf("%s: %q", p, oc.Permission.Bash[p])
		}
	}
	// The side entries follow the root one, before the denies (OpenCode: the last matching rule wins).
	match(t, read(t, d, "opencode.json"), `"bash \.agents/skills/\*": "allow",\n {6}"bash backend/\.agents/skills/\*": "allow",\n {6}"bash frontend/\.agents/skills/\*": "allow",\n {6}"make clean\*": "deny"`)
	agent := read(t, d, ".opencode/agents/backend-data-writer.md")
	match(t, agent, `(?m)^ {4}"bash \.agents/skills/\*": allow\n {4}"bash backend/\.agents/skills/\*": allow\n {4}"bash frontend/\.agents/skills/\*": allow$`)
}

// Single-stack adapters stay byte-identical to the ones before S16.
func TestSingleStackAdaptersHaveNoSideEntries(t *testing.T) {
	d := scratch(t)
	mustOK(t, initq(t, d, "--stack", "fastapi", "--agents", "claude-code,opencode"))
	noMatch(t, read(t, d, "opencode.json")+read(t, d, ".opencode/agents/data-writer.md")+read(t, d, ".claude/settings.json"), `backend/|frontend/`)
	match(t, read(t, d, "opencode.json"), `"bash \.agents/skills/\*": "allow",\n {6}"make clean\*": "deny"`)
	match(t, read(t, d, ".claude/settings.json"), `"Bash\(bash \.agents/skills/py-scaffold/scripts/scaffold\.sh:\*\)"`)
	match(t, read(t, d, ".claude/settings.json"), `"Bash\(bash \.agents/skills/infra-setup/scripts/infra\.sh:\*\)"`)
	match(t, read(t, d, ".opencode/agents/data-writer.md"), `"bash \.agents/skills/\*": allow\n  webfetch: deny`)
}

func TestComboSideEnvFilesAreIgnored(t *testing.T) {
	skipOnWindows(t, "usa git")
	d := scratch(t)
	mustOK(t, initq(t, d, "--stack", "fastapi+react"))
	secrets := []string{"backend/.env", "backend/.env.prod", "frontend/.env", "frontend/.env.prod"}
	ignored := sh(t, d, "git", append([]string{"check-ignore"}, secrets...)...)
	if ignored.code != 0 || strings.TrimSpace(ignored.stdout) != strings.Join(secrets, "\n") {
		t.Fatalf("check-ignore %d: %q %s", ignored.code, ignored.stdout, ignored.stderr)
	}
	if sh(t, d, "git", "check-ignore", "backend/.env.example", "frontend/.env.example").code != 1 {
		t.Error("the templates stay versioned")
	}
}

func TestComboLogsNeedSideForAOneSideService(t *testing.T) {
	skipOnWindows(t, "usa make")
	d := scratch(t)
	mustOK(t, initq(t, d, "--stack", "fastapi+react", "--db", "postgres"))
	both := sh(t, d, "make", "-n", "logs")
	if both.code != 0 {
		t.Fatal(both.stderr)
	}
	match(t, both.stdout, `(?m)-p backend-dev-\d+ .*logs -f --tail=100 *$`)
	match(t, both.stdout, `(?m)-p frontend-dev-\d+ .*logs -f --tail=100 *$`)

	noSide := sh(t, d, "make", "-n", "logs", "S=db")
	if noSide.code != 2 {
		t.Fatalf("exit %d: %s", noSide.code, noSide.stdout)
	}
	match(t, noSide.stderr, `S=db .*SIDE=backend o SIDE=frontend`)
	noMatch(t, noSide.stdout, `docker compose`)

	app := sh(t, d, "make", "-n", "logs", "S=app")
	if app.code != 0 {
		t.Fatal(app.stderr)
	}
	match(t, app.stdout, `(?m)-p backend-dev-\d+ .*logs -f --tail=100 app$`)
	match(t, app.stdout, `(?m)-p frontend-dev-\d+ .*logs -f --tail=100 app$`)

	db := sh(t, d, "make", "-n", "logs", "S=db", "SIDE=backend")
	if db.code != 0 {
		t.Fatal(db.stderr)
	}
	match(t, db.stdout, `(?m)-p backend-dev-\d+ .*logs -f --tail=100 db$`)
	noMatch(t, db.stdout, `frontend-dev-`)
	bad := sh(t, d, "make", "-n", "logs", "SIDE=web")
	if bad.code != 2 {
		t.Fatalf("exit %d", bad.code)
	}
	match(t, bad.stderr, `SIDE debe ser backend o frontend`)
	match(t, sh(t, d, "make", "help").stdout, `logs .*SIDE=backend\|frontend`)
}

func TestComboWiresTheFrontendToTheAPI(t *testing.T) {
	for _, pair := range [][2]string{{"fastapi", "react"}, {"gin", "vue"}} {
		d := scratch(t)
		mustOK(t, initq(t, d, "--stack", pair[0]+"+"+pair[1]))
		api, web := stackByID(t, pair[0]).Port, stackByID(t, pair[1]).Port
		match(t, read(t, d, "frontend/.env.example"), fmt.Sprintf(`(?m)^VITE_API_URL=http://127\.0\.0\.1:%d$`, api))
		match(t, read(t, d, "backend/.env.example"), fmt.Sprintf(`(?m)^CORS_ORIGINS=http://127\.0\.0\.1:%d,http://localhost:%d$`, web, web))
	}
}

func TestComboWarnsWhenTheBackendHasNoCORS(t *testing.T) {
	var noCors string
	for _, s := range stacksOf("backend") {
		if !regexp.MustCompile(`(?m)^CORS_ORIGINS=`).MatchString(repoFile(t, "plantillas/stacks/"+s.ID+"/files/.env.example")) {
			noCors = s.ID
			break
		}
	}
	if noCors == "" {
		t.Skip("every backend stack has CORS_ORIGINS")
	}
	d := scratch(t)
	r := initq(t, d, "--stack", noCors+"+"+stacksOf("frontend")[0].ID)
	mustOK(t, r)
	match(t, r.stdout, `(?m)^! backend/\.env\.example .*CORS`)
	match(t, read(t, d, "frontend/.env.example"), fmt.Sprintf(`(?m)^VITE_API_URL=http://127\.0\.0\.1:%d$`, stackByID(t, noCors).Port))
}

func TestComboRejectsBadPairsAndWritesNothing(t *testing.T) {
	for bad, why := range map[string]string{
		"react+fastapi":     `react.*frontend`,
		"fastapi+django":    `django.*backend`,
		"nextjs+react":      `nextjs.*fullstack`,
		"fastapi+nope":      `'nope' no existe`,
		"fastapi+react+vue": `<backend>\+<frontend>`,
		"fastapi+":          `<backend>\+<frontend>`,
	} {
		t.Run(bad, func(t *testing.T) {
			d := filepath.Join(scratch(t), "app")
			r := initq(t, d, "--stack", bad)
			if r.code != 2 {
				t.Fatalf("exit %d: %s", r.code, r.stdout)
			}
			match(t, r.stderr, why)
			match(t, r.stderr, `<backend>\+<frontend>|no existe`)
			if exists(d, "") {
				t.Error("nothing written")
			}
		})
	}
}

func TestComboRejectsNoDocker(t *testing.T) {
	d := filepath.Join(scratch(t), "app")
	r := initq(t, d, "--stack", "fastapi+react", "--no-docker")
	if r.code != 2 {
		t.Fatalf("exit %d: %s", r.code, r.stdout)
	}
	match(t, r.stderr, `^ERROR: un proyecto combinado \(<backend>\+<frontend>\) necesita Docker: cada lado corre con su Makefile y compose; quita --no-docker\n$`)
	if exists(d, "") {
		t.Error("nothing written")
	}
}

func TestComboDryRunShowsSidePaths(t *testing.T) {
	d := filepath.Join(scratch(t), "app")
	r := initq(t, d, "--stack", "fastapi+react", "--db", "postgres", "--dry-run")
	mustOK(t, r)
	if exists(d, "") {
		t.Fatal("dry-run wrote")
	}
	match(t, r.stdout, `(?m)^\+ backend/Dockerfile$`)
	match(t, r.stdout, `(?m)^\+ frontend/\.make/stack\.mk$`)
	match(t, r.stdout, `(?m)^\+ \.claude/agents/backend-data-writer\.md$`)
	match(t, r.stdout, `(?m)^· ejecutaría: bash backend/\.agents/skills/infra-setup/scripts/infra\.sh --db postgres$`)
	match(t, r.stdout, `(?m)^\+ backend/\.agents/skills/infra-setup/scripts/infra\.sh$`)
}

func TestComboRerunSyncAndDoctor(t *testing.T) {
	skipOnWindows(t, "usa scripts bash")
	d := scratch(t)
	mustOK(t, initq(t, d, "--stack", "fastapi+react", "--agents", "claude-code,opencode"))
	gitCommitAll(t, d)
	before := read(t, d, ".agents/manifest.json")

	again := initq(t, d)
	mustOK(t, again)
	if lines := actionLines(again.stdout, "+~!"); len(lines) > 0 {
		t.Fatalf("rerun changed: %v", lines)
	}
	gitClean(t, d)
	if read(t, d, ".agents/manifest.json") != before {
		t.Fatal("manifest changed")
	}

	if err := os.Remove(filepath.Join(d, ".claude/agents/backend-data-writer.md")); err != nil {
		t.Fatal(err)
	}
	s := run(t, "sync", d, "--no-external-skills")
	mustOK(t, s)
	match(t, s.stdout, `(?m)^\+ \.claude/agents/backend-data-writer\.md$`)
	noMatch(t, s.stdout, `(?m)^\+ \.claude/agents/data-writer\.md$`)
	if manifest(t, d)["stack"] != "fastapi+react" {
		t.Fatal(manifest(t, d)["stack"])
	}
	if st := sh(t, d, "git", "status", "--porcelain").stdout; st != " M .agents/manifest.json\n" {
		t.Fatalf("sync restores exactly what init wrote: %q", st)
	}

	doc := run(t, "doctor", d)
	if doc.code != 1 {
		t.Fatalf("doctor exit %d (no app nor .env yet)", doc.code)
	}
	for _, f := range []string{"backend/Makefile", "backend/.make/stack.mk", "backend/Dockerfile", "backend/compose.yaml", "frontend/compose.prod.yaml",
		"frontend/.env.example", "backend/.agents/skills/py-verify/SKILL.md", "frontend/.agents/skills/react-verify/SKILL.md"} {
		match(t, lineFor(doc.stdout, regexp.MustCompile(` `+regexp.QuoteMeta(f)+`(\s|$)`)), `^OK `)
	}
	match(t, lineFor(doc.stdout, regexp.MustCompile(` Makefile \(raíz\)`)), `^OK `)
	match(t, lineFor(doc.stdout, regexp.MustCompile(`frontend/package\.json`)), "^FALTA .*`make -C frontend new-app`")
	match(t, lineFor(doc.stdout, regexp.MustCompile(`backend/pyproject\.toml`)), `^OK `)
	match(t, lineFor(doc.stdout, regexp.MustCompile(` backend/\.env(\s|$)`)), `^FALTA .*make env`)
	match(t, lineFor(doc.stdout, regexp.MustCompile(`py-verify/SKILL\.md$`)), `^OK `)
	noMatch(t, doc.stdout, `stack 'fastapi\+react'`)
}

func TestComboZoneCheckOwnsEveryFile(t *testing.T) {
	skipOnWindows(t, "usa scripts bash")
	backends, frontends := stacksOf("backend"), stacksOf("frontend")
	pairs := []string{"fastapi+react", "gin+vue", "node-native+vanilla-ts"}
	for i, b := range backends {
		pairs = append(pairs, b.ID+"+"+frontends[i%len(frontends)].ID)
	}
	for _, pair := range pairs {
		t.Run(pair, func(t *testing.T) {
			d := scratch(t)
			mustOK(t, initq(t, d, "--stack", pair))
			sh(t, d, "git", "add", "-A")
			z := sh(t, d, "bash", ".agents/skills/zone-check/scripts/check_zones.sh", "--owners")
			if z.code != 0 {
				t.Fatal(z.stdout + z.stderr)
			}
			match(t, z.stdout, ` 0 sin dueño`)
			for _, f := range entries(t, filepath.Join(d, ".agents/roles")) {
				if !regexp.MustCompile(`^(backend|frontend)-`).MatchString(f) {
					t.Errorf("unprefixed role %s", f)
				}
			}
		})
	}
}

func TestHelpDocumentsTheCombinedStack(t *testing.T) {
	r := run(t, "help")
	mustOK(t, r)
	match(t, r.stdout, `--stack <back>\+<front> +Fullstack combinado: un stack backend en backend/ y uno frontend en\n +frontend/, con un Makefile raíz que delega en los dos`)
	match(t, r.stdout, `--stack <id> +Stack`)
	match(t, r.stdout, `stack\.json \(dbs; todos si falta\); en un combinado, va al backend\)`)
	match(t, r.stdout, `--redis +Agrega Redis \(no en un stack frontend; en un combinado, va al backend\)`)
}

// S16 regression: a single stack keeps its manifest format and layout.
func TestSingleStackKeepsItsFormat(t *testing.T) {
	d := scratch(t)
	mustOK(t, initq(t, d, "--stack", "fastapi"))
	m := manifest(t, d)
	var keys []string
	for k := range m {
		keys = append(keys, k)
	}
	slices.Sort(keys)
	if strings.Join(keys, ",") != "agents,createdAt,externalSkills,files,kit,options,stack,updatedAt,version" {
		t.Fatal(keys)
	}
	if m["stack"] != "fastapi" || exists(d, "backend") || exists(d, "frontend") || !exists(d, ".agents/roles/data-writer.md") {
		t.Fatal("single-stack layout changed")
	}
	for f := range m["files"].(map[string]any) {
		if !strings.HasPrefix(f, ".agents/") && f != "Makefile" {
			t.Errorf("side file %s", f)
		}
	}
}

// The fullstack step offers «Combinar backend + frontend» (S16), port of Node's tests/ui/prompt.test.ts.
func TestWizardCombinesBackendAndFrontend(t *testing.T) {
	fakeNpx(t)
	flags := []string{"--no-external-skills", "--agents", "codex", "--no-git", "--context", "x"}
	integrated := languagesOf("fullstack")

	d := scratch(t)
	r := runTTY(t, "3\nCombinar\nPython\nfastapi\nreact\n", append([]string{"init", d}, flags...)...)
	mustOK(t, r)
	if manifest(t, d)["stack"] != "fastapi+react" {
		t.Fatal(manifest(t, d)["stack"])
	}
	match(t, r.stdout, fmt.Sprintf(`%d\) Combinar backend \+ frontend +monorepo backend/ \+ frontend/; make dev levanta los dos`, len(integrated)+1))
	match(t, r.stdout, `¿En qué lenguaje es el backend\?`)
	for i, s := range stacksOf("backend", "Python") {
		match(t, r.stdout, fmt.Sprintf(`%d\) %s +%s\n`, i+1, regexp.QuoteMeta(s.Name), s.ID))
	}
	match(t, r.stdout, `Lenguaje del frontend: `+languagesOf("frontend")[0]+` \(única opción\)`)
	match(t, r.stdout, `¿Con qué framework de frontend\?`)
	noMatch(t, r.stdout, `Docker \+ Makefile`) // a combined project always uses Docker
	for _, s := range stacksOf("fullstack") {
		noMatch(t, r.stdout, `\) `+regexp.QuoteMeta(s.Name)+` +`+s.ID+`\n`)
	}

	goLang := slices.Index(languagesOf("backend"), "Go") + 1
	for input, want := range map[string]string{
		fmt.Sprintf("3\n%d\n%d\n1\n3\n", len(integrated)+1, goLang): stacksOf("backend", "Go")[0].ID + "+" + stacksOf("frontend")[2].ID,
		"fullstack\ncombinar\ngin\nvue\n":                           "gin+vue",
		"3\ncombinar\nreact\nGo\ngin\nfastapi\nvue\n":               "gin+vue",
	} {
		d := scratch(t)
		r := runTTY(t, input, append([]string{"init", d}, flags...)...)
		mustOK(t, r)
		if got := manifest(t, d)["stack"]; got != want {
			t.Errorf("%q: %v, want %s", input, got, want)
		}
		if strings.Contains(input, "react\nGo") {
			match(t, r.stdout, `«react» no es una opción válida`)
			match(t, r.stdout, `«fastapi» no es una opción válida`)
		}
	}
}

var (
	readSecrets = []string{".env", ".env.prod", ".env.local", ".env.production", ".env.*.local"}
	editSecrets = []string{".env", ".env.prod", ".env.local", ".env.production"}
)

func claudeDeny(t *testing.T, dir string) []string {
	t.Helper()
	var settings struct {
		Permissions struct{ Deny []string } `json:"permissions"`
	}
	readJSON(t, dir, ".claude/settings.json", &settings)
	return settings.Permissions.Deny
}

// S19: Claude's deny list also covers each side's secrets, as the allowlist covers its scripts.
func TestComboClaudeDenyCoversTheSideSecrets(t *testing.T) {
	d := scratch(t)
	mustOK(t, initq(t, d, "--stack", "fastapi+react", "--agents", "claude-code"))
	deny := claudeDeny(t, d)
	for _, dir := range []string{"", "backend/", "frontend/"} {
		for _, f := range readSecrets {
			if !slices.Contains(deny, "Read(./"+dir+f+")") {
				t.Errorf("Read %s%s", dir, f)
			}
		}
		for _, f := range editSecrets {
			if !slices.Contains(deny, "Edit(./"+dir+f+")") {
				t.Errorf("Edit %s%s", dir, f)
			}
		}
		if slices.Contains(deny, "Read(./"+dir+".env.*)") || slices.Contains(deny, "Edit(./"+dir+".env.example)") {
			t.Error("the templates stay readable")
		}
	}
	if len(slices.Compact(slices.Sorted(slices.Values(deny)))) != len(deny) {
		t.Error("duplicates", deny)
	}
}

// S19: OpenCode's read and edit rules already deny each side's secrets. opencode.ai/docs/permissions: `*` matches
// zero or more of any character (also `/`), `?` one character, everything else literally; the last matching rule wins.
func TestComboOpencodeRulesDenyTheSideSecrets(t *testing.T) {
	d := scratch(t)
	mustOK(t, initq(t, d, "--stack", "fastapi+react", "--agents", "opencode"))
	text := read(t, d, "opencode.json")
	for _, tool := range []string{"read", "edit"} {
		// The rules in file order: encoding/json maps would lose it.
		block := regexp.MustCompile(`"` + tool + `": \{([^}]*)\}`).FindStringSubmatch(text)
		if block == nil {
			t.Fatal(tool)
		}
		var rules [][2]string
		for _, m := range regexp.MustCompile(`"([^"]+)": "(\w+)"`).FindAllStringSubmatch(block[1], -1) {
			rules = append(rules, [2]string{m[1], m[2]})
		}
		decide := func(p string) string {
			action := ""
			for _, r := range rules {
				re := "^" + strings.NewReplacer(`\*`, ".*", `\?`, ".").Replace(regexp.QuoteMeta(r[0])) + "$"
				if regexp.MustCompile("(?s)" + re).MatchString(p) {
					action = r[1]
				}
			}
			return action
		}
		for _, side := range []string{"backend", "frontend"} {
			for _, f := range []string{".env", ".env.prod", ".env.local", ".env.production", ".env.development.local"} {
				if got := decide(side + "/" + f); got != "deny" {
					t.Errorf("%s %s/%s: %q", tool, side, f, got)
				}
			}
			if decide(side+"/.env.example") != "allow" || decide(side+"/src/main.py") != "allow" {
				t.Errorf("%s %s: the templates and the code stay open", tool, side)
			}
		}
	}
}

// S19: a single-stack project switched to combined with the database from the manifest gets its overlays in backend/
// (infra.sh runs for it), and init warns about the ones left at the root without deleting them.
func TestComboSwitchMovesTheDatabaseToBackend(t *testing.T) {
	skipOnWindows(t, "usa make y scripts bash")
	d := scratch(t)
	mustOK(t, initq(t, d, "--stack", "fastapi", "--db", "postgres", "--redis", "--agents", "claude-code,opencode"))
	gitCommitAll(t, d)
	r := initq(t, d, "--stack", "fastapi+react")
	mustOK(t, r)
	match(t, r.stdout, `infra OK: db=postgres redis=1`)
	for _, f := range []string{"compose.db.yaml", "compose.redis.yaml", ".make/db.mk"} {
		if !exists(d, "backend/"+f) || exists(d, "frontend/"+f) || !exists(d, f) {
			t.Errorf("%s: backend/ gets it and the root one is not deleted", f)
		}
		match(t, r.stdout, `(?m)^! `+regexp.QuoteMeta(f)+`  \(queda del stack anterior: la base de datos ahora vive en backend/; bórralo si ya no lo necesitas\)$`)
	}
	opts := manifest(t, d)["options"].(map[string]any)
	if opts["db"] != "postgres" || opts["redis"] != true {
		t.Fatal(opts)
	}
	match(t, read(t, d, "backend/.env.example"), `(?m)^DB_PASSWORD=__GENERATE__$`)
	match(t, sh(t, d, "make", "-C", "backend", "-n", "dev").stdout, `-f compose\.db\.yaml -f compose\.redis\.yaml`)
	// settings.json is the user's once created: init only appends the side secret entries it lacks (S20).
	if !slices.Contains(claudeDeny(t, d), "Read(./backend/.env)") {
		t.Error("side entries added")
	}
	match(t, r.stdout, `(?m)^~ \.claude/settings\.json  \(se agregaron 18 bloqueos de secretos de backend/ y frontend/\)$`)

	// A rerun takes the db from the manifest and does not touch the infrastructure again.
	gitCommitAll(t, d)
	again := initq(t, d)
	mustOK(t, again)
	noMatch(t, again.stdout, `infra OK|ejecutaría`)
}

func TestComboSwitchDryRunDoesNotRunInfra(t *testing.T) {
	skipOnWindows(t, "usa scripts bash")
	d := scratch(t)
	mustOK(t, initq(t, d, "--stack", "fastapi", "--db", "postgres", "--redis"))
	gitCommitAll(t, d)
	r := initq(t, d, "--stack", "fastapi+react", "--dry-run")
	mustOK(t, r)
	match(t, r.stdout, `(?m)^· ejecutaría: bash backend/\.agents/skills/infra-setup/scripts/infra\.sh --db postgres --redis$`)
	noMatch(t, r.stdout, `infra OK`)
	if exists(d, "backend") {
		t.Fatal("dry-run wrote")
	}
	gitClean(t, d)
}

func TestComboSwitchWithoutDatabaseRunsNoInfra(t *testing.T) {
	skipOnWindows(t, "usa scripts bash")
	d := scratch(t)
	mustOK(t, initq(t, d, "--stack", "fastapi"))
	gitCommitAll(t, d)
	r := initq(t, d, "--stack", "fastapi+react")
	mustOK(t, r)
	noMatch(t, r.stdout, `infra OK|ejecutaría|la base de datos ahora vive`)
	if exists(d, "backend/compose.db.yaml") {
		t.Fatal("no database, no overlay")
	}
}

// S19 regression: a single stack's deny list only gains the entries Node already had.
func TestSingleStackClaudeDeny(t *testing.T) {
	d := scratch(t)
	mustOK(t, initq(t, d, "--stack", "fastapi", "--agents", "claude-code"))
	want := []string{
		"Read(./.env)", "Read(./.env.prod)", "Read(./.env.local)", "Read(./.env.production)", "Read(./.env.*.local)",
		"Edit(./.env)", "Edit(./.env.prod)", "Edit(./.env.local)", "Edit(./.env.production)",
		"Read(./**/*.pem)", "Read(./**/*.key)", "Bash(make clean:*)", "Bash(git push:*)", "Bash(rm -rf:*)",
		"Bash(curl:*)", "Bash(wget:*)", "Bash(sudo:*)", "Bash(cat .env:*)",
	}
	if got := claudeDeny(t, d); !slices.Equal(got, want) {
		t.Fatalf("deny:\n got %v\nwant %v", got, want)
	}
}

// S20: an existing .claude/settings.json gets the side secret entries on combine.

const settingsRel = ".claude/settings.json"

var mergedLine = regexp.MustCompile(`(?m)^~ \.claude/settings\.json  \(se agregaron (\d+) bloqueos de secretos de backend/ y frontend/\)$`)

func sideSecretEntries() []string {
	var out []string
	for _, dir := range []string{"backend/", "frontend/"} {
		for _, f := range readSecrets {
			out = append(out, "Read(./"+dir+f+")")
		}
		for _, f := range editSecrets {
			out = append(out, "Edit(./"+dir+f+")")
		}
	}
	return out
}

func settingsWarning(reason string) string {
	return `(?m)^! \.claude/settings\.json  \(` + reason + `: se conserva y no niega los secretos de backend/ ni de frontend/ ` +
		`\(Read\(\./backend/\.env\)…\); agrégalos a mano\)$`
}

func mergedCount(out string) string {
	if m := mergedLine.FindStringSubmatch(out); m != nil {
		return m[1]
	}
	return ""
}

// singleWithSettings is a single-stack project whose settings.json the user edited, committed.
func singleWithSettings(t *testing.T, edit func(string) string) string {
	t.Helper()
	d := scratch(t)
	mustOK(t, initq(t, d, "--stack", "fastapi", "--agents", "claude-code"))
	write(t, d, settingsRel, edit(read(t, d, settingsRel)))
	gitCommitAll(t, d)
	return d
}

// withExtras adds a custom key before and after permissions, one allow entry and one deny entry.
func withExtras(text string) string {
	text = strings.Replace(text, `"Bash(make help:*)",`, `"Bash(make help:*)",`+"\n      "+`"Bash(make mine:*)",`, 1)
	text = strings.Replace(text, `"Bash(cat .env:*)"`, `"Bash(cat .env:*)",`+"\n      "+`"Bash(terraform:*)"`, 1)
	text = "{\n  \"model\": \"opus\",\n" + strings.TrimPrefix(text, "{\n")
	return strings.TrimSuffix(text, "\n}\n") + ",\n  \"custom\": {\n    \"keep\": [\n      1,\n      \"two\"\n    ]\n  }\n}\n"
}

func indentedEntries(entries []string) string {
	var b strings.Builder
	for _, e := range entries {
		b.WriteString(",\n      " + jsonQuote(e))
	}
	return b.String()
}

func jsonQuote(s string) string {
	b, _ := json.Marshal(s)
	return string(b)
}

func TestComboSettingsAppendsOnlyTheMissingSideEntries(t *testing.T) {
	d := singleWithSettings(t, withExtras)
	before := read(t, d, settingsRel)
	r := initq(t, d, "--stack", "fastapi+react")
	mustOK(t, r)
	if got := mergedCount(r.stdout); got != "18" {
		t.Fatalf("merged %q:\n%s", got, r.stdout)
	}
	noMatch(t, r.stdout, `(?m)^= \.claude/settings\.json`)
	want := strings.Replace(before, `"Bash(terraform:*)"`, `"Bash(terraform:*)"`+indentedEntries(sideSecretEntries()), 1)
	if got := read(t, d, settingsRel); got != want {
		t.Fatalf("settings.json:\n%s\nwant:\n%s", got, want)
	}
}

func TestComboSettingsAppendsWhatIsStillMissingAndCreatesDeny(t *testing.T) {
	d := singleWithSettings(t, func(text string) string {
		return strings.Replace(text, `"Bash(cat .env:*)"`, `"Bash(cat .env:*)", "Edit(./frontend/.env)", "Read(./backend/.env)"`, 1)
	})
	before := claudeDeny(t, d)
	r := initq(t, d, "--stack", "fastapi+react")
	if got := mergedCount(r.stdout); got != "16" {
		t.Fatalf("merged %q:\n%s", got, r.stdout)
	}
	var added []string
	for _, e := range sideSecretEntries() {
		if e != "Edit(./frontend/.env)" && e != "Read(./backend/.env)" {
			added = append(added, e)
		}
	}
	if got := claudeDeny(t, d); !slices.Equal(got, append(before, added...)) {
		t.Fatalf("deny: %v", got)
	}

	bare := singleWithSettings(t, func(string) string { return `{"model":"opus"}` })
	if got := mergedCount(initq(t, bare, "--stack", "fastapi+react").stdout); got != "18" {
		t.Fatalf("merged %q", got)
	}
	want := "{\n  \"model\": \"opus\",\n  \"permissions\": {\n    \"deny\": [\n      " +
		strings.TrimPrefix(indentedEntries(sideSecretEntries()), ",\n      ") + "\n    ]\n  }\n}\n"
	if got := read(t, bare, settingsRel); got != want {
		t.Fatalf("settings.json:\n%s\nwant:\n%s", got, want)
	}
}

func TestComboSettingsRerunIsIdempotent(t *testing.T) {
	d := singleWithSettings(t, withExtras)
	mustOK(t, initq(t, d, "--stack", "fastapi+react"))
	gitCommitAll(t, d)
	after := read(t, d, settingsRel)
	again := initq(t, d, "--stack", "fastapi+react")
	mustOK(t, again)
	match(t, again.stdout, `(?m)^= \.claude/settings\.json$`)
	if mergedCount(again.stdout) != "" || read(t, d, settingsRel) != after {
		t.Fatalf("rerun changed settings.json:\n%s", again.stdout)
	}
	if s := sh(t, d, "git", "status", "--porcelain", settingsRel).stdout; s != "" {
		t.Fatal(s)
	}
}

func TestComboSettingsUnparseableIsKept(t *testing.T) {
	text := "{ \"permissions\": { \"deny\": [\"Read(./.env)\"], } }\n"
	d := singleWithSettings(t, func(string) string { return text })
	r := initq(t, d, "--stack", "fastapi+react")
	mustOK(t, r)
	if read(t, d, settingsRel) != text {
		t.Fatal("settings.json changed")
	}
	match(t, r.stdout, settingsWarning("no es JSON válido"))
	noMatch(t, r.stdout, mergedLine.String())
}

func TestComboSettingsDenyNotAListIsKept(t *testing.T) {
	for _, text := range []string{"{\"permissions\":{\"deny\":\"Read(./.env)\"}}\n", "{\"permissions\":null}\n", "[]\n"} {
		d := singleWithSettings(t, func(string) string { return text })
		r := initq(t, d, "--stack", "fastapi+react")
		mustOK(t, r)
		if read(t, d, settingsRel) != text {
			t.Fatalf("%s: settings.json changed", text)
		}
		match(t, r.stdout, settingsWarning(`no tiene permissions\.deny como lista`))
	}
}

func TestComboSettingsDryRunWritesNothing(t *testing.T) {
	d := singleWithSettings(t, withExtras)
	before := read(t, d, settingsRel)
	r := initq(t, d, "--stack", "fastapi+react", "--dry-run")
	mustOK(t, r)
	if got := mergedCount(r.stdout); got != "18" {
		t.Fatalf("merged %q:\n%s", got, r.stdout)
	}
	if read(t, d, settingsRel) != before {
		t.Fatal("dry-run wrote settings.json")
	}
	gitClean(t, d)
}

func TestComboSettingsSyncAddsThemToo(t *testing.T) {
	d := scratch(t)
	mustOK(t, initq(t, d, "--stack", "fastapi+react", "--agents", "claude-code"))
	full := read(t, d, settingsRel)
	var kept []string
	for _, l := range strings.Split(full, "\n") {
		if !strings.Contains(l, "./frontend/") {
			kept = append(kept, l)
		}
	}
	write(t, d, settingsRel, strings.Join(kept, "\n"))
	r := run(t, "sync", d, "--no-external-skills")
	mustOK(t, r)
	if got := mergedCount(r.stdout); got != "9" {
		t.Fatalf("merged %q:\n%s", got, r.stdout)
	}
	if got, want := slices.Sorted(slices.Values(claudeDeny(t, d))), slices.Sorted(slices.Values(sideAndRoot(full))); !slices.Equal(got, want) {
		t.Fatalf("deny: %v", got)
	}
}

func sideAndRoot(text string) []string {
	var settings struct {
		Permissions struct{ Deny []string } `json:"permissions"`
	}
	_ = json.Unmarshal([]byte(text), &settings)
	return settings.Permissions.Deny
}

func TestSingleStackSettingsRerunUntouched(t *testing.T) {
	d := singleWithSettings(t, withExtras)
	before := read(t, d, settingsRel)
	r := initq(t, d, "--stack", "fastapi")
	mustOK(t, r)
	match(t, r.stdout, `(?m)^= \.claude/settings\.json$`)
	if read(t, d, settingsRel) != before {
		t.Fatal("single-stack rerun changed settings.json")
	}
}
