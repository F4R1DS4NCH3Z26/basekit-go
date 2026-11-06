package basekit_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"slices"
	"strings"
	"testing"
)

const allAdapters = "claude-code,opencode,gemini-cli,hermes-agent"

var frontmatterRe = regexp.MustCompile(`(?s)^---\n(.*?)\n---\n(.*)$`)

func frontmatter(t *testing.T, text string) ([]string, string) {
	t.Helper()
	m := frontmatterRe.FindStringSubmatch(text)
	if m == nil {
		t.Fatal("missing frontmatter")
	}
	return strings.Split(m[1], "\n"), m[2]
}

var yamlLine = regexp.MustCompile(`^( *)("(?:[^"\\]|\\.)*"|[a-z]+):(?: (.*))?$`)

// assertYamlSubset checks the YAML subset basekit emits: `key: value`, nested maps, JSON-quoted keys/strings.
func assertYamlSubset(t *testing.T, lines []string) {
	t.Helper()
	for _, line := range lines {
		m := yamlLine.FindStringSubmatch(line)
		if m == nil {
			t.Fatalf("not valid YAML line: %s", line)
		}
		if len(m[1])%2 != 0 {
			t.Fatalf("bad indent: %s", line)
		}
		for _, s := range []string{m[2], m[3]} {
			if strings.HasPrefix(s, `"`) {
				var v string
				if err := json.Unmarshal([]byte(s), &v); err != nil {
					t.Fatalf("bad quoted string %s: %v", s, err)
				}
			}
		}
	}
}

func zonesOf(text string) []string {
	var zones []string
	for _, l := range strings.Split(text, "\n") {
		l = strings.TrimSpace(l)
		if l != "" && !strings.HasPrefix(l, "#") {
			zones = append(zones, l)
		}
	}
	return zones
}

// Acceptance 10: OpenCode subagents.
func TestOpenCodeSubagents(t *testing.T) {
	d := scratch(t)
	mustOK(t, initq(t, d, "--stack", "nextjs", "--agents", "opencode"))
	roles := entries(t, filepath.Join(d, ".agents/roles"))
	if got := entries(t, filepath.Join(d, ".opencode/agents")); !reflect.DeepEqual(got, roles) {
		t.Fatalf("%v != %v", got, roles)
	}
	for _, f := range roles {
		role := strings.TrimSuffix(f, ".md")
		lines, body := frontmatter(t, read(t, d, ".opencode/agents/"+f))
		assertYamlSubset(t, lines)
		if !slices.Contains(lines, "mode: subagent") || !slices.Contains(lines, "  webfetch: deny") {
			t.Fatalf("%s: %v", role, lines)
		}
		edit := slices.Index(lines, "  edit:")
		if lines[edit+1] != `    "*": deny` {
			t.Fatalf("%s: edit not denied by default", role)
		}
		zones := zonesOf(read(t, d, ".agents/zones/"+role+".txt"))
		if len(zones) == 0 {
			t.Fatal("no zones")
		}
		for _, z := range zones {
			q, _ := json.Marshal(z)
			if !slices.Contains(lines, "    "+string(q)+": allow") {
				t.Fatalf("%s: zone %s", role, z)
			}
		}
		match(t, body, `^<!-- Generado por basekit desde \.agents/roles/`)
		for _, l := range lines {
			if strings.HasPrefix(l, "description: ") {
				var desc string
				_ = json.Unmarshal([]byte(strings.TrimPrefix(l, "description: ")), &desc)
				if len(desc) <= 10 {
					t.Fatalf("short description %q", desc)
				}
			}
		}
	}
	var cfg struct {
		Permission map[string]map[string]string `json:"permission"`
	}
	if err := json.Unmarshal([]byte(read(t, d, "opencode.json")), &cfg); err != nil {
		t.Fatal(err)
	}
	if cfg.Permission["read"]["*.env"] != "deny" || cfg.Permission["read"]["*.env.example"] != "allow" {
		t.Fatal(cfg.Permission["read"])
	}
	if exists(d, ".claude") {
		t.Fatal("Claude files for an OpenCode-only project")
	}
}

