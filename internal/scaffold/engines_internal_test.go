package scaffold

import (
	"strings"
	"testing"

	"github.com/F4R1DS4NCH3Z26/basekit-go/internal/stack"
)

func labels(opts []Option) []string {
	var out []string
	for _, o := range opts {
		out = append(out, o.Label)
	}
	return out
}

// mongo-support S1: a stack without dbs offers postgres, mysql and sqlite; MongoDB only when the stack lists mongo.
func TestEngineOptionsOfferMongoOnlyWhenListed(t *testing.T) {
	cases := []struct {
		dbs  []string
		want string
	}{
		{nil, "MySQL,SQLite,PostgreSQL"},
		{[]string{"postgres", "mysql", "sqlite", "mongo"}, "MySQL,SQLite,PostgreSQL,MongoDB"},
		{[]string{"mongo"}, "MongoDB"},
	}
	for _, c := range cases {
		got := labels(engineOptions(stack.Stack{ID: "x", Kind: "backend", Dbs: c.dbs}))
		if joined := strings.Join(got, ","); joined != c.want {
			t.Errorf("dbs %v: %s", c.dbs, joined)
		}
	}
}

// mongo-support S1: an explicit --db mongo is accepted by a stack that lists it and rejected by one without dbs.
func TestCheckStackDBMongoNeedsListing(t *testing.T) {
	mongo := "mongo"
	listed := stack.Stack{ID: "x", Kind: "backend", Dbs: []string{"postgres", "mongo"}}
	if err := checkStackDB(listed, &mongo, false); err != nil {
		t.Fatalf("listed: %v", err)
	}
	plain := stack.Stack{ID: "y", Kind: "backend"}
	err := checkStackDB(plain, &mongo, false)
	if err == nil || err.Error() != "--db 'mongo' no aplica a y: motores admitidos: postgres, mysql, sqlite (o auto, none)" {
		t.Fatalf("default: %v", err)
	}
}
