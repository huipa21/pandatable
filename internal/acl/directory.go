package acl

import (
	"context"
	"fmt"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"pandatable/internal/acl/aclsql"
)

type DirectoryPrincipal struct {
	ID       string  `json:"id"`
	Name     string  `json:"name"`
	Username *string `json:"username"`
}

type DirectoryListing struct {
	Items   []DirectoryPrincipal `json:"items"`
	HasMore bool                 `json:"hasMore"`
}

type UserLabel struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// Directory exposes minimal, bounded identity discovery only to scope owners.
// Authorization and identities share one snapshot; the resolver cache is not used.
func (service *Service) Directory(ctx context.Context, userID string, resource Resource, kind, search string) (DirectoryListing, error) {
	search = strings.TrimSpace(search)
	length := utf8.RuneCountInString(search)
	if (kind != "user" && kind != "group") || !utf8.ValidString(search) ||
		length < 2 || length > 100 || len(search) > 254 || strings.ContainsFunc(search, unicode.IsControl) {
		return DirectoryListing{}, ErrValidation()
	}
	user, err := parseCaller(userID)
	if err != nil {
		return DirectoryListing{}, err
	}
	tx, err := service.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly})
	if err != nil {
		return DirectoryListing{}, fmt.Errorf("begin share directory snapshot: %w", err)
	}
	defer func() { _ = tx.Rollback(context.WithoutCancel(ctx)) }()
	queries := service.queries.WithTx(tx)
	if _, err := requireOwner(ctx, queries, user, resource); err != nil {
		return DirectoryListing{}, err
	}
	result := DirectoryListing{Items: []DirectoryPrincipal{}}
	if kind == "user" {
		rows, err := queries.SearchShareUsers(ctx, search)
		if err != nil {
			return DirectoryListing{}, fmt.Errorf("search share users: %w", err)
		}
		for _, row := range rows {
			result.Items = append(result.Items, DirectoryPrincipal{ID: row.ID.String(), Name: row.Name, Username: &row.Username})
		}
	} else {
		rows, err := queries.SearchShareGroups(ctx, search)
		if err != nil {
			return DirectoryListing{}, fmt.Errorf("search share groups: %w", err)
		}
		for _, row := range rows {
			result.Items = append(result.Items, DirectoryPrincipal{ID: row.ID.String(), Name: row.Name})
		}
	}
	result.HasMore = len(result.Items) > 20
	if result.HasMore {
		result.Items = result.Items[:20]
	}
	return result, nil
}

func tableUserSearch(search string) (string, error) {
	search = strings.TrimSpace(search)
	length := utf8.RuneCountInString(search)
	if !utf8.ValidString(search) || length < 2 || length > 100 ||
		len(search) > 254 || strings.ContainsFunc(search, unicode.IsControl) {
		return "", ErrValidation()
	}
	return search, nil
}

// TableUsers exposes the bounded active-user picker only to table writers.
func (service *Service) TableUsers(ctx context.Context, userID, tableID, search string) (DirectoryListing, error) {
	search, err := tableUserSearch(search)
	if err != nil {
		return DirectoryListing{}, err
	}
	user, err := parseCaller(userID)
	if err != nil {
		return DirectoryListing{}, err
	}
	resource, err := NewResource(string(Table), tableID)
	if err != nil {
		return DirectoryListing{}, err
	}
	tx, err := service.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly})
	if err != nil {
		return DirectoryListing{}, fmt.Errorf("begin table user directory snapshot: %w", err)
	}
	defer func() { _ = tx.Rollback(context.WithoutCancel(ctx)) }()
	queries := service.queries.WithTx(tx)
	access, err := queryAccess(ctx, queries, user, resource)
	if err != nil {
		return DirectoryListing{}, err
	}
	if err := access.Check(Write); err != nil {
		return DirectoryListing{}, err
	}
	rows, err := queries.SearchShareUsers(ctx, search)
	if err != nil {
		return DirectoryListing{}, fmt.Errorf("search table users: %w", err)
	}
	result := DirectoryListing{Items: []DirectoryPrincipal{}}
	for _, row := range rows {
		result.Items = append(result.Items, DirectoryPrincipal{
			ID: row.ID.String(), Name: row.Name, Username: &row.Username,
		})
	}
	result.HasMore = len(result.Items) > 20
	if result.HasMore {
		result.Items = result.Items[:20]
	}
	return result, nil
}

