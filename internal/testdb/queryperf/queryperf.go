package queryperf

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"

	"github.com/jackc/pgx/v5"

	"pandatable/internal/db"
	"pandatable/internal/ddl"
	"pandatable/internal/testdb"
)

type LinkedQuerySeed struct {
	ProjectSchema string
	ProjectTable  string
	ProjectLabel  string
	TaskSchema    string
	TaskTable     string
	TaskLabel     string
	Status        string
	Score         string
	ProjectFK     string
	Rows          int
	Projects      int
}

func LinkedQueryStatus(id int) string {
	switch {
	case id%10 == 0:
		return ""
	case id%4 == 0:
		return "Blocked"
	case id%4 == 1:
		return "Open"
	case id%4 == 2:
		return "Done"
	default:
		return "Review"
	}
}

func LinkedQueryScore(id, rows int) (int64, bool) {
	if id%20 == 0 {
		return 0, false
	}
	return int64(id * 37 % rows), true
}

func LinkedQueryProjectID(id, projects int) int {
	return (id-1)%projects + 1
}

func SeedLinkedQuery(ctx context.Context, suite *testdb.Suite, pool *db.Pool, seed LinkedQuerySeed) error {
	if err := suite.CheckPoolOwnership(ctx, pool); err != nil {
		return fmt.Errorf("linked-query seed: %w", err)
	}
	if seed.Rows < 20 || seed.Rows > 200_000 ||
		seed.Projects < 1 || seed.Projects > 1_000 || seed.Projects > seed.Rows {
		return errors.New("invalid linked-query fixture size")
	}
	for _, identifier := range []struct {
		kind  ddl.ObjectKind
		value string
	}{
		{ddl.Book, seed.ProjectSchema}, {ddl.Table, seed.ProjectTable}, {ddl.Column, seed.ProjectLabel},
		{ddl.Book, seed.TaskSchema}, {ddl.Table, seed.TaskTable}, {ddl.Column, seed.TaskLabel},
		{ddl.Column, seed.Status}, {ddl.Column, seed.Score}, {ddl.Column, seed.ProjectFK},
	} {
		if err := ddl.ValidateIdentifier(identifier.kind, identifier.value); err != nil {
			return fmt.Errorf("validate linked-query seed identifier: %w", err)
		}
	}
	projectTable := pgx.Identifier{seed.ProjectSchema, seed.ProjectTable}.Sanitize()
	taskTable := pgx.Identifier{seed.TaskSchema, seed.TaskTable}.Sanitize()
	projectLabel := pgx.Identifier{seed.ProjectLabel}.Sanitize()
	taskLabel := pgx.Identifier{seed.TaskLabel}.Sanitize()
	status := pgx.Identifier{seed.Status}.Sanitize()
	score := pgx.Identifier{seed.Score}.Sanitize()
	fk := pgx.Identifier{seed.ProjectFK}.Sanitize()
	if _, err := pool.Exec(ctx, fmt.Sprintf(`INSERT INTO %s (%s)
		SELECT 'Project ' || lpad(value::text, 4, '0') FROM generate_series(1, $1) AS value`,
		projectTable, projectLabel), seed.Projects); err != nil {
		return fmt.Errorf("seed linked-query projects: %w", err)
	}
	if _, err := pool.Exec(ctx, fmt.Sprintf(`INSERT INTO %s (%s, %s, %s, %s)
		SELECT 'Task ' || lpad(value::text, 6, '0'),
			CASE WHEN value %% 10 = 0 THEN NULL WHEN value %% 4 = 0 THEN 'Blocked'
				WHEN value %% 4 = 1 THEN 'Open' WHEN value %% 4 = 2 THEN 'Done' ELSE 'Review' END,
			CASE WHEN value %% 20 = 0 THEN NULL ELSE (value * 37) %% $2 END,
			((value - 1) %% $3) + 1
		FROM generate_series(1, $1) AS value`, taskTable, taskLabel, status, score, fk),
		seed.Rows, seed.Rows, seed.Projects); err != nil {
		return fmt.Errorf("seed linked-query tasks: %w", err)
	}
	return nil
}

