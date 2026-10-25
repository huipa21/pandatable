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
	"pandatable/internal/db"
)

type Service struct {
	pool     *db.Pool
	queries  *adminsql.Queries
	auth     *auth.Service
	resolver *acl.Resolver
}

func NewService(pool *db.Pool, authentication *auth.Service, resolver *acl.Resolver) (*Service, error) {
	if pool == nil || pool.Pool == nil || authentication == nil || resolver == nil {
		return nil, errors.New("admin service requires database, auth service and ACL resolver")
	}
	return &Service{pool: pool, queries: adminsql.New(pool), auth: authentication, resolver: resolver}, nil
}

type User struct {
	ID                 string     `json:"id"`
	Username           string     `json:"username"`
	Email              *string    `json:"email"`
	DisplayName        string     `json:"displayName"`
	Role               string     `json:"role"`
	Status             string     `json:"status"`
	MustChangePassword bool       `json:"mustChangePassword"`
	LastLoginAt        *time.Time `json:"lastLoginAt"`
	CreatedAt          time.Time  `json:"createdAt"`
	UpdatedAt          time.Time  `json:"updatedAt"`
}

type ListInput struct {
	Q, Status     string
	Offset, Limit int32
}

type Listing struct {
	Items []User `json:"items"`
	Total int64  `json:"total"`
}

type CreateInput struct {
	Username, DisplayName, Role, Password string
	Email                                 *string
}

type UserPatch struct {
	DisplayName, Email, Role, Status *string
	EmailSet                         bool
}

type UpdateResult struct {
	User            User `json:"user"`
	SessionsRevoked bool `json:"sessionsRevoked"`
}

func validation() error {
	return apperr.New(400, "ERR_VALIDATION", "Invalid user request")
}

func identityConflict() error {
	return apperr.New(409, "ERR_IDENTITY_CONFLICT", "Username or email is already in use or ambiguous")
}

func userDTO(row adminsql.GetUserRow) User {
	user := User{ID: row.ID.String(), Username: row.Username, DisplayName: row.DisplayName,
		Role: row.Role, Status: row.Status, MustChangePassword: row.MustChangePassword,
		CreatedAt: row.CreatedAt.Time.UTC(), UpdatedAt: row.UpdatedAt.Time.UTC()}
	if row.Email.Valid {
		user.Email = &row.Email.String
	}
	if row.LastLoginAt.Valid {
		value := row.LastLoginAt.Time.UTC()
		user.LastLoginAt = &value
	}
	return user
}

func normalizedEmail(value *string) (pgtype.Text, error) {
	if value == nil || strings.TrimSpace(*value) == "" {
		return pgtype.Text{}, nil
	}
	email, err := auth.NormalizeEmail(*value)
	return pgtype.Text{String: email, Valid: err == nil}, err
}

func authorize(ctx context.Context, queries *adminsql.Queries, hash []byte) (pgtype.UUID, error) {
	actor, err := queries.SessionUser(ctx, hash)
	if errors.Is(err, pgx.ErrNoRows) || err == nil && actor.Status != "active" {
		return pgtype.UUID{}, apperr.New(401, "ERR_UNAUTHENTICATED", "Authentication required")
	}
	if err != nil {
		return pgtype.UUID{}, fmt.Errorf("read admin session: %w", err)
	}
	if actor.MustChangePassword {
		return pgtype.UUID{}, apperr.New(403, "ERR_PASSWORD_CHANGE_REQUIRED", "Password change required")
	}
	if actor.Role != "admin" {
		return pgtype.UUID{}, apperr.New(403, "ERR_PERMISSION_DENIED", "Administrator required")
	}
	return actor.ID, nil
}

