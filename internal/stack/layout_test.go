package stack

import (
	"path"
	"regexp"
	"strconv"
	"strings"
	"testing"
)

func loadAll(t *testing.T) []Stack {
	t.Helper()
	stacks, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	return stacks
}

func firstOfKind(t *testing.T, stacks []Stack, kind string) Stack {
	t.Helper()
	for _, s := range stacks {
		if s.Kind == kind {
			return s
		}
	}
	t.Fatalf("no %s stack", kind)
	return Stack{}
}

func TestResolvePartsSingleAndPair(t *testing.T) {
	stacks := loadAll(t)
	back, front := firstOfKind(t, stacks, "backend"), firstOfKind(t, stacks, "frontend")

	single, err := ResolveParts(stacks, back.ID)
	if err != nil || len(single) != 1 || single[0].Stack.ID != back.ID || single[0].Side != "" {
		t.Fatalf("%v %v", single, err)
	}
	if IsCombined(single) || SpecOf(single) != back.ID || DisplayName(single) != back.Name {
		t.Fatalf("single: combined %v spec %q name %q", IsCombined(single), SpecOf(single), DisplayName(single))
	}
	if InPart(single[0], "Makefile") != "Makefile" || RoleIn(single[0], "qa-writer") != "qa-writer" || NewAppCommand(single[0]) != "make new-app" {
		t.Fatal("a single stack lives at the root")
	}

	spec := back.ID + "+" + front.ID
	pair, err := ResolveParts(stacks, spec)
	if err != nil || len(pair) != 2 || pair[0].Side != "backend" || pair[1].Side != "frontend" {
		t.Fatalf("%v %v", pair, err)
	}
	if !IsCombined(pair) || SpecOf(pair) != spec {
		t.Fatalf("combined %v spec %q", IsCombined(pair), SpecOf(pair))
	}
	if want := back.Name + " en backend/ + " + front.Name + " en frontend/"; DisplayName(pair) != want {
		t.Fatalf("display %q, want %q", DisplayName(pair), want)
	}
	if InPart(pair[1], "Makefile") != "frontend/Makefile" || RoleIn(pair[0], "qa-writer") != "backend-qa-writer" ||
		NewAppCommand(pair[1]) != "make -C frontend new-app" {
		t.Fatal("a side lives in its folder")
	}
	if got, err := ResolveParts(stacks, " "+back.ID+" + "+front.ID+" "); err != nil || SpecOf(got) != spec {
		t.Fatalf("pair ids are trimmed: %v %v", got, err)
	}
}

func TestResolvePartsRejectsBadPairs(t *testing.T) {
	stacks := loadAll(t)
	hint := "Backend: " + idsOfKind(stacks, "backend") + ". Frontend: " + idsOfKind(stacks, "frontend")
	back, front, full := firstOfKind(t, stacks, "backend"), firstOfKind(t, stacks, "frontend"), firstOfKind(t, stacks, "fullstack")
	cases := map[string]string{
		front.ID + "+" + back.ID: "--stack '" + front.ID + "+" + back.ID + "': " + front.ID + " es frontend, y en <backend>+<frontend> la primera parte es un stack backend. " + hint,
		back.ID + "+" + back.ID:  "--stack '" + back.ID + "+" + back.ID + "': " + back.ID + " es backend, y en <backend>+<frontend> la segunda parte es un stack frontend. " + hint,
		full.ID + "+" + front.ID: "--stack '" + full.ID + "+" + front.ID + "': " + full.ID + " es fullstack, y en <backend>+<frontend> la primera parte es un stack backend. " + hint,
		back.ID + "+nope":        "--stack 'nope' no existe. Disponibles: " + IDs(stacks),
		back.ID + "+":            "--stack '" + back.ID + "+' no es válido: un proyecto combinado es <backend>+<frontend> (un stack backend y uno frontend). " + hint,
		"a+b+c":                  "--stack 'a+b+c' no es válido: un proyecto combinado es <backend>+<frontend> (un stack backend y uno frontend). " + hint,
	}
	for spec, want := range cases {
		if _, err := ResolveParts(stacks, spec); err == nil || err.Error() != want {
			t.Errorf("%s: %v\nwant: %s", spec, err, want)
		}
		if PartsOf(stacks, spec) != nil {
			t.Errorf("PartsOf(%s) must be nil", spec)
		}
	}
	if _, err := ResolveParts(stacks, "nope"); err == nil || err.Error() != "--stack 'nope' no existe. Disponibles: "+IDs(stacks) {
		t.Errorf("single unknown id: %v", err)
	}
	if PartsOf(stacks, "") != nil {
		t.Error("PartsOf of an empty spec")
	}
}

