package structure

import (
	"context"
	"fmt"
	"sort"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"pandatable/internal/acl"
	"pandatable/internal/structure/structuresql"
)

type NavView struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	Type  string `json:"type"`
	Scope string `json:"scope"`
}

type NavTable struct {
	ID            string    `json:"id"`
	Name          string    `json:"name"`
	Position      float64   `json:"position"`
	SchemaVersion int32     `json:"schemaVersion"`
	Level         string    `json:"level"`
	Views         []NavView `json:"views"`
}

type NavBook struct {
	ID           string     `json:"id"`
	Name         string     `json:"name"`
	Icon         *string    `json:"icon"`
	FallbackIcon string     `json:"fallbackIcon"`
	Position     float64    `json:"position"`
	Level        string     `json:"level"`
	PathOnly     bool       `json:"pathOnly"`
	Tables       []NavTable `json:"tables"`
}

type NavWorkspace struct {
	ID           string    `json:"id"`
	Name         string    `json:"name"`
	Icon         *string   `json:"icon"`
	Color        *string   `json:"color"`
	Description  string    `json:"description"`
	FallbackIcon string    `json:"fallbackIcon"`
	CreatedAt    time.Time `json:"createdAt"`
	Level        string    `json:"level"`
	PathOnly     bool      `json:"pathOnly"`
	Books        []NavBook `json:"books"`
}

// Nav is the sidebar tree: only live resources the caller can reach, each with
// its effective level. "none"/pathOnly workspaces and books exist solely to hold
// visible descendants and never carry sibling content (06-permissions). Tables
// are visible only with an effective level, so they have no path-only form.
type Nav struct {
	Workspaces []NavWorkspace `json:"workspaces"`
}

func levelName(level int32) string { return acl.Level(level).String() }

