package basekit_test

import (
	"reflect"
	"strings"
	"testing"
)

const pinned = "-y skills@1.7.4 add https://github.com/mattpocock/skills/tree/5beb35b040e3de97e54fa9e386d7e42d15c295c2 " +
	"-s grill-me grill-with-docs grilling domain-modeling"

var externalSkills = []string{"grill-me", "grill-with-docs", "grilling", "domain-modeling"}

func argsCalls(log []string) []string {
	var calls []string
	for _, l := range log {
		if strings.HasPrefix(l, "ARGS") {
			calls = append(calls, l)
		}
	}
	return calls
}

func TestExternalSkillsInstalledPinnedWithTelemetryOff(t *testing.T) {
	npx := fakeNpx(t)
	d := scratch(t)
	r := run(t, "init", d, "-y", "--stack", "nextjs", "--agents", "claude-code,hermes-agent")
	mustOK(t, r)
	want := []string{"ARGS " + pinned + " -a claude-code hermes-agent -y", "ENV DISABLE_TELEMETRY=1 DO_NOT_TRACK=1"}
	if got := npx(); !reflect.DeepEqual(got, want) {
		t.Fatalf("%q", got)
	}
	for _, s := range externalSkills {
		if !exists(d, ".agents/skills/"+s+"/SKILL.md") {
			t.Fatal(s)
		}
	}
	match(t, r.stdout, `(?m)^\+ skills externas`)
}

func TestExternalSkillsNotReinstalled(t *testing.T) {
	npx := fakeNpx(t)
	d := scratch(t)
	mustOK(t, run(t, "init", d, "-y", "--stack", "nextjs"))
	r := run(t, "init", d, "-y")
	mustOK(t, r)
	if n := len(argsCalls(npx())); n != 1 {
		t.Fatalf("%d npx calls", n)
	}
	match(t, r.stdout, `(?m)^= skills externas`)
}

func TestExternalSkillsFailureDoesNotAbort(t *testing.T) {
	fakeNpx(t)
	t.Setenv("FAKE_NPX_FAIL", "1")
	d := scratch(t)
	r := run(t, "init", d, "-y", "--stack", "nextjs")
	mustOK(t, r)
	match(t, r.stdout, `(?m)^! skills externas.*npx -y skills@1\.7\.4 add`)
	if !exists(d, ".agents/manifest.json") {
		t.Fatal("manifest missing")
	}
}

func TestExternalSkillsDryRunRunsNoNpx(t *testing.T) {
	npx := fakeNpx(t)
	d := scratch(t)
	r := run(t, "init", d, "-y", "--stack", "nextjs", "--dry-run")
	mustOK(t, r)
	if got := npx(); got != nil {
		t.Fatal(got)
	}
	match(t, r.stdout, `ejecutaría: npx -y skills@1\.7\.4`)
}

func TestNoExternalSkillsFlagSkipsInstall(t *testing.T) {
	npx := fakeNpx(t)
	mustOK(t, initq(t, scratch(t), "--stack", "nextjs"))
	if got := npx(); got != nil {
		t.Fatal(got)
	}
}
