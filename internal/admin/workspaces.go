package admin

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgtype"

	"pandatable/internal/admin/adminsql"
)

type WorkspaceOwner struct {
	ID       string  `json:"id"`
	Type     string  `json:"type"`
	Name     string  `json:"name"`
	Username *string `json:"username"`
	Status   *string `json:"status"`
}

type Workspace struct {
	ID              string           `json:"id"`
	Name            string           `json:"name"`
	Owners          []WorkspaceOwner `json:"owners"`
	OwnerTotal      int64            `json:"ownerTotal"`
	Orphaned        bool             `json:"orphaned"`
	AttachmentBytes int64            `json:"attachmentBytes"`
}

type WorkspaceListing struct {
	Items []Workspace `json:"items"`
	Total int64       `json:"total"`
}

func (service *Service) Workspaces(ctx context.Context, token string, input ListInput) (WorkspaceListing, error) {
	input, err := groupListInput(input)
	if err != nil {
		return WorkspaceListing{}, err
	}
	result := WorkspaceListing{Items: []Workspace{}}
	err = service.groupSnapshot(ctx, token, func(queries *adminsql.Queries) error {
		var err error
		result.Total, err = queries.CountWorkspaces(ctx, input.Q)
		if err != nil {
			return fmt.Errorf("count workspaces: %w", err)
		}
		rows, err := queries.ListWorkspaces(ctx, adminsql.ListWorkspacesParams{Q: input.Q,
			PageOffset: input.Offset, PageLimit: input.Limit})
		if err != nil {
			return fmt.Errorf("list workspaces: %w", err)
		}
		ids := make([]pgtype.UUID, len(rows))
		positions := make(map[pgtype.UUID]int, len(rows))
		for index, row := range rows {
			ids[index], positions[row.ID] = row.ID, index
			result.Items = append(result.Items, Workspace{ID: row.ID.String(), Name: row.Name,
				Owners: []WorkspaceOwner{}, OwnerTotal: row.OwnerTotal,
				Orphaned: row.Orphaned, AttachmentBytes: row.AttachmentBytes})
		}
		owners, err := queries.ListWorkspaceOwners(ctx, ids)
		if err != nil {
			return fmt.Errorf("list workspace owners: %w", err)
		}
		for _, owner := range owners {
			var username, status *string
			if owner.Username.Valid {
				username = &owner.Username.String
			}
			if owner.Status.Valid {
				status = &owner.Status.String
			}
			index := positions[owner.ResourceID]
			result.Items[index].Owners = append(result.Items[index].Owners, WorkspaceOwner{
				ID: owner.PrincipalID.String(), Type: owner.PrincipalType, Name: owner.Name, Username: username, Status: status})
		}
		return nil
	})
	return result, err
}
