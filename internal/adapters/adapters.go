package adapters

import (
	"bytes"
	"encoding/json"
	"path"
	"regexp"
	"slices"
	"sort"
	"strings"

	"github.com/F4R1DS4NCH3Z26/basekit-go/internal/fsx"
	"github.com/F4R1DS4NCH3Z26/basekit-go/internal/stack"
)

type role struct {
	name, description, body string
}

var (
	rolePattern   = regexp.MustCompile(`^[a-z0-9][a-z0-9-]*$`)
	roleFrontRe   = regexp.MustCompile(`(?s)^---\n(.*?)\n---\n?(.*)$`)
	singleQuoteRe = regexp.MustCompile(`^'(.*)'$`)
)

// jsonString quotes s as JSON, which is also a valid YAML scalar.
func jsonString(s string) string {
	return strings.TrimSuffix(toJSON(s), "\n")
}

func unquote(v string) string {
	if strings.HasPrefix(v, `"`) && strings.HasSuffix(v, `"`) && len(v) >= 2 {
		var s string
		if json.Unmarshal([]byte(v), &s) == nil {
			return s
		}
		return v[1 : len(v)-1]
	}
	if m := singleQuoteRe.FindStringSubmatch(v); m != nil {
		return strings.ReplaceAll(m[1], "''", "'")
	}
	return v
}

func parseRole(name, text string) role {
	text = strings.ReplaceAll(text, "\r\n", "\n") // roles edited on Windows
	meta := map[string]string{}
	body := text
	if m := roleFrontRe.FindStringSubmatch(text); m != nil {
		body = m[2]
		for _, line := range strings.Split(m[1], "\n") {
			if k, v, ok := strings.Cut(line, ":"); ok && strings.TrimSpace(k) != "" {
				meta[strings.TrimSpace(k)] = unquote(strings.TrimSpace(v))
			}
		}
	}
	desc := meta["description"]
	if desc == "" {
		desc = name
	}
	if !strings.HasSuffix(body, "\n") {
		body += "\n"
	}
	return role{name, desc, body}
}

// readRoles reads the project's .agents/roles/, or the stack templates when it does
// not exist yet (dry-run).
func readRoles(p *fsx.Project, s stack.Stack) []role {
	fromProject := p.Exists(".agents/roles")
	var names []string
	if fromProject {
		names = p.Files(".agents/roles")
	} else {
		names = stack.Walk(path.Join(stack.Dir(s), "roles"))
	}
	var roles []role
	for _, f := range names {
		name, isMd := strings.CutSuffix(f, ".md")
		if !isMd || strings.Contains(f, "/") || !rolePattern.MatchString(name) {
			continue
		}
		var text string
		if fromProject {
			text, _ = p.Read(".agents/roles/" + f)
		} else {
			text = stack.Read(path.Join(stack.Dir(s), "roles", f))
		}
		roles = append(roles, parseRole(name, text))
	}
	return roles
}

func readZones(p *fsx.Project, s stack.Stack, roleName string) []string {
	text, ok := p.Read(".agents/zones/" + roleName + ".txt")
	if !ok {
		text, _ = stack.ReadOptional(path.Join(stack.Dir(s), "zones", roleName+".txt"))
	}
	var zones []string
	for _, l := range strings.Split(text, "\n") {
		l = strings.TrimSpace(l)
		if l != "" && !strings.HasPrefix(l, "#") && !slices.Contains(zones, l) {
			zones = append(zones, l)
		}
	}
	return zones
}

func generatedBy(roleName string) string {
	return "<!-- Generado por basekit desde .agents/roles/" + roleName + ".md — edita ese archivo y ejecuta `basekit sync` -->"
}

func claudeAgent(r role) string {
	return strings.Join([]string{
		"---",
		"name: " + r.name,
		"description: " + jsonString(r.description),
		"tools: Read, Edit, Write, Grep, Glob, Bash",
		"model: sonnet",
		"---",
		generatedBy(r.name),
		r.body,
	}, "\n")
}

var opencodeBash = [][2]string{
	{"*", "ask"}, {"make verify*", "allow"}, {"make test*", "allow"}, {"make lint*", "allow"},
	{"git status*", "allow"}, {"git diff*", "allow"}, {"bash .agents/skills/*", "allow"},
}

// opencodeAgent turns zones into real edit permissions; every pattern is a JSON
// string, which is valid YAML.
func opencodeAgent(r role, zones []string) string {
	lines := []string{"---", "description: " + jsonString(r.description), "mode: subagent", "permission:", "  edit:", `    "*": deny`}
	for _, z := range zones {
		lines = append(lines, "    "+jsonString(z)+": allow")
	}
	lines = append(lines, "  bash:")
	for _, b := range opencodeBash {
		lines = append(lines, "    "+jsonString(b[0])+": "+b[1])
	}
	lines = append(lines, "  webfetch: deny", "---", generatedBy(r.name), r.body)
	return strings.Join(lines, "\n")
}