func (service *Service) List(ctx context.Context, token string, input ListInput) (Listing, error) {
	hash, err := auth.SessionHash(token)
	if err != nil {
		return Listing{}, err
	}
	input.Q = strings.ToLower(strings.TrimSpace(input.Q))
	if input.Offset < 0 || input.Limit < 1 || input.Limit > 500 ||
		input.Status != "" && input.Status != "active" && input.Status != "disabled" ||
		len(input.Q) > 254 || !utf8.ValidString(input.Q) || strings.ContainsFunc(input.Q, unicode.IsControl) {
		return Listing{}, validation()
	}
	tx, err := service.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly})
	if err != nil {
		return Listing{}, fmt.Errorf("begin user listing: %w", err)
	}
	defer func() {
		cleanup, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
		defer cancel()
		_ = tx.Rollback(cleanup)
	}()
	queries := service.queries.WithTx(tx)
	if _, err := authorize(ctx, queries, hash); err != nil {
		return Listing{}, err
	}
	total, err := queries.CountUsers(ctx, adminsql.CountUsersParams{Q: input.Q, Status: input.Status})
	if err != nil {
		return Listing{}, fmt.Errorf("count users: %w", err)
	}
	rows, err := queries.ListUsers(ctx, adminsql.ListUsersParams{Q: input.Q, Status: input.Status,
		PageOffset: input.Offset, PageLimit: input.Limit})
	if err != nil {
		return Listing{}, fmt.Errorf("list users: %w", err)
	}
	result := Listing{Items: make([]User, len(rows)), Total: total}
	for index, row := range rows {
		result.Items[index] = userDTO(adminsql.GetUserRow(row))
	}
	if err := tx.Commit(ctx); err != nil {
		return Listing{}, fmt.Errorf("commit user listing: %w", err)
	}
	return result, nil
}

func (service *Service) mutation(ctx context.Context, token string, target pgtype.UUID,
	fn func(*adminsql.Queries, *bool) error) error {
	return service.adminMutation(ctx, token, target, false, fn)
}

func (service *Service) adminMutation(ctx context.Context, token string, target pgtype.UUID, groupOperation bool,
	fn func(*adminsql.Queries, *bool) error) error {
	hash, err := auth.SessionHash(token)
	if err != nil {
		return err
	}
	if _, err := authorize(ctx, service.queries, hash); err != nil {
		return err
	}
	invalidate := false
	err = service.pool.WithTx(ctx, func(tx pgx.Tx) error {
		queries := service.queries.WithTx(tx)
		if err := queries.LockUserAdministration(ctx); err != nil {
			return fmt.Errorf("lock user administration: %w", err)
		}
		actor, err := authorize(ctx, queries, hash)
		if err != nil {
			return err
		}
		lockActor := queries.LockUser
		if groupOperation {
			lockActor = queries.LockGroupAdministrator
		}
		if _, err := lockActor(ctx, actor); err != nil {
			return fmt.Errorf("lock acting administrator: %w", err)
		}
		if _, err := authorize(ctx, queries, hash); err != nil {
			return err
		}
		if target.Valid && target != actor {
			if _, err := queries.LockUser(ctx, target); errors.Is(err, pgx.ErrNoRows) {
				return apperr.New(404, "ERR_NOT_FOUND", "User not found")
			} else if err != nil {
				return fmt.Errorf("lock managed user: %w", err)
			}
		}
		return fn(queries, &invalidate)
	})
	if invalidate {
		// WithTx can report an uncertain commit; invalidation is required then too.
		service.resolver.Invalidate()
	}
	var postgres *pgconn.PgError
	if errors.As(err, &postgres) && postgres.Code == "23505" {
		return identityConflict()
	}
	return err
}

func checkIdentity(ctx context.Context, queries *adminsql.Queries, id pgtype.UUID, username string, email pgtype.Text) error {
	exists, err := queries.IdentityExists(ctx, adminsql.IdentityExistsParams{ExcludeID: id, Username: username, Email: email})
	if err != nil {
		return fmt.Errorf("check user identity: %w", err)
	}
	if exists || email.Valid && strings.EqualFold(email.String, username) {
		return identityConflict()
	}
	return nil
}

func (service *Service) Create(ctx context.Context, token string, input CreateInput) (User, error) {
	hash, err := auth.SessionHash(token)
	if err != nil {
		return User{}, err
	}
	if _, err := authorize(ctx, service.queries, hash); err != nil {
		return User{}, err
	}
	username, err := auth.NormalizeUsername(input.Username)
	if err != nil {
		return User{}, err
	}
	email, err := normalizedEmail(input.Email)
	if err != nil {
		return User{}, err
	}
	if !auth.ValidDisplayName(input.DisplayName) || input.Role != "admin" && input.Role != "user" {
		return User{}, validation()
	}
	password, err := service.auth.HashPassword(ctx, input.Password)
	if err != nil {
		return User{}, err
	}
	var user User
	err = service.mutation(ctx, token, pgtype.UUID{}, func(queries *adminsql.Queries, _ *bool) error {
		if err := checkIdentity(ctx, queries, pgtype.UUID{}, username, email); err != nil {
			return err
		}
		id, err := queries.CreateUser(ctx, adminsql.CreateUserParams{Username: username, Email: email,
			DisplayName: input.DisplayName, Role: input.Role, PasswordHash: password})
		if err != nil {
			return fmt.Errorf("create user: %w", err)
		}
		row, err := queries.GetUser(ctx, id)
		if err != nil {
			return fmt.Errorf("read created user: %w", err)
		}
		user = userDTO(row)
		return nil
	})
	return user, err
}

