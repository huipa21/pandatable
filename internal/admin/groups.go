package admin

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"

	"pandatable/internal/acl"
	"pandatable/internal/admin/adminsql"
	"pandatable/internal/apperr"
	"pandatable/internal/auth"
)

type Group struct {
	ID          string    `json:"id"`
	Name        string    `json:"name"`
	Description string    `json:"description"`
	CreatedAt   time.Time `json:"createdAt"`
	MemberCount int64     `json:"memberCount"`
}

type GroupListing struct {
	Items []Group `json:"items"`
	Total int64   `json:"total"`
}

type GroupPatch struct {
	Name, Description *string
}

func groupDTO(row adminsql.GetGroupRow) Group {
	return Group{ID: row.ID.String(), Name: row.Name, Description: row.Description,
		CreatedAt: row.CreatedAt.Time.UTC(), MemberCount: row.MemberCount}
}

func groupValidation() error {
	return apperr.New(400, "ERR_VALIDATION", "Invalid group request")
}

func groupNotFound() error {
	return apperr.New(404, "ERR_NOT_FOUND", "Group not found")
}

func groupID(value string) (pgtype.UUID, error) {
	id, ok := acl.ParseUUID(value)
	if !ok {
		return pgtype.UUID{}, groupNotFound()
	}
	return id, nil
}

func normalizeGroup(name, description string) (string, error) {
	name = strings.TrimSpace(name)
	if !utf8.ValidString(name) || !utf8.ValidString(description) || name == "" ||
		utf8.RuneCountInString(name) > 100 || utf8.RuneCountInString(description) > 1000 ||
		strings.ContainsFunc(name, unicode.IsControl) || strings.ContainsFunc(description, func(value rune) bool {
		return unicode.IsControl(value) && value != '\n' && value != '\r' && value != '\t'
	}) {
		return "", groupValidation()
	}
	return name, nil
}

func groupWriteError(err error) error {
	var postgres *pgconn.PgError
	if errors.As(err, &postgres) && postgres.Code == "23505" {
		return apperr.New(409, "ERR_GROUP_NAME_CONFLICT", "Group name is already in use")
	}
	return err
}

func groupListInput(input ListInput) (ListInput, error) {
	input.Q = strings.ToLower(strings.TrimSpace(input.Q))
	if input.Status != "" || input.Offset < 0 || input.Limit < 1 || input.Limit > 500 ||
		len(input.Q) > 254 || !utf8.ValidString(input.Q) || strings.ContainsFunc(input.Q, unicode.IsControl) {
		return input, groupValidation()
	}
	return input, nil
}

func (service *Service) groupMutation(ctx context.Context, token string, target pgtype.UUID,
	fn func(*adminsql.Queries, *bool) error) error {
	return service.adminMutation(ctx, token, target, true, fn)
}

func (service *Service) groupSnapshot(ctx context.Context, token string, fn func(*adminsql.Queries) error) error {
	hash, err := auth.SessionHash(token)
	if err != nil {
		return err
	}
	tx, err := service.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly})
	if err != nil {
		return fmt.Errorf("begin group snapshot: %w", err)
	}
	defer func() {
		cleanup, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
		defer cancel()
		_ = tx.Rollback(cleanup)
	}()
	queries := service.queries.WithTx(tx)
	if _, err := authorize(ctx, queries, hash); err != nil {
		return err
	}
	if err := fn(queries); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (service *Service) Groups(ctx context.Context, token, userID string, input ListInput) (GroupListing, error) {
	input, err := groupListInput(input)
	if err != nil {
		return GroupListing{}, err
	}
	var user pgtype.UUID
	if userID != "" {
		if user, err = groupID(userID); err != nil {
			return GroupListing{}, err
		}
	}
	result := GroupListing{Items: []Group{}}
	err = service.groupSnapshot(ctx, token, func(queries *adminsql.Queries) error {
		if user.Valid {
			if _, err := queries.GetUser(ctx, user); errors.Is(err, pgx.ErrNoRows) {
				return apperr.New(404, "ERR_NOT_FOUND", "User not found")
			} else if err != nil {
				return err
			}
		}
		var err error
		result.Total, err = queries.CountGroups(ctx, adminsql.CountGroupsParams{Q: input.Q, UserID: user})
		if err != nil {
			return err
		}
		rows, err := queries.ListGroups(ctx, adminsql.ListGroupsParams{Q: input.Q, UserID: user,
			PageLimit: input.Limit, PageOffset: input.Offset})
		for _, row := range rows {
			result.Items = append(result.Items, groupDTO(adminsql.GetGroupRow(row)))
		}
		return err
	})
	return result, err
}

func (service *Service) Group(ctx context.Context, token, id string) (Group, error) {
	target, err := groupID(id)
	if err != nil {
		return Group{}, err
	}
	var result Group
	err = service.groupSnapshot(ctx, token, func(queries *adminsql.Queries) error {
		row, err := queries.GetGroup(ctx, target)
		if errors.Is(err, pgx.ErrNoRows) {
			return groupNotFound()
		}
		result = groupDTO(row)
		return err
	})
	return result, err
}

