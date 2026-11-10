package adapters

import (
	"bytes"
	"encoding/json"
	"path"
	"regexp"
	"slices"
	"sort"
	"strconv"
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

// readRoles reads the project's .agents/roles/, or the roles init would write when
// it does not exist yet (dry-run).
func readRoles(p *fsx.Project, parts []stack.Part) []role {
	fromProject := p.Exists(".agents/roles")
	kit := map[string]string{}
	var names []string
	if fromProject {
		names = p.Files(".agents/roles")
	} else {
		for _, f := range stack.RoleFiles(parts) {
			kit[f.Rel] = f.Text
			names = append(names, f.Rel)
		}
		sort.Strings(names)
	}
	var roles []role
	for _, f := range names {
		name, isMd := strings.CutSuffix(f, ".md")
		if !isMd || strings.Contains(f, "/") || !rolePattern.MatchString(name) {
			continue
		}
		text := kit[f]
		if fromProject {
			text, _ = p.Read(".agents/roles/" + f)
		}
		roles = append(roles, parseRole(name, text))
	}
	return roles
}

func readZones(p *fsx.Project, parts []stack.Part, roleName string) []string {
	text, ok := p.Read(".agents/zones/" + roleName + ".txt")
	if !ok {
		for _, f := range stack.ZoneFiles(parts) {
			if f.Rel == roleName+".txt" {
				text = f.Text
			}
		}
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

// rootSkills allows the skill scripts of the project's .agents/skills/.
const rootSkills = "bash .agents/skills/*"

var opencodeBash = [][2]string{
	{"*", "ask"}, {"make verify*", "allow"}, {"make test*", "allow"}, {"make lint*", "allow"},
	{"git status*", "allow"}, {"git diff*", "allow"}, {rootSkills, "allow"},
}

// sideSkills are the side copies of the skill scripts (`bash backend/.agents/skills/*`);
// empty for a single-stack project, whose scripts are the root ones.
func sideSkills(parts []stack.Part) []string {
	var out []string
	for _, p := range parts {
		if g := "bash " + stack.InPart(p, ".agents/skills/*"); g != rootSkills && !slices.Contains(out, g) {
			out = append(out, g)
		}
	}
	return out
}

// opencodeAgent turns zones into real edit permissions; every pattern is a JSON
// string, which is valid YAML.
func opencodeAgent(r role, zones []string, parts []stack.Part) string {
	lines := []string{"---", "description: " + jsonString(r.description), "mode: subagent", "permission:", "  edit:", `    "*": deny`}
	for _, z := range zones {
		lines = append(lines, "    "+jsonString(z)+": allow")
	}
	lines = append(lines, "  bash:")
	for _, b := range opencodeBash {
		lines = append(lines, "    "+jsonString(b[0])+": "+b[1])
	}
	for _, g := range sideSkills(parts) {
		lines = append(lines, "    "+jsonString(g)+": allow")
	}
	lines = append(lines, "  webfetch: deny", "---", generatedBy(r.name), r.body)
	return strings.Join(lines, "\n")
}

// claudeSettings allowlists the skill scripts where they run: the skill scripts work on the project that holds them
// (S17), so in a combined project the allowed invocations are the side copies, run from the root
// (`bash backend/.agents/skills/<skill>/scripts/scaffold.sh`), as AGENTS.md documents.
func claudeSettings(parts []stack.Part) string {
	allow := []string{
		"Bash(make help:*)", "Bash(make verify:*)", "Bash(make test:*)", "Bash(make lint:*)", "Bash(make ps:*)",
		"Bash(make logs:*)", "Bash(make doctor:*)", "Bash(git status:*)", "Bash(git diff:*)",
		"Bash(bash .agents/skills/zone-check/scripts/check_zones.sh:*)",
	}
	for _, p := range parts {
		for _, row := range p.Stack.Skills {
			if stack.Exists(path.Join(stack.Dir(p.Stack), "skills", row[0], "scripts/scaffold.sh")) {
				allow = append(allow, "Bash(bash "+stack.InPart(p, ".agents/skills/"+row[0]+"/scripts/scaffold.sh")+":*)")
			}
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
			"Bash(make prod-migrate:*)", "Bash(bash " + stack.InPart(parts[0], stack.InfraScript) + ":*)",
			"Bash(docker:*)", "Bash(npx:*)", "Bash(git commit:*)", "Bash(git checkout:*)",
		},
		Deny: append(secretDeny(parts),
			"Read(./**/*.pem)", "Read(./**/*.key)", "Bash(make clean:*)", "Bash(git push:*)", "Bash(rm -rf:*)",
			"Bash(curl:*)", "Bash(wget:*)", "Bash(sudo:*)", "Bash(cat .env:*)",
		),
	}})
}

var (
	readSecrets = []string{".env", ".env.prod", ".env.local", ".env.production", ".env.*.local"}
	editSecrets = []string{".env", ".env.prod", ".env.local", ".env.production"}
)

// sideDirs are the side folders of a combined project ("backend/", "frontend/"); empty for a single stack.
func sideDirs(parts []stack.Part) []string {
	var out []string
	for _, p := range parts {
		if p.Side != "" {
			out = append(out, stack.InPart(p, ""))
		}
	}
	return out
}

// secretDeny are Claude's deny entries for the secrets of the root and, in a combined project, of each side (S19), as
// the allowlist covers each side's scripts. They are listed one by one: Read(./.env.*) would also block .env.example
// (deny wins over allow).
func secretDeny(parts []stack.Part) []string {
	var out []string
	for _, dir := range append([]string{""}, sideDirs(parts)...) {
		for _, f := range readSecrets {
			out = append(out, "Read(./"+dir+f+")")
		}
		for _, f := range editSecrets {
			out = append(out, "Edit(./"+dir+f+")")
		}
	}
	return out
}

// claudeSettingsFile: settings.json is the user's once it exists (for example, from before the project was combined).
// In a combined project init appends to its permissions.deny only the side secret entries it lacks (S20) and keeps
// every other key, value and entry in order; a file it cannot read as JSON with a deny list is kept, with a warning.
func claudeSettingsFile(p *fsx.Project, parts []stack.Part) {
	const rel = ".claude/settings.json"
	sides := sideDirs(parts)
	cur, ok := "", false
	if len(sides) > 0 && p.IsFile(rel) {
		cur, ok = p.Read(rel)
	}
	if !ok {
		p.Create(rel, claudeSettings(parts), 0)
		return
	}
	var required []string
	for _, e := range secretDeny(parts) {
		if slices.ContainsFunc(sides, func(s string) bool { return strings.Contains(e, "./"+s) }) {
			required = append(required, e)
		}
	}
	next, added, reason := withDenyEntries([]byte(cur), required)
	switch {
	case reason != "":
		for _, e := range required {
			if !strings.Contains(cur, jsonString(e)) {
				p.Report(fsx.Warned, rel, reason+": se conserva y no niega los secretos de "+strings.Join(sides, " ni de ")+
					" ("+e+"…); agrégalos a mano")
				return
			}
		}
		p.Report(fsx.Kept, rel, "")
	case added == 0:
		p.Report(fsx.Kept, rel, "")
	default:
		p.Update(rel, next, "se agregaron "+strconv.Itoa(added)+" bloqueos de secretos de "+strings.Join(sides, " y "))
	}
}

// jsonMember is one key of a JSON object with its raw value.
type jsonMember struct {
	key string
	raw json.RawMessage
}

// parseObject reads a JSON object keeping its keys in file order and their values as written; false when data is
// not an object. encoding/json maps would sort the keys.
func parseObject(data []byte) ([]jsonMember, bool) {
	dec := json.NewDecoder(bytes.NewReader(data))
	if tok, err := dec.Token(); err != nil || tok != json.Delim('{') {
		return nil, false
	}
	var out []jsonMember
	for dec.More() {
		tok, err := dec.Token()
		key, isKey := tok.(string)
		if err != nil || !isKey {
			return nil, false
		}
		var raw json.RawMessage
		if err := dec.Decode(&raw); err != nil {
			return nil, false
		}
		out = append(out, jsonMember{key, raw})
	}
	return out, true
}

func encodeObject(members []jsonMember) json.RawMessage {
	var b bytes.Buffer
	b.WriteByte('{')
	for i, m := range members {
		if i > 0 {
			b.WriteByte(',')
		}
		b.WriteString(jsonString(m.key))
		b.WriteByte(':')
		b.Write(m.raw)
	}
	b.WriteByte('}')
	return b.Bytes()
}

// setMember replaces the value of key (its last occurrence, which is the one JSON readers keep) or appends it.
func setMember(members []jsonMember, key string, raw json.RawMessage) []jsonMember {
	for i := len(members) - 1; i >= 0; i-- {
		if members[i].key == key {
			members[i].raw = raw
			return members
		}
	}
	return append(members, jsonMember{key, raw})
}

func member(members []jsonMember, key string) (json.RawMessage, bool) {
	for i := len(members) - 1; i >= 0; i-- {
		if members[i].key == key {
			return members[i].raw, true
		}
	}
	return nil, false
}

// withDenyEntries appends to permissions.deny the required entries it lacks, creating permissions and deny when
// absent, and returns the file with a two-space indent and how many were added. reason says why data cannot be
// merged: it is not JSON, or permissions.deny is not a list.
func withDenyEntries(data []byte, required []string) (next string, added int, reason string) {
	const notList = "no tiene permissions.deny como lista"
	if !json.Valid(data) {
		return "", 0, "no es JSON válido"
	}
	root, ok := parseObject(data)
	if !ok {
		return "", 0, notList
	}
	var perms []jsonMember
	if raw, found := member(root, "permissions"); found {
		if perms, ok = parseObject(raw); !ok {
			return "", 0, notList
		}
	}
	var deny []json.RawMessage
	if raw, found := member(perms, "deny"); found {
		if !bytes.HasPrefix(bytes.TrimSpace(raw), []byte("[")) || json.Unmarshal(raw, &deny) != nil {
			return "", 0, notList
		}
	}
	have := map[string]bool{}
	for _, raw := range deny {
		var s string
		if json.Unmarshal(raw, &s) == nil {
			have[s] = true
		}
	}
	for _, e := range required {
		if !have[e] {
			deny = append(deny, json.RawMessage(jsonString(e)))
			added++
		}
	}
	if added == 0 {
		return "", 0, ""
	}
	var list bytes.Buffer
	list.WriteByte('[')
	for i, raw := range deny {
		if i > 0 {
			list.WriteByte(',')
		}
		list.Write(raw)
	}
	list.WriteByte(']')
	perms = setMember(perms, "deny", list.Bytes())
	root = setMember(root, "permissions", encodeObject(perms))
	var compact, indented bytes.Buffer
	if json.Compact(&compact, encodeObject(root)) != nil || json.Indent(&indented, compact.Bytes(), "", "  ") != nil {
		return "", 0, notList
	}
	return indented.String() + "\n", added, ""
}

// opencodeJSON is written literally because rule order matters: in OpenCode the
// last matching rule wins, so *.env.example reopens what *.env.* closes. `*` matches any
// character, `/` included, so these rules also cover backend/.env and frontend/.env.prod (S19).
const opencodeJSONTemplate = `{
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

// opencodeJSON is opencodeJSONTemplate with the side copies of the skill scripts allowed right after the root ones,
// before the denies; a single-stack project gets the template unchanged.
func opencodeJSON(parts []stack.Part) string {
	const root = `      "bash .agents/skills/*": "allow",` + "\n"
	extra := ""
	for _, g := range sideSkills(parts) {
		extra += "      " + jsonString(g) + `: "allow",` + "\n"
	}
	return strings.Replace(opencodeJSONTemplate, root, root+extra, 1)
}

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
// created only if missing, then they belong to the user, except the side secret
// entries a combined project adds to Claude's settings (S20).
func Write(p *fsx.Project, parts []stack.Part, agents []string) {
	roles := readRoles(p, parts)
	if slices.Contains(agents, "claude-code") {
		claudeMd(p)
		for _, r := range roles {
			p.Generated(".claude/agents/"+r.name+".md", claudeAgent(r), 0)
		}
		claudeSettingsFile(p, parts)
	}
	if slices.Contains(agents, "opencode") {
		for _, r := range roles {
			p.Generated(".opencode/agents/"+r.name+".md", opencodeAgent(r, readZones(p, parts, r.name), parts), 0)
		}
		p.Create("opencode.json", opencodeJSON(parts), 0)
	}
	if slices.Contains(agents, "gemini-cli") {
		p.Create(".gemini/settings.json", geminiJSON, 0)
	}
	// Hermes reads AGENTS.md from the cwd: it gets no instruction file of its own (never .hermes.md).
}

// LinkSkills links every skill in .agents/skills/ into the folders of the tools
// that do not read it natively.
func LinkSkills(p *fsx.Project, parts []stack.Part, agents []string) {
	skills := p.Dirs(".agents/skills")
	for _, k := range stack.SkillsOf(parts) {
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