func (service *Service) Update(ctx context.Context, token, id string, patch UserPatch) (UpdateResult, error) {
	target, ok := acl.ParseUUID(id)
	if !ok {
		return UpdateResult{}, apperr.New(404, "ERR_NOT_FOUND", "User not found")
	}
	if patch.DisplayName == nil && !patch.EmailSet && patch.Role == nil && patch.Status == nil ||
		patch.DisplayName != nil && !auth.ValidDisplayName(*patch.DisplayName) ||
		patch.Role != nil && *patch.Role != "admin" && *patch.Role != "user" ||
		patch.Status != nil && *patch.Status != "active" && *patch.Status != "disabled" {
		return UpdateResult{}, validation()
	}
	email, err := normalizedEmail(patch.Email)
	if err != nil {
		return UpdateResult{}, err
	}
	var result UpdateResult
	err = service.mutation(ctx, token, target, func(queries *adminsql.Queries, invalidate *bool) error {
		row, err := queries.GetUser(ctx, target)
		if err != nil {
			return fmt.Errorf("read managed user: %w", err)
		}
		next := row
		if patch.DisplayName != nil {
			next.DisplayName = *patch.DisplayName
		}
		if patch.EmailSet {
			next.Email = email
			if err := checkIdentity(ctx, queries, target, next.Username, next.Email); err != nil {
				return err
			}
		}
		if patch.Role != nil {
			next.Role = *patch.Role
		}
		if patch.Status != nil {
			next.Status = *patch.Status
		}
		if row.Role == "admin" && row.Status == "active" && (next.Role != "admin" || next.Status != "active") {
			count, err := queries.CountActiveAdmins(ctx)
			if err != nil {
				return fmt.Errorf("count active administrators: %w", err)
			}
			if count <= 1 {
				return apperr.New(409, "ERR_LAST_ADMIN", "At least one active administrator is required")
			}
		}
		if err := queries.UpdateUser(ctx, adminsql.UpdateUserParams{ID: target, DisplayName: next.DisplayName,
			Email: next.Email, Role: next.Role, Status: next.Status}); err != nil {
			return fmt.Errorf("update user: %w", err)
		}
		result.SessionsRevoked = row.Status != next.Status || row.Role != next.Role
		if result.SessionsRevoked {
			*invalidate = true
			if err := queries.RevokeUserSessions(ctx, target); err != nil {
				return fmt.Errorf("revoke managed user sessions: %w", err)
			}
		}
		updated, err := queries.GetUser(ctx, target)
		if err != nil {
			return fmt.Errorf("read updated user: %w", err)
		}
		result.User = userDTO(updated)
		return nil
	})
	return result, err
}

func (service *Service) ResetPassword(ctx context.Context, token, id, password string) error {
	target, ok := acl.ParseUUID(id)
	if !ok {
		return apperr.New(404, "ERR_NOT_FOUND", "User not found")
	}
	hash, err := auth.SessionHash(token)
	if err != nil {
		return err
	}
	if _, err := authorize(ctx, service.queries, hash); err != nil {
		return err
	}
	passwordHash, err := service.auth.HashPassword(ctx, password)
	if err != nil {
		return err
	}
	return service.mutation(ctx, token, target, func(queries *adminsql.Queries, _ *bool) error {
		if err := queries.ResetPassword(ctx, adminsql.ResetPasswordParams{ID: target, PasswordHash: passwordHash}); err != nil {
			return fmt.Errorf("reset managed password: %w", err)
		}
		if err := queries.RevokeUserSessions(ctx, target); err != nil {
			return fmt.Errorf("revoke reset password sessions: %w", err)
		}
		return nil
	})
}
