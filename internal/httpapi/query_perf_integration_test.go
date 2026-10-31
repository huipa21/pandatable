package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"strconv"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"pandatable/internal/acl"
	"pandatable/internal/auth"
	"pandatable/internal/codec"
	"pandatable/internal/db"
	"pandatable/internal/ddl"
	"pandatable/internal/structure"
	"pandatable/internal/testdb/queryperf"
)

const (
	httpQueryPerfRows     = 200_000
	httpQueryPerfProjects = 1_000
)

type httpQueryPerfFixture struct {
	f                    *aclFixture
	owner                *aclUser
	viewID               string
	tableName            string
	label, status, score structure.Field
	linkID               string
	rows, projects       int
	threshold            int
}

func newHTTPQueryPerfFixture(t *testing.T, rows, projects int) *httpQueryPerfFixture {
	t.Helper()
	f := newACLFixture(t)
	owner := f.user("user", false)
	book := f.newBook(owner, f.newWorkspace(owner, "Query performance").ID, "Book")
	projectTable := f.newTable(owner, book.ID, "Projects")
	taskTable := f.newTable(owner, book.ID, "Tasks")
	if taskTable.Table.DisplayFieldID == nil {
		t.Fatal("task table is missing its default display field")
	}
	defaultTaskLabel := *taskTable.Table.DisplayFieldID
	projectLabel := f.newField(owner, projectTable.Table.ID, `{"name":"Label","uiType":"text"}`)
	taskLabel := f.newField(owner, taskTable.Table.ID, `{"name":"Label","uiType":"text"}`)
	for _, item := range []struct {
		table, field string
	}{{projectTable.Table.ID, projectLabel.ID}, {taskTable.Table.ID, taskLabel.ID}} {
		f.expectStatus(f.fieldCall(owner, "PATCH", "/tables/"+item.table,
			fmt.Sprintf(`{"displayFieldId":%q}`, item.field)), http.StatusOK, "")
	}
	status := f.newField(owner, taskTable.Table.ID, `{"name":"Status","uiType":"text"}`)
	score := f.newField(owner, taskTable.Table.ID, `{"name":"Score","uiType":"integer","options":{"size":"int"}}`)
	relation := decode[relationBody](f, f.api(owner, "POST", "/tables/"+taskTable.Table.ID+"/relations",
		fmt.Sprintf(`{"name":"Project","reverseName":"Tasks","targetTableId":%q,"kind":"one_many","sourceSide":"child"}`,
			projectTable.Table.ID)), http.StatusCreated)
	fk := f.scan("SELECT fk_column FROM meta.relations WHERE id=$1", relation.Relation.ID)
	physical := func(tableID string) (string, string, string) {
		t.Helper()
		var schema, name string
		if err := f.pool.QueryRow(f.ctx, `SELECT b.pg_schema,t.pg_name FROM meta.tables t
			JOIN meta.books b ON b.id=t.book_id WHERE t.id=$1`, tableID).Scan(&schema, &name); err != nil {
			t.Fatal(err)
		}
		if err := ddl.ValidateIdentifier(ddl.Book, schema); err != nil {
			t.Fatal(err)
		}
		if err := ddl.ValidateIdentifier(ddl.Table, name); err != nil {
			t.Fatal(err)
		}
		return schema, name, pgx.Identifier{schema, name}.Sanitize()
	}
	projectSchema, projectName, projectSQL := physical(projectTable.Table.ID)
	taskSchema, taskName, taskSQL := physical(taskTable.Table.ID)
	column := func(id string) string {
		t.Helper()
		return f.scan("SELECT pg_column FROM meta.fields WHERE id=$1", id)
	}
	seedStart := time.Now()
	err := queryperf.SeedLinkedQuery(f.ctx, &authDatabase, f.pool, queryperf.LinkedQuerySeed{
		ProjectSchema: projectSchema, ProjectTable: projectName, ProjectLabel: column(projectLabel.ID),
		TaskSchema: taskSchema, TaskTable: taskName, TaskLabel: column(taskLabel.ID),
		Status: column(status.ID), Score: column(score.ID), ProjectFK: fk, Rows: rows, Projects: projects,
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("HTTP fixture: %d tasks, %d projects, one persisted FK per task; seed=%s",
		rows, projects, time.Since(seedStart))

	threshold := rows - rows/20
	config := structure.ViewColumns{
		Fields: []structure.ViewColumn{
			{ID: taskLabel.ID, Visible: true}, {ID: status.ID, Visible: true},
			{ID: score.ID, Visible: true}, {ID: relation.Relation.ChildFieldID, Visible: true},
			{ID: defaultTaskLabel, Visible: false},
		},
		Sort:       []structure.ViewSort{{Field: score.ID, Dir: "asc"}},
		GroupBy:    []string{status.ID},
		Aggregates: []structure.ViewAggregate{{FieldID: score.ID, Aggregate: codec.AggSum}},
		Filter:     json.RawMessage(fmt.Sprintf(`{"field":%q,"cmp":"gte","value":%d}`, score.ID, threshold)),
	}
	payload, err := json.Marshal(struct {
		Config structure.ViewColumns `json:"config"`
	}{Config: config})
	if err != nil {
		t.Fatal(err)
	}
	viewID := taskTable.Views[0].ID
	version := f.scan("SELECT version::text FROM meta.views WHERE id=$1", viewID)
	f.expectStatus(f.fieldCall(owner, "PATCH", "/views/"+viewID, string(payload), version), http.StatusOK, "")
	p := &httpQueryPerfFixture{
		f: f, owner: owner, viewID: viewID, tableName: taskSQL,
		label: taskLabel, status: status, score: score, linkID: relation.Relation.ChildFieldID,
		rows: rows, projects: projects, threshold: threshold,
	}
	if rows > 10_000 {
		p.waitForIndexes(t)
	}
	f.exec("ANALYZE " + projectSQL)
	f.exec("ANALYZE " + taskSQL)
	return p
}

func (p *httpQueryPerfFixture) waitForIndexes(t *testing.T) {
	t.Helper()
	service, err := structure.New(p.f.pool, p.f.resolver)
	if err != nil {
		t.Fatal(err)
	}
	var logs bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&logs, nil))
	ctx, cancel := context.WithCancel(p.f.ctx)
	done := make(chan struct{})
	go func() {
		service.RunAutoIndexWorker(ctx, logger)
		close(done)
	}()
	start := time.Now()
	deadline := time.NewTimer(45 * time.Second)
	defer deadline.Stop()
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()
	var ready bool
	var scanErr error
	for !ready && scanErr == nil {
		var labelTrigram, statusTrigram, statusBtree, scoreBtree bool
		scanErr = p.f.pool.QueryRow(p.f.ctx, `SELECT
			COALESCE((SELECT options->'indexed'->>'trigram' = 'true' FROM meta.fields WHERE id=$1),false),
			COALESCE((SELECT options->'indexed'->>'trigram' = 'true' FROM meta.fields WHERE id=$2),false),
			COALESCE((SELECT options->'indexed'->>'btree' = 'true' FROM meta.fields WHERE id=$2),false),
			COALESCE((SELECT options->'indexed'->>'btree' = 'true' FROM meta.fields WHERE id=$3),false)`,
			p.label.ID, p.status.ID, p.score.ID).Scan(&labelTrigram, &statusTrigram, &statusBtree, &scoreBtree)
		ready = labelTrigram && statusTrigram && statusBtree && scoreBtree
		if ready || scanErr != nil {
			break
		}
		select {
		case <-deadline.C:
			scanErr = fmt.Errorf("shared-view indexes not ready within 45s")
		case <-p.f.ctx.Done():
			scanErr = p.f.ctx.Err()
		case <-ticker.C:
		}
	}
	cancel()
	<-done
	if scanErr != nil || logs.Len() != 0 {
		t.Fatalf("shared-view index worker: %v; logs=%s", scanErr, logs.String())
	}
	t.Logf("HTTP fixture shared-view index scan=%s", time.Since(start))
}

