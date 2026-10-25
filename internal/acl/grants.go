package acl

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"pandatable/internal/acl/aclsql"
	"pandatable/internal/db"
)

// Service implements the grants API. Every mutation re-evaluates the caller's
// ownership uncached inside a transaction that holds the workspace lock, so a
// stale cache can never authorize a change and concurrent changes serialize.
type Service struct {
	pool     *db.Pool
	queries  *aclsql.Queries
	resolver *Resolver
}

func NewService(pool *db.Pool, resolver *Resolver) (*Service, error) {
	if pool == nil || pool.Pool == nil || resolver == nil {
		return nil, errors.New("acl service requires a database pool and resolver")
	}
	return &Service{pool: pool, queries: aclsql.New(pool), resolver: resolver}, nil
}

func (service *Service) Resolver() *Resolver { return service.resolver }

type Principal struct {
	Name     string  `json:"name"`
	Username *string `json:"username"`
	Status   *string `json:"status"`
}

type Grant struct {
	ID            string     `json:"id"`
	ResourceType  string     `json:"resourceType"`
	ResourceID    string     `json:"resourceId"`
	PrincipalType string     `json:"principalType"`
	PrincipalID   string     `json:"principalId"`
	Level         string     `json:"level"`
	GrantedBy     *string    `json:"grantedBy"`
	CreatedAt     time.Time  `json:"createdAt"`
	Principal     *Principal `json:"principal"`
}

// DirectGrant adds the level the same principal already receives from an
// ancestor, so clients can warn that the lower direct level has no effect.
type DirectGrant struct {
	Grant
	InheritedLevel *string `json:"inheritedLevel"`
}

type ResourceRef struct {
	Type string `json:"type"`
	ID   string `json:"id"`
}

type Listing struct {
	Resource       ResourceRef   `json:"resource"`
	EffectiveLevel string        `json:"effectiveLevel"`
	Items          []DirectGrant `json:"items"`
	Inherited      []Grant       `json:"inherited"`
}

type PutInput struct {
	ResourceType  string
	ResourceID    string
	PrincipalType string
	PrincipalID   string
	Level         string
}

type PutResult struct {
	Grant   DirectGrant `json:"grant"`
	Created bool        `json:"created"`
}

type target struct {
	workspace, book pgtype.UUID
}

type grantRow struct {
	aclsql.MetaGrant
	userUsername, userDisplayName, userStatus, groupName pgtype.Text
}

func uuidPointer(value pgtype.UUID) *string {
	if !value.Valid {
		return nil
	}
	text := value.String()
	return &text
}

func (row grantRow) dto() Grant {
	grant := Grant{ID: row.ID.String(), ResourceType: row.ResourceType, ResourceID: row.ResourceID.String(),
		PrincipalType: row.PrincipalType, PrincipalID: row.PrincipalID.String(), Level: row.Level,
		GrantedBy: uuidPointer(row.GrantedBy), CreatedAt: row.CreatedAt.Time.UTC()}
	switch {
	case row.PrincipalType == "user" && row.userUsername.Valid:
		grant.Principal = &Principal{Name: row.userDisplayName.String, Username: &row.userUsername.String,
			Status: &row.userStatus.String}
	case row.PrincipalType == "group" && row.groupName.Valid:
		grant.Principal = &Principal{Name: row.groupName.String}
	}
	return grant
}

func directRows(rows []aclsql.ListDirectGrantsRow) []grantRow {
	result := make([]grantRow, len(rows))
	for index, row := range rows {
		result[index] = grantRow{MetaGrant: aclsql.MetaGrant{ID: row.ID, ResourceType: row.ResourceType,
			ResourceID: row.ResourceID, PrincipalType: row.PrincipalType, PrincipalID: row.PrincipalID,
			Level: row.Level, GrantedBy: row.GrantedBy, CreatedAt: row.CreatedAt},
			userUsername: row.UserUsername, userDisplayName: row.UserDisplayName,
			userStatus: row.UserStatus, groupName: row.GroupName}
	}
	return result
}

