package basekit

import (
	"bytes"
	"encoding/json"
	"fmt"
	"time"
)

const manifestPath = ".agents/manifest.json"

// External skills pinned per spec 02 §9. Change only after reviewing the upstream diff.
const (
	externalCLI    = "skills@1.7.4"
	externalSource = "https://github.com/mattpocock/skills"
	externalRef    = "5beb35b040e3de97e54fa9e386d7e42d15c295c2"
)

var externalNames = []string{"grill-me", "grill-with-docs", "grilling", "domain-modeling"}

// Manifest keeps every field it was read with, so unknown fields survive rewrites
// (spec 02 §8).
type Manifest map[string]any

func (m Manifest) str(key string) string {
	s, _ := m[key].(string)
	return s
}

func (m Manifest) strings(key string) []string {
	var out []string
	if list, ok := m[key].([]any); ok {
		for _, v := range list {
			if s, ok := v.(string); ok {
				out = append(out, s)
			}
		}
	}
	return out
}

func (m Manifest) option(key string) any {
	opts, _ := m["options"].(map[string]any)
	return opts[key]
}

func (m Manifest) files() map[string]string {
	out := map[string]string{}
	if files, ok := m["files"].(map[string]any); ok {
		for k, v := range files {
			if s, ok := v.(string); ok {
				out[k] = s
			}
		}
	}
	return out
}

func readManifest(p *Project) (Manifest, error) {
	text, ok := p.read(manifestPath)
	if !ok {
		return nil, p.Err()
	}
	var m Manifest
	if err := json.Unmarshal([]byte(text), &m); err != nil || m == nil {
		return nil, fmt.Errorf("%s no es JSON válido; corrígelo o bórralo antes de continuar", manifestPath)
	}
	return m, nil
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

type manifestData struct {
	stack  string
	agents []string
	db     string
	redis  bool
	docker bool
	files  map[string]string
}

func withoutUpdatedAt(m Manifest) string {
	c := Manifest{}
	for k, v := range m {
		if k != "updatedAt" {
			c[k] = v
		}
	}
	return toJSON(c)
}

// writeManifest keeps createdAt and unknown fields; updatedAt only moves when
// something else changed.
func writeManifest(p *Project, prev Manifest, d manifestData) {
	now := time.Now().UTC().Format("2006-01-02T15:04:05.000Z")
	next := Manifest{}
	for k, v := range prev {
		next[k] = v
	}
	opts := map[string]any{}
	if o, ok := prev["options"].(map[string]any); ok {
		for k, v := range o {
			opts[k] = v
		}
	}
	opts["db"], opts["redis"], opts["docker"] = d.db, d.redis, d.docker
	next["kit"] = "basekit"
	next["version"] = Version
	next["stack"] = d.stack
	next["agents"] = append([]string{}, d.agents...)
	next["options"] = opts
	next["externalSkills"] = map[string]any{
		"cli": externalCLI, "source": externalSource, "ref": externalRef, "skills": externalNames,
	}
	next["files"] = d.files
	if prev.str("createdAt") == "" {
		next["createdAt"] = now
	}
	same := prev != nil && withoutUpdatedAt(prev) == withoutUpdatedAt(next)
	next["updatedAt"] = now
	if same && !p.Changed && prev.str("updatedAt") != "" {
		next["updatedAt"] = prev["updatedAt"]
	}
	p.generated(manifestPath, toJSON(next), 0)
}
