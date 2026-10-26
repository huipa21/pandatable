package acl

import (
	"context"
	"fmt"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/jackc/pgx/v5"
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