func claudeSettings(s stack.Stack) string {
	allow := []string{
		"Bash(make help:*)", "Bash(make verify:*)", "Bash(make test:*)", "Bash(make lint:*)", "Bash(make ps:*)",
		"Bash(make logs:*)", "Bash(make doctor:*)", "Bash(git status:*)", "Bash(git diff:*)",
		"Bash(bash .agents/skills/zone-check/scripts/check_zones.sh:*)",
	}
	for _, row := range s.Skills {
		if stack.Exists(path.Join(stack.Dir(s), "skills", row[0], "scripts/scaffold.sh")) {
			allow = append(allow, "Bash(bash .agents/skills/"+row[0]+"/scripts/scaffold.sh:*)")
		}
	}
	type permissions struct {
		Allow []string `json:"allow"`
		Ask   []string `json:"ask"`
		Deny  []string `json:"deny"`
	}
	return toJSON(struct {
		Permissions permissions `json:"permissions"`
	}{permissions{
		Allow: allow,
		Ask: []string{
			"Bash(make dev:*)", "Bash(make up:*)", "Bash(make migrate:*)", "Bash(make migration:*)", "Bash(make install:*)",
			"Bash(make new-app:*)", "Bash(make deps-add:*)", "Bash(make prod-build:*)", "Bash(make prod-up:*)",
			"Bash(make prod-migrate:*)", "Bash(bash .agents/skills/infra-setup/scripts/infra.sh:*)",
			"Bash(docker:*)", "Bash(npx:*)", "Bash(git commit:*)", "Bash(git checkout:*)",
		},
		// Secrets are listed one by one: Read(./.env.*) would also block .env.example (deny wins over allow).
		Deny: []string{
			"Read(./.env)", "Read(./.env.prod)", "Read(./.env.*.local)", "Edit(./.env)", "Edit(./.env.prod)",
			"Read(./**/*.pem)", "Read(./**/*.key)", "Bash(make clean:*)", "Bash(git push:*)", "Bash(rm -rf:*)",
			"Bash(curl:*)", "Bash(wget:*)", "Bash(sudo:*)", "Bash(cat .env:*)",
		},
	}})
}

// opencodeJSON is written literally because rule order matters: in OpenCode the
// last matching rule wins, so *.env.example reopens what *.env.* closes.
const opencodeJSON = `{
  "$schema": "https://opencode.ai/config.json",
  "permission": {
    "read": {
      "*": "allow",
      "*.env": "deny",
      "*.env.*": "deny",
      "*.env.example": "allow"
    },
    "edit": {
      "*": "allow",
      "*.env": "deny",
      "*.env.*": "deny",
      "*.env.example": "allow"
    },
    "bash": {
      "*": "ask",
      "make help*": "allow",
      "make verify*": "allow",
      "make test*": "allow",
      "make lint*": "allow",
      "git status*": "allow",
      "git diff*": "allow",
      "bash .agents/skills/*": "allow",
      "make clean*": "deny",
      "git push*": "deny",
      "rm -rf*": "deny",
      "curl *": "deny",
      "wget *": "deny",
      "sudo *": "deny"
    }
  }
}
`

const geminiJSON = `{
  "context": {
    "fileName": [
      "AGENTS.md"
    ]
  }
}
`

func claudeMd(p *fsx.Project) {
	cur, ok := p.Read("CLAUDE.md")
	switch {
	case !ok:
		p.Generated("CLAUDE.md", "@AGENTS.md\n", 0)
	case slices.ContainsFunc(strings.Split(cur, "\n"), func(l string) bool { return strings.TrimSpace(l) == "@AGENTS.md" }):
		p.Report(fsx.Kept, "CLAUDE.md", "")
	default:
		p.Generated("CLAUDE.md", "@AGENTS.md\n\n"+cur, 0)
	}
}

// Write rewrites generated subagents when they differ; config files are
// created only if missing, then they belong to the user.
func Write(p *fsx.Project, s stack.Stack, agents []string) {
	roles := readRoles(p, s)
	if slices.Contains(agents, "claude-code") {
		claudeMd(p)
		for _, r := range roles {
			p.Generated(".claude/agents/"+r.name+".md", claudeAgent(r), 0)
		}
		p.Create(".claude/settings.json", claudeSettings(s), 0)
	}
	if slices.Contains(agents, "opencode") {
		for _, r := range roles {
			p.Generated(".opencode/agents/"+r.name+".md", opencodeAgent(r, readZones(p, s, r.name)), 0)
		}
		p.Create("opencode.json", opencodeJSON, 0)
	}
	if slices.Contains(agents, "gemini-cli") {
		p.Create(".gemini/settings.json", geminiJSON, 0)
	}
	// Hermes reads AGENTS.md from the cwd: it gets no instruction file of its own (never .hermes.md).
}

// LinkSkills links every skill in .agents/skills/ into the folders of the tools
// that do not read it natively.
func LinkSkills(p *fsx.Project, s stack.Stack, agents []string) {
	skills := p.Dirs(".agents/skills")
	for _, k := range stack.KitSkills(s) {
		if !slices.Contains(skills, k) {
			skills = append(skills, k)
		}
	}
	sort.Strings(skills)
	for _, a := range All {
		if a.SkillsDir == "" || !slices.Contains(agents, a.ID) {
			continue
		}
		for _, sk := range skills {
			p.Link(a.SkillsDir+"/"+sk, ".agents/skills/"+sk)
		}
	}
}

// toJSON encodes v with a two-space indent, a trailing newline and no HTML escaping.
func toJSON(v any) string {
	var b bytes.Buffer
	enc := json.NewEncoder(&b)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")
	_ = enc.Encode(v)
	return b.String()
}

// LinkGlobalSkills links each skill of ~/.agents/skills into the global folder of
// every chosen tool, except the tools that read ~/.agents/skills directly. p is
// rooted at the home directory.
func LinkGlobalSkills(p *fsx.Project, skills, agents []string) {
	for _, a := range All {
		if a.GlobalSkillsDir == CanonicalSkillsDir || !slices.Contains(agents, a.ID) {
			continue
		}
		for _, sk := range skills {
			p.Link(a.GlobalSkillsDir+"/"+sk, CanonicalSkillsDir+"/"+sk)
		}
	}
}
