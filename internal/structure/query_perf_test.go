package structure

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"pandatable/internal/acl"
	"pandatable/internal/codec"
	"pandatable/internal/db"
	"pandatable/internal/testdb/queryperf"
)

const (
	queryPerfRows     = 200_000
	queryPerfProjects = 1_000
)

type queryPerfFixture struct {
	f                    *fixture
	owner                Actor
	viewID               string
	tableName            string
	label, status, score Field
	linkID               string
	rowCount             int
	projectCount         int
	threshold            int
}

func newQueryPerfFixture(t testing.TB, rowCount, projectCount int) *queryPerfFixture {
	t.Helper()
	f := newFixture(t)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	t.Cleanup(cancel)
	f.ctx = ctx
	owner := f.user("user", "active", "en")
	book := f.book(owner, f.workspace(owner).ID)
	projects, projectLabel := relationTable(t, f, owner, book.ID, "Projects")
	tasks, taskLabel := relationTable(t, f, owner, book.ID, "Tasks")
	schema, name := f.physicalNames(tasks.Table.ID)
	taskTable := &fieldTable{f: f, owner: owner, schema: tasks, pgSchema: schema, pgName: name}
	status := taskTable.create(FieldInput{Name: "Status", UIType: "text"})
	score := taskTable.create(FieldInput{Name: "Score", UIType: "integer", Options: json.RawMessage(`{"size":"int"}`)})
	relation, err := f.service.CreateRelation(ctx, owner, tasks.Table.ID, RelationInput{
		Name: "Project", ReverseName: "Tasks", TargetTableID: projects.Table.ID,
		Kind: "one_many", SourceSide: "child", OnDelete: "set_null",
	})
	if err != nil {
		t.Fatal(err)
	}
	fk := scanOne[string](f, "SELECT fk_column FROM meta.relations WHERE id=$1", relation.Relation.ID)
	projectSchema, projectName := f.physicalNames(projects.Table.ID)
	projectTable, err := physicalTable(projectSchema, projectName)
	if err != nil {
		t.Fatal(err)
	}
	taskName, err := physicalTable(schema, name)
	if err != nil {
		t.Fatal(err)
	}
	seedStart := time.Now()
	err = queryperf.SeedLinkedQuery(ctx, &database, f.pool, queryperf.LinkedQuerySeed{
		ProjectSchema: projectSchema, ProjectTable: projectName,
		ProjectLabel: scanOne[string](f, "SELECT pg_column FROM meta.fields WHERE id=$1", projectLabel.ID),
		TaskSchema:   schema, TaskTable: name, TaskLabel: taskTable.column(taskLabel),
		Status: taskTable.column(status), Score: taskTable.column(score), ProjectFK: fk,
		Rows: rowCount, Projects: projectCount,
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("fixture: %d tasks, %d projects, one persisted FK per task; seed=%s",
		rowCount, projectCount, time.Since(seedStart))

	threshold := rowCount - rowCount/20
	columns := []ViewColumn{
		{ID: taskLabel.ID, Visible: true}, {ID: status.ID, Visible: true},
		{ID: score.ID, Visible: true}, {ID: relation.Relation.ChildFieldID, Visible: true},
	}
	for _, field := range tasks.Fields {
		if !field.IsSystem && field.ID != taskLabel.ID {
			columns = append(columns, ViewColumn{ID: field.ID, Visible: false})
		}
	}
	viewID := tasks.Views[0].ID
	version := scanOne[int32](f, "SELECT version FROM meta.views WHERE id=$1", viewID)
	_, err = f.service.UpdateViewColumns(ctx, owner, viewID, version, ViewColumns{
		Fields: columns, Sort: []ViewSort{{Field: score.ID, Dir: "asc"}},
		GroupBy:    []string{status.ID},
		Aggregates: []ViewAggregate{{FieldID: score.ID, Aggregate: codec.AggSum}},
		Filter:     json.RawMessage(fmt.Sprintf(`{"field":%q,"cmp":"gte","value":%d}`, score.ID, threshold)),
		FilterSet:  true,
	})
	if err != nil {
		t.Fatalf("configure shared view: %v", err)
	}
	if rowCount > autoIndexThreshold {
		indexStart := time.Now()
		if err := f.service.scanAutoIndexes(ctx); err != nil {
			t.Fatalf("create shared-view indexes: %v", err)
		}
		for _, field := range []Field{score, status} {
			if !f.scanIndexExists(taskTable, field.ID, automaticBtree) {
				t.Fatalf("missing shared-view btree for %s", field.Name)
			}
		}
		t.Logf("shared-view index scan=%s", time.Since(indexStart))
	}
	f.exec("ANALYZE " + projectTable)
	f.exec("ANALYZE " + taskName)
	return &queryPerfFixture{
		f: f, owner: owner, viewID: viewID, tableName: taskName,
		label: taskLabel, status: status, score: score, linkID: relation.Relation.ChildFieldID,
		rowCount: rowCount, projectCount: projectCount, threshold: threshold,
	}
}

type queryPerfResult struct {
	rows   RowQueryResult
	groups RowGroupResult
}

func (p *queryPerfFixture) run(service *Service, name string) (queryPerfResult, error) {
	fields := []string{p.label.ID, p.status.ID, p.score.ID, p.linkID}
	switch name {
	case "filter":
		noSort := []RowSort{}
		result, err := service.QueryRows(p.f.ctx, p.owner, p.viewID, RowQueryRequest{
			Offset: intPtr(0), Limit: intPtr(100), IncludeCount: boolPtr(true), FieldIDs: &fields, Sorts: &noSort,
			Filter: &FilterNode{Field: p.score.ID, Cmp: codec.OpGte,
				Value: json.RawMessage(strconv.Itoa(p.threshold))},
		})
		return queryPerfResult{rows: result}, err
	case "sort":
		result, err := service.QueryRows(p.f.ctx, p.owner, p.viewID, RowQueryRequest{
			Offset: intPtr(0), Limit: intPtr(100), IncludeCount: boolPtr(true), FieldIDs: &fields,
			Sort: &RowSort{FieldID: p.score.ID, Direction: "desc"},
		})
		return queryPerfResult{rows: result}, err
	case "group":
		result, err := service.QueryGroups(p.f.ctx, p.owner, p.viewID, RowGroupRequest{Path: []json.RawMessage{}})
		return queryPerfResult{groups: result}, err
	default:
		return queryPerfResult{}, fmt.Errorf("unknown query performance case %q", name)
	}
}

func (p *queryPerfFixture) verify(t testing.TB, name string, result queryPerfResult) {
	t.Helper()
	type groupExpectation struct {
		count, sum int64
		hasScore   bool
	}
	groups := make(map[string]groupExpectation)
	filtered := 0
	for id := 1; id <= p.rowCount; id++ {
		key := queryperf.LinkedQueryStatus(id)
		group := groups[key]
		group.count++
		if score, valid := queryperf.LinkedQueryScore(id, p.rowCount); valid {
			group.sum += score
			group.hasScore = true
			if score >= int64(p.threshold) {
				filtered++
			}
		}
		groups[key] = group
	}
	if name == "group" {
		if len(result.groups.Groups) != len(groups) {
			t.Fatalf("got %d groups, want %d", len(result.groups.Groups), len(groups))
		}
		seen := make(map[string]bool)
		for _, bucket := range result.groups.Groups {
			var key *string
			if err := json.Unmarshal(bucket.Key, &key); err != nil {
				t.Fatalf("decode group key %s: %v", bucket.Key, err)
			}
			name := ""
			if key != nil {
				name = *key
			}
			want, ok := groups[name]
			if !ok || seen[name] || bucket.Count != want.count || len(bucket.Aggregates) != 1 {
				t.Fatalf("unexpected group %s: %+v", bucket.Key, bucket)
			}
			seen[name] = true
			aggregate := bucket.Aggregates[0]
			var sum *int64
			if err := json.Unmarshal(aggregate.Value, &sum); err != nil {
				t.Fatalf("decode aggregate %s: %v", aggregate.Value, err)
			}
			if aggregate.FieldID != p.score.ID || aggregate.Aggregate != codec.AggSum ||
				(sum != nil) != want.hasScore || sum != nil && *sum != want.sum {
				t.Fatalf("group %s sum=%s, want %+v", bucket.Key, aggregate.Value, want)
			}
		}
		return
	}
	total := p.rowCount
	if name == "filter" {
		total = filtered
	}
	page := result.rows
	if page.Total == nil || *page.Total != int64(total) || len(page.Rows) != min(100, total) {
		t.Fatalf("%s: total=%v, rows=%d, want total=%d and rows=%d",
			name, page.Total, len(page.Rows), total, min(100, total))
	}
	var previousID int
	var previousScore *int64
	for index, row := range page.Rows {
		id, err := strconv.Atoi(row.ID)
		if err != nil || id < 1 || id > p.rowCount {
			t.Fatalf("invalid task ID %q: %v", row.ID, err)
		}
		var score *int64
		if err := json.Unmarshal(row.Fields[p.score.ID], &score); err != nil {
			t.Fatalf("decode score %s: %v", row.Fields[p.score.ID], err)
		}
		expectedScore, hasScore := queryperf.LinkedQueryScore(id, p.rowCount)
		if !hasScore {
			if score != nil {
				t.Fatalf("row %d: score=%v, want null", id, *score)
			}
		} else if score == nil || *score != expectedScore {
			t.Fatalf("row %d: score=%v does not match fixture", id, score)
		}
		if name == "filter" && (score == nil || *score < int64(p.threshold) || index > 0 && id <= previousID) {
			t.Fatalf("filter returned an unexpected row or order: %+v", row)
		}
		if name == "sort" && index > 0 &&
			(previousScore == nil && score != nil || previousScore != nil && score != nil &&
				(*score > *previousScore || *score == *previousScore && id <= previousID)) {
			t.Fatalf("sort returned an unexpected row or order: %+v", row)
		}
		projectID := queryperf.LinkedQueryProjectID(id, p.projectCount)
		var links []LinkValue
		if err := json.Unmarshal(row.Fields[p.linkID], &links); err != nil || len(links) != 1 ||
			links[0].ID != strconv.Itoa(projectID) || links[0].Label != fmt.Sprintf("Project %04d", projectID) {
			t.Fatalf("row %d: links=%s, expected project %d: %v", id, row.Fields[p.linkID], projectID, err)
		}
		previousID, previousScore = id, score
	}
}

func TestTracedQueryBoundArgs(t *testing.T) {
	query := queryperf.TracedQuery{Args: []any{pgx.QueryExecModeDescribeExec, 190_000}}
	args := query.BoundArgs()
	if len(args) != 1 || args[0] != 190_000 {
		t.Fatalf("bound args = %#v", args)
	}
	query.Args = []any{3}
	if args := query.BoundArgs(); len(args) != 1 || args[0] != 3 {
		t.Fatalf("ordinary integer argument was lost: %#v", args)
	}
}

func (p *queryPerfFixture) tracedService(t testing.TB) (*Service, *queryperf.QueryTracer) {
	t.Helper()
	tracer := &queryperf.QueryTracer{}
	config := p.f.pool.Config()
	config.ConnConfig.Tracer = tracer
	pool, err := pgxpool.NewWithConfig(p.f.ctx, config)
	if err != nil {
		t.Fatal("create trace pool: ", err)
	}
	t.Cleanup(pool.Close)
	if err := pool.Ping(p.f.ctx); err != nil {
		t.Fatal("ping trace pool: ", err)
	}
	wrapped := &db.Pool{Pool: pool}
	resolver, err := acl.NewResolver(wrapped)
	if err != nil {
		t.Fatal(err)
	}
	service, err := New(wrapped, resolver)
	if err != nil {
		t.Fatal(err)
	}
	return service, tracer
}

func (p *queryPerfFixture) relevantQueries(t testing.TB, name string, traced []queryperf.TracedQuery) map[string]queryperf.TracedQuery {
	t.Helper()
	found, err := queryperf.LinkedQueryStatements(p.tableName, name == "group", traced)
	if err != nil {
		t.Fatalf("%s: %v", name, err)
	}
	return found
}

func (p *queryPerfFixture) explain(t testing.TB, query queryperf.TracedQuery) string {
	t.Helper()
	plan, err := queryperf.ExplainQuery(p.f.ctx, p.f.pool, query)
	if err != nil {
		t.Fatal(err)
	}
	return plan
}

func TestQueryPerformanceFixture(t *testing.T) {
	p := newQueryPerfFixture(t, 40, 5)
	service, tracer := p.tracedService(t)
	for _, name := range []string{"filter", "sort", "group"} {
		t.Run(name, func(t *testing.T) {
			tracer.Start()
			result, err := p.run(service, name)
			queries := tracer.Stop()
			if err != nil {
				t.Fatal(err)
			}
			p.verify(t, name, result)
			for _, query := range p.relevantQueries(t, name, queries) {
				p.explain(t, query)
			}
		})
	}
}

func TestQueryPerformance200k(t *testing.T) {
	if os.Getenv("PT_RUN_QUERY_PERF") != "1" {
		t.Skip("set PT_RUN_QUERY_PERF=1 to measure the 200k linked-row PG18 query fixture")
	}
	p := newQueryPerfFixture(t, queryPerfRows, queryPerfProjects)
	service, tracer := p.tracedService(t)
	t.Logf("conditions: seeded/analyzed caches not evicted; first and repeated requests; trace pool pinged; filter score >= %d, no sort, page 0/100, exact count; sort score DESC NULLS LAST, page 0/100, exact count; group status with sum(score), no filter; linked Project field visible",
		p.threshold)
	captured := make(map[string]map[string]queryperf.TracedQuery)
	for _, name := range []string{"filter", "sort", "group"} {
		for _, sample := range []string{"first", "warm"} {
			tracer.Start()
			start := time.Now()
			result, err := p.run(service, name)
			elapsed := time.Since(start)
			queries := tracer.Stop()
			if err != nil {
				t.Fatalf("%s %s: %v", name, sample, err)
			}
			p.verify(t, name, result)
			t.Logf("%s %s service elapsed=%s, target <1s: %t", name, sample, elapsed, elapsed < time.Second)
			if elapsed >= time.Second {
				t.Errorf("%s %s exceeded the strict <1s comparison", name, sample)
			}
			if sample == "warm" {
				captured[name] = p.relevantQueries(t, name, queries)
			}
		}
	}
	for _, name := range []string{"filter", "sort", "group"} {
		for _, kind := range []string{"page", "count", "links", "group"} {
			query, ok := captured[name][kind]
			if !ok {
				continue
			}
			plan := p.explain(t, query)
			t.Logf("%s %s captured SQL=%s\nbound args=%#v\nEXPLAIN (ANALYZE, BUFFERS):\n%s",
				name, kind, query.SQL, query.BoundArgs(), plan)
		}
	}
}

func BenchmarkQueryPerformance200k(b *testing.B) {
	if os.Getenv("PT_RUN_QUERY_PERF") != "1" {
		b.Skip("set PT_RUN_QUERY_PERF=1 to benchmark the 200k linked-row PG18 query fixture")
	}
	p := newQueryPerfFixture(b, queryPerfRows, queryPerfProjects)
	for _, name := range []string{"filter", "sort", "group"} {
		b.Run(name, func(b *testing.B) {
			var result queryPerfResult
			var err error
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				result, err = p.run(p.f.service, name)
				if err != nil {
					b.Fatal(err)
				}
			}
			b.StopTimer()
			p.verify(b, name, result)
		})
	}
}