func inheritedRows(rows []aclsql.ListInheritedGrantsRow) []grantRow {
	result := make([]grantRow, len(rows))
	for index, row := range rows {
		result[index] = grantRow{MetaGrant: aclsql.MetaGrant{ID: row.ID, ResourceType: row.ResourceType,
			ResourceID: row.ResourceID, PrincipalType: row.PrincipalType, PrincipalID: row.PrincipalID,
			Level: row.Level, GrantedBy: row.GrantedBy, CreatedAt: row.CreatedAt},
			userUsername: row.UserUsername, userDisplayName: row.UserDisplayName,
			userStatus: row.UserStatus, groupName: row.GroupName}
	}
	return result
}

func notFoundIfNoRows(err error) error {
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrNotFound()
	}
	return err
}

// resolveTarget finds the live ancestors of the resource; a missing or trashed
// target (or ancestor) is indistinguishable from a hidden one.
func resolveTarget(ctx context.Context, queries *aclsql.Queries, resource Resource) (target, error) {
	switch resource.Type {
	case Workspace:
		workspace, err := queries.WorkspaceTarget(ctx, resource.ID)
		return target{workspace: workspace}, notFoundIfNoRows(err)
	case Book:
		row, err := queries.BookTarget(ctx, resource.ID)
		return target{workspace: row.WorkspaceID, book: row.BookID}, notFoundIfNoRows(err)
	case Table:
		row, err := queries.TableTarget(ctx, resource.ID)
		return target{workspace: row.WorkspaceID, book: row.BookID}, notFoundIfNoRows(err)
	default:
		return target{}, ErrValidation()
	}
}

// lockTarget takes the D-070 serialization locks in a fixed order.
func lockTarget(ctx context.Context, queries *aclsql.Queries, resource Resource) (target, error) {
	path, err := resolveTarget(ctx, queries, resource)
	if err != nil {
		return target{}, err
	}
	if _, err := queries.LockWorkspace(ctx, path.workspace); err != nil {
		return target{}, notFoundIfNoRows(err)
	}
	if resource.Type == Workspace {
		return path, nil
	}
	if _, err := queries.LockBook(ctx, aclsql.LockBookParams{BookID: path.book, WorkspaceID: path.workspace}); err != nil {
		return target{}, notFoundIfNoRows(err)
	}
	if resource.Type == Table {
		if _, err := queries.LockTable(ctx, aclsql.LockTableParams{TableID: resource.ID, BookID: path.book}); err != nil {
			return target{}, notFoundIfNoRows(err)
		}
	}
	return path, nil
}

func parseCaller(userID string) (pgtype.UUID, error) {
	user, ok := ParseUUID(userID)
	if !ok {
		return pgtype.UUID{}, ErrValidation()
	}
	return user, nil
}

func requireOwner(ctx context.Context, queries *aclsql.Queries, user pgtype.UUID, resource Resource) (Access, error) {
	access, err := queryAccess(ctx, queries, user, resource)
	if err != nil {
		return Access{}, err
	}
	return access, access.Check(Owner)
}

// List returns direct grants plus the grants inherited from ancestors. Only an
// effective owner of the resource may read its sharing configuration.
func (service *Service) List(ctx context.Context, userID string, resource Resource) (Listing, error) {
	user, err := parseCaller(userID)
	if err != nil {
		return Listing{}, err
	}
	tx, err := service.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly})
	if err != nil {
		return Listing{}, fmt.Errorf("begin grants snapshot: %w", err)
	}
	defer func() { _ = tx.Rollback(context.WithoutCancel(ctx)) }()
	queries := service.queries.WithTx(tx)
	access, err := requireOwner(ctx, queries, user, resource)
	if err != nil {
		return Listing{}, err
	}
	path, err := resolveTarget(ctx, queries, resource)
	if err != nil {
		return Listing{}, err
	}
	direct, err := queries.ListDirectGrants(ctx, aclsql.ListDirectGrantsParams{
		ResourceType: string(resource.Type), ResourceID: resource.ID})
	if err != nil {
		return Listing{}, fmt.Errorf("list direct grants: %w", err)
	}
	ancestors, err := listInherited(ctx, queries, resource, path)
	if err != nil {
		return Listing{}, err
	}
	listing := Listing{Resource: ResourceRef{Type: string(resource.Type), ID: resource.ID.String()},
		EffectiveLevel: access.Level.String(), Items: make([]DirectGrant, 0, len(direct)),
		Inherited: make([]Grant, 0, len(ancestors))}
	for _, row := range directRows(direct) {
		listing.Items = append(listing.Items, directDTO(row, ancestors))
	}
	for _, row := range ancestors {
		listing.Inherited = append(listing.Inherited, row.dto())
	}
	return listing, nil
}

