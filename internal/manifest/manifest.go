// Package manifest reads and writes .agents/manifest.json (spec 02 §8).
package manifest

import (
	"bytes"
	"encoding/json"
	"fmt"
	"time"

	"github.com/F4R1DS4NCH3Z26/basekit-go/internal/fsx"
)

// Path of the manifest inside the project.
const Path = ".agents/manifest.json"

// External skills pinned per spec 02 §9. Change only after reviewing the upstream diff.
const (
	ExternalCLI    = "skills@1.7.4"
	ExternalSource = "https://github.com/mattpocock/skills"
	ExternalRef    = "5beb35b040e3de97e54fa9e386d7e42d15c295c2"
)

// ExternalNames are the four external skills (two wrappers and their dependencies).
var ExternalNames = []string{"grill-me", "grill-with-docs", "grilling", "domain-modeling"}

// Manifest keeps every field it was read with, so unknown fields survive rewrites.
type Manifest map[string]any

// Str returns a string field ("" when missing).
func (m Manifest) Str(key string) string {
	s, _ := m[key].(string)
	return s
}

// Strings returns a list-of-strings field.
func (m Manifest) Strings(key string) []string {
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

// Option returns options.<key> (nil when missing).
func (m Manifest) Option(key string) any {
	opts, _ := m["options"].(map[string]any)
	return opts[key]
}

// Files returns the hashes of the kit files basekit wrote.
func (m Manifest) Files() map[string]string {
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

// GlobalPath is the manifest of a global install, relative to the home directory.
const GlobalPath = ".agents/basekit.json"

// Read returns the project's manifest, or nil when there is none.
func Read(p *fsx.Project) (Manifest, error) { return ReadAt(p, Path) }

// ReadAt reads the manifest stored at path (Path or GlobalPath).
func ReadAt(p *fsx.Project, path string) (Manifest, error) {
	text, ok := p.Read(path)
	if !ok {
		return nil, p.Err()
	}
	var m Manifest
	if err := json.Unmarshal([]byte(text), &m); err != nil || m == nil {
		return nil, fmt.Errorf("%s no es JSON válido; corrígelo o bórralo antes de continuar", path)
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

// Data is what basekit decided. A global install (Global true) has no stack and
// no options: only agents and kit files.
type Data struct {
	Global  bool
	Version string
	Stack   string
	Agents  []string
	DB      string
	Redis   bool
	Docker  bool
	Files   map[string]string
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

// Write keeps createdAt and unknown fields; updatedAt only moves when something
// else changed.
func Write(p *fsx.Project, prev Manifest, d Data) {
	path := Path
	if d.Global {
		path = GlobalPath
	}
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
	opts["db"], opts["redis"], opts["docker"] = d.DB, d.Redis, d.Docker
	next["kit"] = "basekit"
	next["version"] = d.Version
	next["agents"] = append([]string{}, d.Agents...)
	if d.Global {
		next["scope"] = "global"
	} else {
		next["stack"] = d.Stack
		next["options"] = opts
	}
	next["externalSkills"] = map[string]any{
		"cli": ExternalCLI, "source": ExternalSource, "ref": ExternalRef, "skills": ExternalNames,
	}
	next["files"] = d.Files
	if prev.Str("createdAt") == "" {
		next["createdAt"] = now
	}
	same := prev != nil && withoutUpdatedAt(prev) == withoutUpdatedAt(next)
	next["updatedAt"] = now
	if same && !p.Changed && prev.Str("updatedAt") != "" {
		next["updatedAt"] = prev["updatedAt"]
	}
	p.Generated(path, toJSON(next), 0)
}
