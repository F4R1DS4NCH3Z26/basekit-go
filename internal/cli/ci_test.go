package cli_test

import (
	"os"
	"regexp"
	"strings"
	"testing"

	"github.com/F4R1DS4NCH3Z26/basekit-go/internal/stack"
)

type ciRow struct{ stack, db, env string }

// e2eMatrix reads the e2e job's matrix rows: `- stack: x` (with an indented `db:` line) or
// `- { stack: x, db: y, env: prod }`.
func e2eMatrix(t *testing.T) []ciRow {
	t.Helper()
	b, err := os.ReadFile("../../.github/workflows/ci.yml")
	if err != nil {
		t.Fatal(err)
	}
	ci := string(b)
	job := ci[strings.Index(ci, "\n  e2e:"):]
	include := job[strings.Index(job, "include:"):strings.Index(job, "steps:")]
	flow := regexp.MustCompile(`^\s*- \{(.*)\}\s*$`)
	block := regexp.MustCompile(`^\s*- stack: (\S+)\s*$`)
	extra := regexp.MustCompile(`^\s+(db|env): (\S+)\s*$`)
	var rows []ciRow
	set := func(r *ciRow, k, v string) {
		switch k {
		case "stack":
			r.stack = v
		case "db":
			r.db = v
		case "env":
			r.env = v
		default:
			t.Fatalf("unknown matrix key %q", k)
		}
	}
	for _, line := range strings.Split(include, "\n") {
		switch {
		case flow.MatchString(line):
			var r ciRow
			for _, kv := range strings.Split(flow.FindStringSubmatch(line)[1], ",") {
				k, v, _ := strings.Cut(kv, ":")
				set(&r, strings.TrimSpace(k), strings.TrimSpace(v))
			}
			rows = append(rows, r)
		case block.MatchString(line):
			rows = append(rows, ciRow{stack: block.FindStringSubmatch(line)[1]})
		case extra.MatchString(line) && len(rows) > 0:
			m := extra.FindStringSubmatch(line)
			set(&rows[len(rows)-1], m[1], m[2])
		}
	}
	return rows
}

func TestCIRunsEveryStackInDevAndProd(t *testing.T) {
	rows := e2eMatrix(t)
	for _, s := range allStacks {
		dev, prod := false, false
		for _, r := range rows {
			if r.stack != s.ID {
				continue
			}
			dev = dev || r.env == ""
			prod = prod || r.env == "prod"
			if s.Kind == "frontend" && r.db != "" {
				t.Errorf("%s: a frontend job has a db", s.ID)
			}
		}
		if !dev || !prod {
			t.Errorf("%s: dev job %v, prod job %v", s.ID, dev, prod)
		}
	}
	for _, r := range rows {
		if !strings.Contains(" "+strings.Join(stackIDs, " ")+" ", " "+r.stack+" ") {
			t.Errorf("unknown stack in the matrix: %+v", r)
		}
	}
}

// mongo-support S3: every stack whose stack.json lists mongo gets a dev e2e job with mongo.
func TestCIRunsMongoInDevForEveryStackThatListsIt(t *testing.T) {
	rows := e2eMatrix(t)
	for _, s := range allStacks {
		if !strings.Contains(" "+strings.Join(stack.AcceptedEngines(s), " ")+" ", " mongo ") {
			continue
		}
		found := false
		for _, r := range rows {
			found = found || (r.stack == s.ID && r.db == "mongo" && r.env == "")
		}
		if !found {
			t.Errorf("%s: no dev job with mongo", s.ID)
		}
	}
}

// Matrix values reach the e2e step only through env, never pasted into the command.
func TestCIE2EStepTakesMatrixValuesThroughEnv(t *testing.T) {
	b, err := os.ReadFile("../../.github/workflows/ci.yml")
	if err != nil {
		t.Fatal(err)
	}
	run := regexp.MustCompile(`(?m)^\s*run: (go test .*BASEKIT_E2E_STACK.*)$`).FindStringSubmatch(string(b))
	if run == nil {
		t.Fatal("no e2e go test step")
	}
	noMatch(t, run[1], `\$\{\{`)
}