// Node's tests/kit/layout.test.ts: each half of a combined project points at the other.
func TestWireEnv(t *testing.T) {
	stacks := loadAll(t)
	back, front := firstOfKind(t, stacks, "backend"), firstOfKind(t, stacks, "frontend")
	parts, err := ResolveParts(stacks, back.ID+"+"+front.ID)
	if err != nil {
		t.Fatal(err)
	}
	api := "http://127.0.0.1:" + itoa(back.Port)
	web := "http://127.0.0.1:" + itoa(front.Port)

	if text, warn := WireEnv(parts, parts[0], "A=1\nCORS_ORIGINS=x\n"); text != "A=1\nCORS_ORIGINS="+web+",http://localhost:"+itoa(front.Port)+"\n" || warn != "" {
		t.Errorf("backend: %q %q", text, warn)
	}
	if text, warn := WireEnv(parts, parts[1], "VITE_API_URL=x\nB=2\n"); text != "VITE_API_URL="+api+"\nB=2\n" || warn != "" {
		t.Errorf("frontend: %q %q", text, warn)
	}
	text, warn := WireEnv(parts, parts[0], "A=1\n")
	if text != "A=1\n" || !regexp.MustCompile(`^`+back.ID+` no tiene CORS_ORIGINS: .*`+regexp.QuoteMeta(web)).MatchString(warn) {
		t.Errorf("backend without CORS_ORIGINS: %q %q", text, warn)
	}
	if warn != back.ID+" no tiene CORS_ORIGINS: la API debe permitir (CORS) el origen "+web+" del frontend" {
		t.Errorf("warning text: %q", warn)
	}
	text, warn = WireEnv(parts, parts[1], "B=2\n")
	if text != "B=2\n" || warn != front.ID+" no tiene VITE_API_URL: apunta el frontend a la API en "+api {
		t.Errorf("frontend without VITE_API_URL: %q %q", text, warn)
	}
	single := parts[:1]
	single[0].Side = ""
	if text, warn := WireEnv(single, single[0], "A=1\n"); text != "A=1\n" || warn != "" {
		t.Errorf("single: %q %q", text, warn)
	}
}

func TestNameCompose(t *testing.T) {
	part := Part{Stack: Stack{ID: "x"}, Side: "backend"}
	if got := NameCompose("/tmp/Mi.Tienda", part, "# c\nservices:\n  app: {}\n"); got != "# c\nname: mitienda-backend\nservices:\n  app: {}\n" {
		t.Errorf("%q", got)
	}
	if got := NameCompose("/tmp/__", part, "services: {}\n"); got != "name: app-backend\nservices: {}\n" {
		t.Errorf("empty name falls back to app: %q", got)
	}
	if got := NameCompose("/tmp/p", part, "name: own\nservices: {}\n"); got != "name: own\nservices: {}\n" {
		t.Errorf("an existing name stays: %q", got)
	}
	if got := NameCompose("/tmp/p", Part{Stack: Stack{ID: "x"}}, "services: {}\n"); got != "services: {}\n" {
		t.Errorf("a single stack is copied as is: %q", got)
	}
}

func TestRoleAndZoneFilesOfASingleStackAreTheTemplates(t *testing.T) {
	stacks := loadAll(t)
	s := firstOfKind(t, stacks, "backend")
	parts, _ := ResolveParts(stacks, s.ID)
	roles := RoleFiles(parts)
	if len(roles) != len(Walk(path.Join(Dir(s), "roles"))) {
		t.Fatalf("%d roles", len(roles))
	}
	for _, f := range roles {
		if f.Text != Read(path.Join(Dir(s), "roles", f.Rel)) {
			t.Errorf("role %s changed", f.Rel)
		}
	}
	zones := ZoneFiles(parts)
	last := zones[len(zones)-1]
	stackZones, _ := ReadOptional(path.Join(Dir(s), "zones/orchestrator.stack.txt"))
	if last.Rel != "orchestrator.txt" || last.Text != Read("core/zones.orchestrator.common.txt")+stackZones {
		t.Errorf("orchestrator zones: %q", last.Text)
	}
	for _, f := range zones[:len(zones)-1] {
		if f.Text != Read(path.Join(Dir(s), "zones", f.Rel)) {
			t.Errorf("zone %s changed", f.Rel)
		}
	}
}

func TestRoleAndZoneFilesOfACombinedProjectArePrefixedBySide(t *testing.T) {
	stacks := loadAll(t)
	back, front := firstOfKind(t, stacks, "backend"), firstOfKind(t, stacks, "frontend")
	parts, _ := ResolveParts(stacks, back.ID+"+"+front.ID)
	for _, f := range RoleFiles(parts) {
		if !strings.HasPrefix(f.Rel, "backend-") && !strings.HasPrefix(f.Rel, "frontend-") {
			t.Errorf("unprefixed role %s", f.Rel)
		}
		name := strings.TrimSuffix(f.Rel, ".md")
		if !regexp.MustCompile(`(?m)^name: ` + name + `$`).MatchString(f.Text) {
			t.Errorf("%s: name not rewritten:\n%s", f.Rel, f.Text)
		}
		if !strings.Contains(f.Text, "> **Proyecto combinado: trabajas en `") || !strings.Contains(f.Text, ".agents/zones/"+name+".txt") {
			t.Errorf("%s: no side note", f.Rel)
		}
	}
	for _, f := range ZoneFiles(parts) {
		for _, l := range strings.Split(f.Text, "\n") {
			l = strings.TrimSpace(l)
			if l == "" || strings.HasPrefix(l, "#") || f.Rel == "orchestrator.txt" {
				continue
			}
			side, _, _ := strings.Cut(f.Rel, "-")
			if !strings.HasPrefix(l, side+"/") {
				t.Errorf("%s: glob %q outside %s/", f.Rel, l, side)
			}
		}
	}
	orch := ZoneFiles(parts)[len(ZoneFiles(parts))-1]
	if !strings.HasPrefix(orch.Text, Read("core/zones.orchestrator.common.txt")) ||
		!strings.Contains(orch.Text, "# backend/ ("+back.Name+")\n") || !strings.Contains(orch.Text, "# frontend/ ("+front.Name+")\n") {
		t.Errorf("orchestrator zones:\n%s", orch.Text)
	}
}

func itoa(n int) string { return strconv.Itoa(n) }