func listInherited(ctx context.Context, queries *aclsql.Queries, resource Resource, path target) ([]grantRow, error) {
	params := aclsql.ListInheritedGrantsParams{}
	switch resource.Type {
	case Book:
		params.WorkspaceID = path.workspace
	case Table:
		params.WorkspaceID, params.BookID = path.workspace, path.book
	default:
		return nil, nil
	}
	rows, err := queries.ListInheritedGrants(ctx, params)
	if err != nil {
		return nil, fmt.Errorf("list inherited grants: %w", err)
	}
	return inheritedRows(rows), nil
}

func directDTO(row grantRow, ancestors []grantRow) DirectGrant {
	best := None
	for _, ancestor := range ancestors {
		if ancestor.PrincipalType == row.PrincipalType && ancestor.PrincipalID == row.PrincipalID {
			if level, _ := ParseLevel(ancestor.Level); level > best {
				best = level
			}
		}
	}
	result := DirectGrant{Grant: row.dto()}
	if best != None {
		text := best.String()
		result.InheritedLevel = &text
	}
	return result
}

// mutate authorizes the caller as owner of the resource before and after
// taking the target locks, runs fn and invalidates the cache when fn wrote.
func (service *Service) mutate(ctx context.Context, userID string, resource Resource,
	fn func(queries *aclsql.Queries, user pgtype.UUID, path target) (bool, error)) error {
	user, err := parseCaller(userID)
	if err != nil {
		return err
	}
	// Cheap rejection first so non-owners cannot contend for the workspace lock.
	if _, err := requireOwner(ctx, service.queries, user, resource); err != nil {
		return err
	}
	var mutated bool
	err = service.pool.WithTx(ctx, func(tx pgx.Tx) error {
		queries := service.queries.WithTx(tx)
		path, err := lockTarget(ctx, queries, resource)
		if err != nil {
			return err
		}
		if _, err := requireOwner(ctx, queries, user, resource); err != nil {
			return err
		}
		mutated, err = fn(queries, user, path)
		return err
	})
	if mutated {
		service.resolver.Invalidate()
	}
	return err
}

func validatePrincipal(ctx context.Context, queries *aclsql.Queries, principalType string, id pgtype.UUID) error {
	switch principalType {
	case "user":
		status, err := queries.LockUserPrincipal(ctx, id)
		if errors.Is(err, pgx.ErrNoRows) {
			return errPrincipalNotFound()
		}
		if err != nil {
			return fmt.Errorf("lock user principal: %w", err)
		}
		if status != "active" {
			return errPrincipalDisabled()
		}
	case "group":
		if _, err := queries.LockGroupPrincipal(ctx, id); errors.Is(err, pgx.ErrNoRows) {
			return errPrincipalNotFound()
		} else if err != nil {
			return fmt.Errorf("lock group principal: %w", err)
		}
	default:
		return ErrValidation()
	}
	return nil
}

// ensureOwnerRemains rejects removing or downgrading the last active direct
// user owner of a workspace (D-023); group ownership and disabled owners do
// not count.
func ensureOwnerRemains(ctx context.Context, queries *aclsql.Queries, grant aclsql.MetaGrant) error {
	if grant.ResourceType != string(Workspace) || grant.PrincipalType != "user" || grant.Level != Owner.String() {
		return nil
	}
	status, err := queries.LockUserPrincipal(ctx, grant.PrincipalID)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("lock owner principal: %w", err)
	}
	if status != "active" {
		return nil
	}
	others, err := queries.CountOtherWorkspaceOwners(ctx, aclsql.CountOtherWorkspaceOwnersParams{
		WorkspaceID: grant.ResourceID, ExcludedGrantID: grant.ID})
	if err != nil {
		return fmt.Errorf("count workspace owners: %w", err)
	}
	if others == 0 {
		return errLastOwner()
	}
	return nil
}