func TestClaudeCodeAdapter(t *testing.T) {
	d := scratch(t)
	mustOK(t, initq(t, d, "--stack", "laravel", "--agents", "claude-code"))
	if read(t, d, "CLAUDE.md") != "@AGENTS.md\n" {
		t.Fatal(read(t, d, "CLAUDE.md"))
	}
	roleBody := regexp.MustCompile(`(?s)^---\n.*?\n---\n`).ReplaceAllString(read(t, d, ".agents/roles/data-writer.md"), "")
	lines, body := frontmatter(t, read(t, d, ".claude/agents/data-writer.md"))
	assertYamlSubset(t, lines)
	for _, want := range []string{"name: data-writer", "tools: Read, Edit, Write, Grep, Glob, Bash", "model: sonnet"} {
		if !slices.Contains(lines, want) {
			t.Fatalf("missing %q", want)
		}
	}
	if !strings.HasSuffix(body, roleBody) {
		t.Fatal("role body not copied as is")
	}
	var settings struct {
		Permissions struct{ Allow, Deny []string } `json:"permissions"`
	}
	if err := json.Unmarshal([]byte(read(t, d, ".claude/settings.json")), &settings); err != nil {
		t.Fatal(err)
	}
	if !slices.Contains(settings.Permissions.Deny, "Read(./.env)") || slices.Contains(settings.Permissions.Deny, "Read(./.env.*)") {
		t.Fatal(settings.Permissions.Deny)
	}
	if !slices.Contains(settings.Permissions.Allow, "Bash(bash .agents/skills/php-scaffold/scripts/scaffold.sh:*)") {
		t.Fatal(settings.Permissions.Allow)
	}
	if exists(d, "opencode.json") {
		t.Fatal("opencode.json for a Claude-only project")
	}
}

func TestClaudeKeepsExistingFiles(t *testing.T) {
	d := scratch(t)
	write(t, d, "CLAUDE.md", "# Notas mías\n")
	write(t, d, ".claude/settings.json", "{\"mine\":true}\n")
	mustOK(t, initq(t, d, "--stack", "nextjs", "--agents", "claude-code"))
	if read(t, d, "CLAUDE.md") != "@AGENTS.md\n\n# Notas mías\n" {
		t.Fatal(read(t, d, "CLAUDE.md"))
	}
	if read(t, d, ".claude/settings.json") != "{\"mine\":true}\n" {
		t.Fatal("settings.json replaced")
	}
}

func TestGeminiAndHermes(t *testing.T) {
	d := scratch(t)
	mustOK(t, initq(t, d, "--stack", "fastapi", "--agents", "gemini-cli,hermes-agent"))
	var cfg any
	_ = json.Unmarshal([]byte(read(t, d, ".gemini/settings.json")), &cfg)
	want := map[string]any{"context": map[string]any{"fileName": []any{"AGENTS.md"}}}
	if !reflect.DeepEqual(cfg, want) {
		t.Fatal(cfg)
	}
	if exists(d, ".hermes.md") || exists(d, "HERMES.md") {
		t.Fatal("Hermes instruction file created")
	}
}

func TestWarnsWhenHermesFileExists(t *testing.T) {
	d := scratch(t)
	write(t, d, ".hermes.md", "x\n")
	r := initq(t, d, "--stack", "fastapi", "--agents", "hermes-agent")
	mustOK(t, r)
	match(t, r.stdout, `(?m)^! \.hermes\.md`)
}

// Acceptance 11: skill links.
func TestSkillLinksAreRelative(t *testing.T) {
	d := scratch(t)
	mustOK(t, initq(t, d, "--stack", "spring", "--agents", allAdapters))
	skills := entries(t, filepath.Join(d, ".agents/skills"))
	for _, dir := range []string{".claude/skills", ".hermes/skills"} {
		if got := entries(t, filepath.Join(d, dir)); !reflect.DeepEqual(got, skills) {
			t.Fatalf("%s: %v != %v", dir, got, skills)
		}
		for _, s := range skills {
			link := filepath.Join(d, dir, s)
			target, err := os.Readlink(link)
			if err != nil {
				t.Fatalf("%s/%s is not a link", dir, s)
			}
			if filepath.IsAbs(target) {
				t.Fatal("link must be relative")
			}
			a, _ := filepath.EvalSymlinks(link)
			b, _ := filepath.EvalSymlinks(filepath.Join(d, ".agents/skills", s))
			if a != b {
				t.Fatalf("%s -> %s", link, a)
			}
		}
	}
	if exists(d, ".opencode/skills") {
		t.Fatal("OpenCode reads .agents/skills natively")
	}
}

func TestIdempotentWithEveryAdapter(t *testing.T) {
	d := scratch(t)
	mustOK(t, initq(t, d, "--stack", "nextjs", "--agents", allAdapters))
	gitCommitAll(t, d)
	r := initq(t, d)
	mustOK(t, r)
	if changed := actionLines(r.stdout, "+~!"); len(changed) > 0 {
		t.Fatal(changed)
	}
	gitClean(t, d)
}