// MentionUsers exposes only active users who can read the same live table.
func (service *Service) MentionUsers(ctx context.Context, userID, tableID, search string) (DirectoryListing, error) {
	search, err := tableUserSearch(search)
	if err != nil {
		return DirectoryListing{}, err
	}
	user, err := parseCaller(userID)
	if err != nil {
		return DirectoryListing{}, err
	}
	resource, err := NewResource(string(Table), tableID)
	if err != nil {
		return DirectoryListing{}, err
	}
	tx, err := service.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly})
	if err != nil {
		return DirectoryListing{}, fmt.Errorf("begin mention directory snapshot: %w", err)
	}
	defer func() { _ = tx.Rollback(context.WithoutCancel(ctx)) }()
	queries := service.queries.WithTx(tx)
	access, err := queryAccess(ctx, queries, user, resource)
	if err != nil {
		return DirectoryListing{}, err
	}
	if err := access.Check(Read); err != nil {
		return DirectoryListing{}, err
	}
	rows, err := queries.SearchMentionUsers(ctx, aclsql.SearchMentionUsersParams{
		TableID: resource.ID, Search: search,
	})
	if err != nil {
		return DirectoryListing{}, fmt.Errorf("search mention users: %w", err)
	}
	result := DirectoryListing{Items: make([]DirectoryPrincipal, 0, len(rows))}
	for _, row := range rows {
		result.Items = append(result.Items, DirectoryPrincipal{
			ID: row.ID.String(), Name: row.Name, Username: &row.Username,
		})
	}
	result.HasMore = len(result.Items) > 20
	if result.HasMore {
		result.Items = result.Items[:20]
	}
	return result, nil
}

// TableUserLabels resolves only caller-submitted IDs, avoiding user enumeration.
func (service *Service) TableUserLabels(ctx context.Context, userID, tableID string, ids []string) ([]UserLabel, error) {
	if len(ids) > 100 {
		return nil, ErrValidation()
	}
	user, err := parseCaller(userID)
	if err != nil {
		return nil, err
	}
	resource, err := NewResource(string(Table), tableID)
	if err != nil {
		return nil, err
	}
	parsedIDs := make([]pgtype.UUID, 0, len(ids))
	seen := make(map[string]struct{}, len(ids))
	for _, id := range ids {
		value, ok := ParseUUID(id)
		if !ok || id != strings.ToLower(id) {
			return nil, ErrValidation()
		}
		if _, exists := seen[id]; exists {
			continue
		}
		seen[id] = struct{}{}
		parsedIDs = append(parsedIDs, value)
	}
	tx, err := service.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly})
	if err != nil {
		return nil, fmt.Errorf("begin table user label snapshot: %w", err)
	}
	defer func() { _ = tx.Rollback(context.WithoutCancel(ctx)) }()
	queries := service.queries.WithTx(tx)
	access, err := queryAccess(ctx, queries, user, resource)
	if err != nil {
		return nil, err
	}
	if err := access.Check(Read); err != nil {
		return nil, err
	}
	rows, err := queries.ResolveTableUserLabels(ctx, parsedIDs)
	if err != nil {
		return nil, fmt.Errorf("resolve table user labels: %w", err)
	}
	result := make([]UserLabel, 0, len(rows))
	for _, row := range rows {
		result = append(result, UserLabel{ID: row.ID.String(), Name: row.Name})
	}
	return result, nil
}