// Put creates or replaces the (resource, principal) grant.
func (service *Service) Put(ctx context.Context, userID string, input PutInput) (PutResult, error) {
	resource, err := NewResource(input.ResourceType, input.ResourceID)
	if err != nil {
		return PutResult{}, err
	}
	level, ok := ParseLevel(input.Level)
	if !ok || (input.PrincipalType != "user" && input.PrincipalType != "group") {
		return PutResult{}, ErrValidation()
	}
	principal, ok := ParseUUID(input.PrincipalID)
	if !ok {
		return PutResult{}, ErrValidation()
	}
	var result PutResult
	err = service.mutate(ctx, userID, resource, func(queries *aclsql.Queries, caller pgtype.UUID, path target) (bool, error) {
		if err := validatePrincipal(ctx, queries, input.PrincipalType, principal); err != nil {
			return false, err
		}
		existing, err := queries.LockGrantByPair(ctx, aclsql.LockGrantByPairParams{
			ResourceType: string(resource.Type), ResourceID: resource.ID,
			PrincipalType: input.PrincipalType, PrincipalID: principal})
		var stored aclsql.MetaGrant
		mutated := false
		switch {
		case errors.Is(err, pgx.ErrNoRows):
			stored, err = queries.InsertGrant(ctx, aclsql.InsertGrantParams{
				ResourceType: string(resource.Type), ResourceID: resource.ID,
				PrincipalType: input.PrincipalType, PrincipalID: principal,
				Level: level.String(), GrantedBy: caller})
			if err != nil {
				return false, fmt.Errorf("insert grant: %w", err)
			}
			result.Created, mutated = true, true
		case err != nil:
			return false, fmt.Errorf("lock grant: %w", err)
		case existing.Level == level.String():
			stored = existing
		default:
			if level != Owner {
				if err := ensureOwnerRemains(ctx, queries, existing); err != nil {
					return false, err
				}
			}
			stored, err = queries.UpdateGrantLevel(ctx, aclsql.UpdateGrantLevelParams{
				ID: existing.ID, Level: level.String(), GrantedBy: caller})
			if err != nil {
				return false, fmt.Errorf("update grant: %w", err)
			}
			mutated = true
		}
		result.Grant, err = service.describe(ctx, queries, resource, path, stored)
		return mutated, err
	})
	if err != nil {
		return PutResult{}, err
	}
	return result, nil
}

func (service *Service) describe(ctx context.Context, queries *aclsql.Queries, resource Resource, path target,
	stored aclsql.MetaGrant) (DirectGrant, error) {
	direct, err := queries.ListDirectGrants(ctx, aclsql.ListDirectGrantsParams{
		ResourceType: string(resource.Type), ResourceID: resource.ID})
	if err != nil {
		return DirectGrant{}, fmt.Errorf("describe grant: %w", err)
	}
	ancestors, err := listInherited(ctx, queries, resource, path)
	if err != nil {
		return DirectGrant{}, err
	}
	for _, row := range directRows(direct) {
		if row.ID == stored.ID {
			return directDTO(row, ancestors), nil
		}
	}
	return DirectGrant{}, errors.New("stored grant missing from its resource")
}

// Delete removes one grant by id.
func (service *Service) Delete(ctx context.Context, userID, grantID string) error {
	id, ok := ParseUUID(grantID)
	if !ok {
		return ErrValidation()
	}
	if _, err := parseCaller(userID); err != nil {
		return err
	}
	grant, err := service.queries.GetGrant(ctx, id)
	if err != nil {
		return notFoundIfNoRows(err)
	}
	resource, ok := grantResource(grant)
	if !ok {
		return ErrNotFound()
	}
	return service.mutate(ctx, userID, resource, func(queries *aclsql.Queries, _ pgtype.UUID, _ target) (bool, error) {
		locked, err := queries.LockGrant(ctx, id)
		if err != nil {
			return false, notFoundIfNoRows(err)
		}
		if locked.ResourceType != grant.ResourceType || locked.ResourceID != grant.ResourceID {
			return false, ErrNotFound()
		}
		if err := ensureOwnerRemains(ctx, queries, locked); err != nil {
			return false, err
		}
		if err := queries.DeleteGrant(ctx, id); err != nil {
			return false, fmt.Errorf("delete grant: %w", err)
		}
		return true, nil
	})
}

func grantResource(grant aclsql.MetaGrant) (Resource, bool) {
	kind, ok := ParseResourceType(grant.ResourceType)
	return Resource{Type: kind, ID: grant.ResourceID}, ok
}