func (service *Service) ManagedUser(ctx context.Context, token, id string) (User, error) {
	target, err := groupID(id)
	if err != nil {
		return User{}, err
	}
	var result User
	err = service.groupSnapshot(ctx, token, func(queries *adminsql.Queries) error {
		row, err := queries.GetUser(ctx, target)
		if errors.Is(err, pgx.ErrNoRows) {
			return apperr.New(404, "ERR_NOT_FOUND", "User not found")
		}
		result = userDTO(row)
		return err
	})
	return result, err
}

func (service *Service) CreateGroup(ctx context.Context, token, name, description string) (Group, error) {
	name, err := normalizeGroup(name, description)
	if err != nil {
		return Group{}, err
	}
	var result Group
	err = service.groupMutation(ctx, token, pgtype.UUID{}, func(queries *adminsql.Queries, _ *bool) error {
		id, err := queries.CreateGroup(ctx, adminsql.CreateGroupParams{Name: name, Description: description})
		if err != nil {
			return groupWriteError(err)
		}
		row, err := queries.GetGroup(ctx, id)
		result = groupDTO(row)
		return err
	})
	return result, err
}

func (service *Service) UpdateGroup(ctx context.Context, token, id string, patch GroupPatch) (Group, error) {
	target, err := groupID(id)
	if err != nil {
		return Group{}, err
	}
	if patch.Name == nil && patch.Description == nil {
		return Group{}, groupValidation()
	}
	var result Group
	err = service.groupMutation(ctx, token, pgtype.UUID{}, func(queries *adminsql.Queries, _ *bool) error {
		if _, err := queries.LockGroup(ctx, target); errors.Is(err, pgx.ErrNoRows) {
			return groupNotFound()
		} else if err != nil {
			return err
		}
		row, err := queries.GetGroup(ctx, target)
		if err != nil {
			return err
		}
		if patch.Name != nil {
			row.Name = *patch.Name
		}
		if patch.Description != nil {
			row.Description = *patch.Description
		}
		name, err := normalizeGroup(row.Name, row.Description)
		if err != nil {
			return err
		}
		if err := queries.UpdateGroup(ctx, adminsql.UpdateGroupParams{ID: target, Name: name, Description: row.Description}); err != nil {
			return groupWriteError(err)
		}
		row.Name = name
		result = groupDTO(row)
		return nil
	})
	return result, err
}

func (service *Service) DeleteGroup(ctx context.Context, token, id string) error {
	target, err := groupID(id)
	if err != nil {
		return err
	}
	return service.groupMutation(ctx, token, pgtype.UUID{}, func(queries *adminsql.Queries, invalidate *bool) error {
		if _, err := queries.DeleteGroup(ctx, target); errors.Is(err, pgx.ErrNoRows) {
			return groupNotFound()
		} else if err != nil {
			return err
		}
		*invalidate = true
		return queries.DeleteGroupGrants(ctx, target)
	})
}

func (service *Service) Members(ctx context.Context, token, id string, input ListInput) (Listing, error) {
	target, err := groupID(id)
	if err != nil {
		return Listing{}, err
	}
	input, err = groupListInput(input)
	if err != nil {
		return Listing{}, err
	}
	result := Listing{Items: []User{}}
	err = service.groupSnapshot(ctx, token, func(queries *adminsql.Queries) error {
		if _, err := queries.GetGroup(ctx, target); errors.Is(err, pgx.ErrNoRows) {
			return groupNotFound()
		} else if err != nil {
			return err
		}
		var err error
		result.Total, err = queries.CountGroupMembers(ctx, adminsql.CountGroupMembersParams{GroupID: target, Q: input.Q})
		if err != nil {
			return err
		}
		rows, err := queries.ListGroupMembers(ctx, adminsql.ListGroupMembersParams{GroupID: target, Q: input.Q,
			PageLimit: input.Limit, PageOffset: input.Offset})
		for _, row := range rows {
			result.Items = append(result.Items, userDTO(adminsql.GetUserRow(row)))
		}
		return err
	})
	return result, err
}

func (service *Service) ChangeMember(ctx context.Context, token, id, userID string, add bool) error {
	group, err := groupID(id)
	if err != nil {
		return err
	}
	user, ok := acl.ParseUUID(userID)
	if !ok {
		return apperr.New(404, "ERR_NOT_FOUND", "User not found")
	}
	return service.groupMutation(ctx, token, user, func(queries *adminsql.Queries, invalidate *bool) error {
		if _, err := queries.LockMembershipGroup(ctx, group); errors.Is(err, pgx.ErrNoRows) {
			return groupNotFound()
		} else if err != nil {
			return err
		}
		var changed int64
		var err error
		if add {
			row, err := queries.GetUser(ctx, user)
			if err != nil {
				return err
			}
			if row.Status != "active" {
				return apperr.New(409, "ERR_PRINCIPAL_DISABLED", "User is disabled")
			}
			changed, err = queries.AddGroupMember(ctx, adminsql.AddGroupMemberParams{GroupID: group, UserID: user})
			if err == nil && changed == 0 {
				return apperr.New(409, "ERR_MEMBERSHIP_EXISTS", "User is already a member")
			}
		} else {
			changed, err = queries.RemoveGroupMember(ctx, adminsql.RemoveGroupMemberParams{GroupID: group, UserID: user})
		}
		*invalidate = changed > 0
		return err
	})
}
