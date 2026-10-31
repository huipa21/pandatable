package queryperf

import (
	"context"
	"os"
	"strings"
	"testing"

	"pandatable/internal/testdb"
)

var database testdb.Suite

func TestMain(m *testing.M) {
	os.Exit(database.Run(m))
}

func TestExplainQueryRejectsNonSelect(t *testing.T) {
	for _, statement := range []string{"", "DELETE FROM example", "UPDATE example SET x=1"} {
		if _, err := ExplainQuery(context.Background(), nil, TracedQuery{SQL: statement}); err == nil {
			t.Fatalf("accepted non-SELECT statement %q", statement)
		}
	}
}

func TestLinkedSeedRejectsInvalidInput(t *testing.T) {
	ctx := context.Background()
	pool := database.Open(t)
	for _, test := range []struct {
		seed LinkedQuerySeed
		want string
	}{
		{LinkedQuerySeed{Rows: 200_001, Projects: 1}, "fixture size"},
		{LinkedQuerySeed{Rows: 2_000, Projects: 1_001}, "fixture size"},
		{LinkedQuerySeed{Rows: 20, Projects: 1}, "identifier"},
	} {
		if err := SeedLinkedQuery(ctx, &database, pool, test.seed); err == nil ||
			!strings.Contains(err.Error(), test.want) {
			t.Fatalf("accepted invalid seed %+v: %v", test.seed, err)
		}
	}
}