// Nav returns the permitted tree from one repeatable-read snapshot. The admin
// role has no special case: it sees exactly what its grants allow. Levels are
// the maximum of the grants on the path (raise-only), like the resolver.
func (s *Service) Nav(ctx context.Context, actor Actor) (Nav, error) {
	user, ok := acl.ParseUUID(actor.ID)
	if !ok {
		return Nav{}, acl.ErrValidation()
	}
	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly})
	if err != nil {
		return Nav{}, fmt.Errorf("begin navigation snapshot: %w", err)
	}
	defer func() { _ = tx.Rollback(context.WithoutCancel(ctx)) }()
	q := s.queries.WithTx(tx)

	grants, err := q.NavGrants(ctx, user)
	if err != nil {
		return Nav{}, fmt.Errorf("read navigation grants: %w", err)
	}
	granted := make(map[acl.Resource]int32, len(grants))
	for _, grant := range grants {
		granted[acl.Resource{Type: acl.ResourceType(grant.ResourceType), ID: grant.ResourceID}] = grant.Level
	}
	result := Nav{Workspaces: []NavWorkspace{}}
	workspaceIDs, err := q.NavGrantedWorkspaces(ctx, user)
	if err != nil {
		return Nav{}, fmt.Errorf("read navigation workspaces: %w", err)
	}
	if len(workspaceIDs) == 0 {
		return result, nil
	}
	workspaces, err := q.NavWorkspaces(ctx, workspaceIDs)
	if err != nil {
		return Nav{}, fmt.Errorf("list navigation workspaces: %w", err)
	}
	books, err := q.NavBooks(ctx, workspaceIDs)
	if err != nil {
		return Nav{}, fmt.Errorf("list navigation books: %w", err)
	}
	tables, err := q.NavTables(ctx, workspaceIDs)
	if err != nil {
		return Nav{}, fmt.Errorf("list navigation tables: %w", err)
	}

	workspaceLevel := make(map[pgtype.UUID]int32, len(workspaces))
	for _, row := range workspaces {
		workspaceLevel[row.ID] = granted[acl.Resource{Type: acl.Workspace, ID: row.ID}]
	}
	bookLevel := make(map[pgtype.UUID]int32, len(books))
	for _, row := range books {
		if level, live := workspaceLevel[row.WorkspaceID]; live {
			bookLevel[row.ID] = max(level, granted[acl.Resource{Type: acl.Book, ID: row.ID}])
		}
	}
	tablesByBook := make(map[pgtype.UUID][]NavTable)
	var tableIDs []pgtype.UUID
	for _, row := range tables {
		parent, live := bookLevel[row.BookID]
		if !live {
			continue
		}
		level := max(parent, granted[acl.Resource{Type: acl.Table, ID: row.ID}])
		if level == 0 {
			continue
		}
		tableIDs = append(tableIDs, row.ID)
		tablesByBook[row.BookID] = append(tablesByBook[row.BookID], NavTable{ID: row.ID.String(), Name: row.Name,
			Position: row.Position, SchemaVersion: row.SchemaVersion, Level: levelName(level), Views: []NavView{}})
	}
	views := map[string][]NavView{}
	if len(tableIDs) > 0 {
		rows, err := q.NavViews(ctx, structuresql.NavViewsParams{TableIds: tableIDs, UserID: user})
		if err != nil {
			return Nav{}, fmt.Errorf("list navigation views: %w", err)
		}
		for _, row := range rows {
			key := row.TableID.String()
			views[key] = append(views[key], NavView{ID: row.ID.String(), Name: row.Name, Type: row.Type, Scope: row.Scope})
		}
	}

	booksByWorkspace := make(map[pgtype.UUID][]NavBook)
	for _, row := range books {
		level, live := bookLevel[row.ID]
		visible := tablesByBook[row.ID]
		if !live || level == 0 && len(visible) == 0 {
			continue
		}
		sort.Slice(visible, func(i, j int) bool {
			return before(visible[i].Position, visible[j].Position, visible[i].ID, visible[j].ID)
		})
		for index := range visible {
			visible[index].Views = append(visible[index].Views, views[visible[index].ID]...)
		}
		if visible == nil {
			visible = []NavTable{}
		}
		booksByWorkspace[row.WorkspaceID] = append(booksByWorkspace[row.WorkspaceID], NavBook{ID: row.ID.String(),
			Name: row.Name, Icon: textPointer(row.Icon), FallbackIcon: row.FallbackIcon,
			Position: row.Position, Level: levelName(level), PathOnly: level == 0, Tables: visible})
	}
	for _, row := range workspaces {
		level := workspaceLevel[row.ID]
		visible := booksByWorkspace[row.ID]
		if level == 0 && len(visible) == 0 {
			continue
		}
		description := row.Description
		if level == 0 {
			description = ""
		}
		sort.Slice(visible, func(i, j int) bool {
			return before(visible[i].Position, visible[j].Position, visible[i].ID, visible[j].ID)
		})
		if visible == nil {
			visible = []NavBook{}
		}
		result.Workspaces = append(result.Workspaces, NavWorkspace{ID: row.ID.String(), Name: row.Name,
			Icon: textPointer(row.Icon), Color: textPointer(row.Color), Description: description,
			FallbackIcon: row.FallbackIcon, CreatedAt: timeOf(row.CreatedAt),
			Level: levelName(level), PathOnly: level == 0, Books: visible})
	}
	sort.Slice(result.Workspaces, func(i, j int) bool {
		left, right := result.Workspaces[i], result.Workspaces[j]
		if !left.CreatedAt.Equal(right.CreatedAt) {
			return left.CreatedAt.Before(right.CreatedAt)
		}
		return left.ID < right.ID
	})
	return result, nil
}

// before orders siblings by stored position, then by the canonical lowercase
// UUID text (time-ordered uuidv7), so equal positions are still deterministic.
func before(left, right float64, leftID, rightID string) bool {
	if left != right {
		return left < right
	}
	return leftID < rightID
}