func (p *httpQueryPerfFixture) server(t *testing.T) (*httptest.Server, *queryperf.QueryTracer) {
	t.Helper()
	tracer := &queryperf.QueryTracer{}
	config := p.f.pool.Config()
	config.ConnConfig.Tracer = tracer
	pool, err := pgxpool.NewWithConfig(p.f.ctx, config)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	if err := pool.Ping(p.f.ctx); err != nil {
		t.Fatal(err)
	}
	wrapped := &db.Pool{Pool: pool}
	resolver, err := acl.NewResolver(wrapped)
	if err != nil {
		t.Fatal(err)
	}
	authService, err := auth.New(wrapped)
	if err != nil {
		t.Fatal(err)
	}
	structureService, err := structure.New(wrapped, resolver)
	if err != nil {
		t.Fatal(err)
	}
	handler, err := New(slog.New(slog.NewJSONHandler(io.Discard, nil)), testAssets(), AuthOptions{
		Service: authService, Structure: structureService, BaseURL: "https://table.example",
	})
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	return server, tracer
}

func (p *httpQueryPerfFixture) request(t *testing.T, server *httptest.Server, name string) *http.Request {
	t.Helper()
	limit, offset, includeCount := 100, 0, true
	fields := []string{p.label.ID, p.status.ID, p.score.ID, p.linkID}
	noSort := []structure.RowSort{}
	var input any
	endpoint := "query"
	switch name {
	case "filter":
		input = structure.RowQueryRequest{
			Offset: &offset, Limit: &limit, IncludeCount: &includeCount, FieldIDs: &fields, Sorts: &noSort,
			Filter: &structure.FilterNode{Field: p.score.ID, Cmp: codec.OpGte,
				Value: json.RawMessage(strconv.Itoa(p.threshold))},
		}
	case "sort":
		input = structure.RowQueryRequest{
			Offset: &offset, Limit: &limit, IncludeCount: &includeCount, FieldIDs: &fields,
			Sort: &structure.RowSort{FieldID: p.score.ID, Direction: "desc"},
		}
	case "group":
		endpoint = "groups"
		input = structure.RowGroupRequest{Path: []json.RawMessage{}}
	default:
		t.Fatalf("unknown HTTP performance case %q", name)
	}
	body, err := json.Marshal(input)
	if err != nil {
		t.Fatal(err)
	}
	request, err := http.NewRequestWithContext(p.f.ctx, http.MethodPost,
		server.URL+"/api/v1/views/"+p.viewID+"/"+endpoint, bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	p.f.decorate(request, p.owner)
	return request
}

func (p *httpQueryPerfFixture) call(t *testing.T, server *httptest.Server, tracer *queryperf.QueryTracer,
	name string,
) ([]byte, time.Duration, []queryperf.TracedQuery) {
	t.Helper()
	request := p.request(t, server, name)
	tracer.Start()
	start := time.Now()
	response, err := server.Client().Do(request)
	if err != nil {
		tracer.Stop()
		t.Fatalf("%s HTTP request: %v", name, err)
	}
	body, readErr := io.ReadAll(io.LimitReader(response.Body, (4<<20)+1))
	elapsed := time.Since(start)
	queries := tracer.Stop()
	closeErr := response.Body.Close()
	if readErr != nil || closeErr != nil || len(body) > 4<<20 {
		t.Fatalf("%s HTTP body: read=%v close=%v bytes=%d", name, readErr, closeErr, len(body))
	}
	if response.StatusCode != http.StatusOK {
		t.Fatalf("%s HTTP status=%d body=%s", name, response.StatusCode, body)
	}
	return body, elapsed, queries
}

func (p *httpQueryPerfFixture) verify(t *testing.T, name string, body []byte) {
	t.Helper()
	type groupExpectation struct {
		count, sum int64
	}
	groups := make(map[string]groupExpectation)
	filtered := 0
	for id := 1; id <= p.rows; id++ {
		key := queryperf.LinkedQueryStatus(id)
		group := groups[key]
		group.count++
		if score, valid := queryperf.LinkedQueryScore(id, p.rows); valid {
			group.sum += score
			if score >= int64(p.threshold) {
				filtered++
			}
		}
		groups[key] = group
	}
	if name == "group" {
		var result structure.RowGroupResult
		if err := json.Unmarshal(body, &result); err != nil || len(result.Groups) != len(groups) {
			t.Fatalf("HTTP group response: %s (%v)", body, err)
		}
		for _, bucket := range result.Groups {
			var key *string
			if err := json.Unmarshal(bucket.Key, &key); err != nil {
				t.Fatal(err)
			}
			value := ""
			if key != nil {
				value = *key
			}
			want, ok := groups[value]
			if !ok || bucket.Count != want.count || len(bucket.Aggregates) != 1 ||
				bucket.Aggregates[0].FieldID != p.score.ID || bucket.Aggregates[0].Aggregate != codec.AggSum ||
				string(bucket.Aggregates[0].Value) != strconv.FormatInt(want.sum, 10) {
				t.Fatalf("unexpected HTTP group: %+v, want %+v", bucket, want)
			}
			delete(groups, value)
		}
		if len(groups) != 0 {
			t.Fatalf("missing HTTP groups: %v", groups)
		}
		return
	}
	var result structure.RowQueryResult
	if err := json.Unmarshal(body, &result); err != nil {
		t.Fatalf("decode HTTP query response: %v", err)
	}
	total := p.rows
	if name == "filter" {
		total = filtered
	}
	if result.Total == nil || *result.Total != int64(total) || len(result.Rows) != min(100, total) {
		t.Fatalf("%s HTTP total=%v rows=%d, want %d/%d", name, result.Total, len(result.Rows), total, min(100, total))
	}
	var previousID int
	var previousScore *int64
	for index, row := range result.Rows {
		id, err := strconv.Atoi(row.ID)
		if err != nil || id < 1 || id > p.rows {
			t.Fatalf("invalid HTTP row ID %q: %v", row.ID, err)
		}
		var score *int64
		if err := json.Unmarshal(row.Fields[p.score.ID], &score); err != nil {
			t.Fatal(err)
		}
		expected, valid := queryperf.LinkedQueryScore(id, p.rows)
		if (score != nil) != valid || score != nil && *score != expected {
			t.Fatalf("unexpected HTTP score for row %d: %s", id, row.Fields[p.score.ID])
		}
		if name == "filter" && (score == nil || *score < int64(p.threshold) || index > 0 && id <= previousID) {
			t.Fatalf("unexpected HTTP filter row/order: %+v", row)
		}
		if name == "sort" && index > 0 &&
			(previousScore == nil && score != nil || previousScore != nil && score != nil &&
				(*score > *previousScore || *score == *previousScore && id <= previousID)) {
			t.Fatalf("unexpected HTTP sort row/order: %+v", row)
		}
		projectID := queryperf.LinkedQueryProjectID(id, p.projects)
		var links []structure.LinkValue
		if err := json.Unmarshal(row.Fields[p.linkID], &links); err != nil || len(links) != 1 ||
			links[0].ID != strconv.Itoa(projectID) || links[0].Label != fmt.Sprintf("Project %04d", projectID) {
			t.Fatalf("HTTP row %d links=%s, want project %d: %v", id, row.Fields[p.linkID], projectID, err)
		}
		previousID, previousScore = id, score
	}
}

func TestLinkedQueryHTTPFixture(t *testing.T) {
	p := newHTTPQueryPerfFixture(t, 40, 5)
	server, tracer := p.server(t)
	for _, name := range []string{"filter", "sort", "group"} {
		t.Run(name, func(t *testing.T) {
			body, _, queries := p.call(t, server, tracer, name)
			p.verify(t, name, body)
			statements, err := queryperf.LinkedQueryStatements(p.tableName, name == "group", queries)
			if err != nil {
				t.Fatal(err)
			}
			for _, statement := range statements {
				if _, err := queryperf.ExplainQuery(p.f.ctx, p.f.pool, statement); err != nil {
					t.Fatal(err)
				}
			}
		})
	}
}

func TestLinkedQueryHTTPPerformance200k(t *testing.T) {
	if os.Getenv("PT_RUN_QUERY_PERF") != "1" {
		t.Skip("set PT_RUN_QUERY_PERF=1 to measure the 200k linked-row PG18 HTTP responses")
	}
	p := newHTTPQueryPerfFixture(t, httpQueryPerfRows, httpQueryPerfProjects)
	server, tracer := p.server(t)
	t.Logf("conditions: seeded/analyzed caches not evicted; first and repeated authenticated loopback HTTP responses including body read; filter score >= %d, no sort, page 0/100, exact count; sort score DESC NULLS LAST, page 0/100, exact count; group status with sum(score), no filter; linked Project field visible",
		p.threshold)
	captured := make(map[string]map[string]queryperf.TracedQuery)
	for _, name := range []string{"filter", "sort", "group"} {
		for _, sample := range []string{"first", "warm"} {
			body, elapsed, queries := p.call(t, server, tracer, name)
			p.verify(t, name, body)
			t.Logf("%s %s HTTP response elapsed=%s, target <1s: %t", name, sample, elapsed, elapsed < time.Second)
			if elapsed >= time.Second {
				t.Errorf("%s %s exceeded the strict <1s comparison", name, sample)
			}
			if sample == "warm" {
				var err error
				captured[name], err = queryperf.LinkedQueryStatements(p.tableName, name == "group", queries)
				if err != nil {
					t.Fatal(err)
				}
			}
		}
	}
	for _, name := range []string{"filter", "sort", "group"} {
		for _, kind := range []string{"page", "count", "links", "group"} {
			query, ok := captured[name][kind]
			if !ok {
				continue
			}
			plan, err := queryperf.ExplainQuery(p.f.ctx, p.f.pool, query)
			if err != nil {
				t.Fatal(err)
			}
			t.Logf("%s %s captured SQL=%s\nbound args=%#v\nEXPLAIN (ANALYZE, BUFFERS):\n%s",
				name, kind, query.SQL, query.BoundArgs(), plan)
		}
	}
}