type TracedQuery struct {
	SQL  string
	Args []any
}

func (query TracedQuery) BoundArgs() []any {
	if len(query.Args) > 0 {
		if _, ok := query.Args[0].(pgx.QueryExecMode); ok {
			return query.Args[1:]
		}
	}
	return query.Args
}

type QueryTracer struct {
	mu      sync.Mutex
	active  bool
	queries []TracedQuery
}

func (tracer *QueryTracer) TraceQueryStart(ctx context.Context, _ *pgx.Conn, data pgx.TraceQueryStartData) context.Context {
	tracer.mu.Lock()
	defer tracer.mu.Unlock()
	if tracer.active {
		tracer.queries = append(tracer.queries, TracedQuery{SQL: data.SQL, Args: append([]any(nil), data.Args...)})
	}
	return ctx
}

func (*QueryTracer) TraceQueryEnd(context.Context, *pgx.Conn, pgx.TraceQueryEndData) {}

func (tracer *QueryTracer) Start() {
	tracer.mu.Lock()
	defer tracer.mu.Unlock()
	tracer.queries = nil
	tracer.active = true
}

func (tracer *QueryTracer) Stop() []TracedQuery {
	tracer.mu.Lock()
	defer tracer.mu.Unlock()
	tracer.active = false
	return append([]TracedQuery(nil), tracer.queries...)
}

func LinkedQueryStatements(table string, grouped bool, queries []TracedQuery) (map[string]TracedQuery, error) {
	found := make(map[string]TracedQuery)
	for _, query := range queries {
		if !strings.Contains(query.SQL, " FROM "+table+` AS "source"`) {
			continue
		}
		kind := ""
		switch {
		case strings.HasPrefix(query.SQL, "SELECT count(*) FROM "):
			kind = "count"
		case strings.Contains(query.SQL, ` GROUP BY "key"`):
			kind = "group"
		case strings.Contains(query.SQL, " JOIN "):
			kind = "links"
		case strings.Contains(query.SQL, " ORDER BY "):
			kind = "page"
		}
		if kind != "" {
			if _, duplicate := found[kind]; duplicate {
				return nil, fmt.Errorf("multiple %s statements in linked-query trace", kind)
			}
			found[kind] = query
		}
	}
	expected := []string{"page", "count", "links"}
	if grouped {
		expected = []string{"group"}
	}
	for _, kind := range expected {
		if _, ok := found[kind]; !ok {
			return nil, fmt.Errorf("missing %s statement in linked-query trace", kind)
		}
	}
	if len(found) != len(expected) {
		return nil, errors.New("linked-query trace contains unexpected data statements")
	}
	return found, nil
}

func ExplainQuery(ctx context.Context, pool *db.Pool, query TracedQuery) (string, error) {
	if !strings.HasPrefix(strings.ToUpper(strings.TrimSpace(query.SQL)), "SELECT ") {
		return "", errors.New("EXPLAIN ANALYZE is restricted to captured SELECT statements")
	}
	rows, err := pool.Query(ctx, "EXPLAIN (ANALYZE, BUFFERS) "+query.SQL, query.Args...)
	if err != nil {
		return "", fmt.Errorf("explain captured statement: %w", err)
	}
	defer rows.Close()
	var lines []string
	for rows.Next() {
		var line string
		if err := rows.Scan(&line); err != nil {
			return "", fmt.Errorf("scan captured plan: %w", err)
		}
		lines = append(lines, line)
	}
	if err := rows.Err(); err != nil {
		return "", fmt.Errorf("read captured plan: %w", err)
	}
	plan := strings.Join(lines, "\n")
	if !strings.Contains(plan, "Buffers:") || !strings.Contains(plan, "Execution Time:") {
		return "", errors.New("EXPLAIN did not return analyzed buffer evidence")
	}
	return plan, nil
}
