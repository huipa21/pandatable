package structure

import (
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
)

func TestRowQueryPerformance200k(t *testing.T) {
	if os.Getenv("PT_RUN_ROW_PERF") != "1" {
		t.Skip("set PT_RUN_ROW_PERF=1 to run the documented 200k-row PG18 measurement")
	}
	f := newFixture(t)
	owner := f.user("user", "active", "en")
	ft := f.fieldTable(owner)
	field := ft.create(FieldInput{Name: "Payload", UIType: "text"})
	tableName, err := physicalTable(ft.pgSchema, ft.pgName)
	if err != nil {
		t.Fatal(err)
	}
	column := pgx.Identifier{ft.column(field)}.Sanitize()
	start := time.Now()
	if _, err := f.pool.Exec(f.ctx, fmt.Sprintf(
		"INSERT INTO %s (%s) SELECT 'row-' || value::text FROM generate_series(1, 200000) AS value",
		tableName, column)); err != nil {
		t.Fatal(err)
	}
	seedDuration := time.Since(start)
	viewID := ft.schema.Views[0].ID

	start = time.Now()
	page, err := f.service.QueryRows(f.ctx, owner, viewID, RowQueryRequest{
		Offset: intPtr(100_000), Limit: intPtr(maxQueryLimit), IncludeCount: boolPtr(false),
	})
	pageDuration := time.Since(start)
	if err != nil || len(page.Rows) != maxQueryLimit || page.Total != nil {
		t.Fatalf("page query returned %d rows, total=%v, err=%v", len(page.Rows), page.Total, err)
	}

	start = time.Now()
	counted, err := f.service.QueryRows(f.ctx, owner, viewID, RowQueryRequest{
		Limit: intPtr(maxQueryLimit), IncludeCount: boolPtr(true),
	})
	countDuration := time.Since(start)
	if err != nil || counted.Total == nil || *counted.Total != 200_000 {
		t.Fatalf("count query total=%v, err=%v", counted.Total, err)
	}
	t.Logf("fixture=generate_series(1,200000), seed=%s, page(size=100,offset=100000,count=false)=%s, query+exact-count=%s",
		seedDuration, pageDuration, countDuration)
}
