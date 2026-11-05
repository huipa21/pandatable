// Package workspace contains transaction-bound workspace creation shared by
// account provisioning and the structure service.
package workspace

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgtype"

	"pandatable/internal/structure/structuresql"
)

const StarterName = "Default"

// CreateOwned inserts the workspace and its creator's direct owner grant.
// The caller must pass queries bound to its transaction.
func CreateOwned(ctx context.Context, queries *structuresql.Queries, params structuresql.InsertWorkspaceParams) (structuresql.InsertWorkspaceRow, error) {
	row, err := queries.InsertWorkspace(ctx, params)
	if err != nil {
		return structuresql.InsertWorkspaceRow{}, fmt.Errorf("insert workspace: %w", err)
	}
	if err := queries.InsertWorkspaceOwnerGrant(ctx, structuresql.InsertWorkspaceOwnerGrantParams{
		WorkspaceID: row.ID, UserID: params.CreatedBy,
	}); err != nil {
		return structuresql.InsertWorkspaceRow{}, fmt.Errorf("insert workspace owner grant: %w", err)
	}
	return row, nil
}

// CreateStarter initializes a user who has no preference record yet. The
// caller serializes initialization and commits the user, workspace and grant.
func CreateStarter(ctx context.Context, queries *structuresql.Queries, user pgtype.UUID) (pgtype.UUID, error) {
	row, err := CreateOwned(ctx, queries, structuresql.InsertWorkspaceParams{Name: StarterName, CreatedBy: user})
	if err != nil {
		return pgtype.UUID{}, err
	}
	if err := queries.InsertWorkspacePreference(ctx, structuresql.InsertWorkspacePreferenceParams{
		UserID: user, WorkspaceID: row.ID,
	}); err != nil {
		return pgtype.UUID{}, fmt.Errorf("insert workspace preference: %w", err)
	}
	return row.ID, nil
}
