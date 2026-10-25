package auth

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"net/netip"
	"strings"
	"sync"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"pandatable/internal/apperr"
	"pandatable/internal/auth/authsql"
	"pandatable/internal/db"
)

const (
	IdleLifetime     = 7 * 24 * time.Hour
	AbsoluteLifetime = 30 * 24 * time.Hour
)

type User struct {
	ID                 string  `json:"id"`
	Username           string  `json:"username"`
	Email              *string `json:"email"`
	DisplayName        string  `json:"displayName"`
	Role               string  `json:"role"`
	MustChangePassword bool    `json:"mustChangePassword"`
	Locale             string  `json:"locale"`
	Theme              string  `json:"theme"`
	Timezone           *string `json:"timezone"`
	EffectiveTimezone  string  `json:"effectiveTimezone"`
	AvatarColor        string  `json:"avatarColor"`
}

type Session struct {
	User      User
	ExpiresAt time.Time
}

type ProfilePatch struct {
	DisplayName *string
	Locale      *string
	Theme       *string
	Timezone    *string
	TimezoneSet bool
}

type Service struct {
	pool        *db.Pool
	queries     *authsql.Queries
	passwords   *Passwords
	dummy       string
	limits      *limits
	now         func() time.Time
	setupMu     sync.RWMutex
	setupToken  string
	setupExpiry time.Time
}

func New(pool *db.Pool) (*Service, error) {
	if pool == nil || pool.Pool == nil {
		return nil, errors.New("auth requires a database pool")
	}
	passwords := NewPasswords()
	dummy, err := passwords.Hash(context.Background(), rand.Text())
	if err != nil {
		return nil, fmt.Errorf("initialize password worker: %w", err)
	}
	return &Service{pool: pool, queries: authsql.New(pool), passwords: passwords,
		dummy: dummy, limits: newLimits(), now: time.Now}, nil
}

func unauthenticated() *apperr.Error {
	return apperr.New(401, "ERR_UNAUTHENTICATED", "Authentication required")
}

func credentials() *apperr.Error {
	return apperr.New(401, "ERR_INVALID_CREDENTIALS", "Invalid login or password")
}

func validation() *apperr.Error {
	return apperr.New(400, "ERR_VALIDATION", "Invalid request")
}

func limited() *apperr.Error {
	return apperr.New(429, "ERR_RATE_LIMITED", "Too many attempts; retry later")
}

func passwordError(err error) error {
	if errors.Is(err, ErrBusy) {
		return apperr.New(503, "ERR_AUTH_BUSY", "Password worker busy; retry later")
	}
	return fmt.Errorf("password operation: %w", err)
}

func timestamp(now time.Time) pgtype.Timestamptz {
	return pgtype.Timestamptz{Time: now.UTC(), Valid: true}
}

func text(value *string) pgtype.Text {
	if value == nil {
		return pgtype.Text{}
	}
	return pgtype.Text{String: *value, Valid: true}
}

func stringPointer(value pgtype.Text) *string {
	if !value.Valid {
		return nil
	}
	return &value.String
}

func userDTO(ctx context.Context, queries *authsql.Queries, row authsql.GetUserRow) (User, error) {
	timezone := row.Timezone.String
	if !row.Timezone.Valid {
		var err error
		timezone, err = queries.DefaultTimezone(ctx)
		if err != nil {
			return User{}, fmt.Errorf("read default timezone: %w", err)
		}
	}
	valid, err := queries.ValidTimezone(ctx, timezone)
	if err != nil {
		return User{}, fmt.Errorf("validate stored timezone: %w", err)
	}
	if !valid {
		return User{}, errors.New("invalid stored timezone")
	}
	return User{
		ID: row.ID.String(), Username: row.Username, Email: stringPointer(row.Email),
		DisplayName: row.DisplayName, Role: row.Role, MustChangePassword: row.MustChangePassword,
		Locale: row.Locale, Theme: row.Theme, Timezone: stringPointer(row.Timezone),
		EffectiveTimezone: timezone, AvatarColor: row.AvatarColor,
	}, nil
}

func (service *Service) CheckLoginIP(ip netip.Addr) error {
	if !ip.IsValid() {
		return validation()
	}
	if !service.limits.ip(ip.Unmap(), service.now()) {
		return limited()
	}
	return nil
}

func (service *Service) Login(ctx context.Context, login, password string, ip netip.Addr, userAgent string) (Session, string, error) {
	if err := service.CheckLoginIP(ip); err != nil {
		return Session{}, "", err
	}
	login = strings.TrimSpace(login)
	if len(login) == 0 || len(login) > 254 || !utf8.ValidString(login) {
		return Session{}, "", credentials()
	}
	rows, err := service.queries.FindLoginUsers(ctx, login)
	if err != nil {
		return Session{}, "", fmt.Errorf("find login account: %w", err)
	}
	if len(rows) != 1 || rows[0].Status != "active" {
		if validPassword(password) {
			if _, err := service.passwords.Verify(ctx, password, service.dummy); err != nil {
				return Session{}, "", passwordError(err)
			}
		}
		return Session{}, "", credentials()
	}
	account := rows[0]
	id := account.ID.String()
	attempt, ok := service.limits.beginAccount(id, service.now())
	if !ok {
		return Session{}, "", limited()
	}
	var failed, success bool
	defer func() { service.limits.finishAccount(id, attempt, service.now(), failed, success) }()
	if !validPassword(password) {
		failed = true
		return Session{}, "", credentials()
	}
	match, err := service.passwords.Verify(ctx, password, account.PasswordHash)
	if err != nil {
		return Session{}, "", passwordError(err)
	}
	if !match {
		failed = true
		return Session{}, "", credentials()
	}
	token, hash, err := newToken()
	if err != nil {
		return Session{}, "", fmt.Errorf("generate session token: %w", err)
	}
	var session Session
	err = service.pool.WithTx(ctx, func(tx pgx.Tx) error {
		queries := service.queries.WithTx(tx)
		locked, err := queries.LockUser(ctx, account.ID)
		if errors.Is(err, pgx.ErrNoRows) {
			return credentials()
		}
		if err != nil {
			return fmt.Errorf("lock login account: %w", err)
		}
		if locked.Status != "active" || locked.PasswordHash != account.PasswordHash {
			return credentials()
		}
		now := service.now().UTC()
		session.ExpiresAt = now.Add(IdleLifetime)
		// Bound untrusted metadata without splitting a UTF-8 sequence.
		userAgent = strings.ToValidUTF8(userAgent, "")
		if len(userAgent) > 512 {
			userAgent = userAgent[:512]
			userAgent = strings.ToValidUTF8(userAgent, "")
		}
		ip = ip.Unmap()
		if err := queries.CreateSession(ctx, authsql.CreateSessionParams{
			IDHash: hash, UserID: account.ID, Now: timestamp(now), ExpiresAt: timestamp(session.ExpiresAt),
			Ip: &ip, UserAgent: pgtype.Text{String: userAgent, Valid: true},
		}); err != nil {
			return fmt.Errorf("create session: %w", err)
		}
		if err := queries.RecordLogin(ctx, authsql.RecordLoginParams{ID: account.ID, LastLoginAt: timestamp(now)}); err != nil {
			return fmt.Errorf("record login: %w", err)
		}
		row, err := queries.GetUser(ctx, account.ID)
		if err != nil {
			return fmt.Errorf("read login profile: %w", err)
		}
		session.User, err = userDTO(ctx, queries, row)
		return err
	})
	if err != nil {
		return Session{}, "", err
	}
	success = true
	return session, token, nil
}

func (service *Service) Authenticate(ctx context.Context, token string) (Session, error) {
	hash, err := tokenHash(token)
	if err != nil {
		return Session{}, unauthenticated()
	}
	touched, err := service.queries.TouchSession(ctx, authsql.TouchSessionParams{IDHash: hash, Now: timestamp(service.now())})
	if errors.Is(err, pgx.ErrNoRows) {
		return Session{}, unauthenticated()
	}
	if err != nil {
		return Session{}, fmt.Errorf("validate session: %w", err)
	}
	row, err := service.queries.GetUser(ctx, touched.UserID)
	if errors.Is(err, pgx.ErrNoRows) {
		return Session{}, unauthenticated()
	}
	if err != nil {
		return Session{}, fmt.Errorf("read session profile: %w", err)
	}
	user, err := userDTO(ctx, service.queries, row)
	if err != nil {
		return Session{}, err
	}
	return Session{User: user, ExpiresAt: touched.ExpiresAt.Time.UTC()}, nil
}

func (service *Service) Logout(ctx context.Context, token string) error {
	hash, err := tokenHash(token)
	if err != nil {
		return unauthenticated()
	}
	if err := service.queries.RevokeSession(ctx, hash); err != nil {
		return fmt.Errorf("revoke session: %w", err)
	}
	return nil
}

func (service *Service) withSession(ctx context.Context, token string, fn func(*authsql.Queries, authsql.LockUserRow, time.Time) error) error {
	hash, err := tokenHash(token)
	if err != nil {
		return unauthenticated()
	}
	session, err := service.Authenticate(ctx, token)
	if err != nil {
		return err
	}
	var id pgtype.UUID
	if err := id.Scan(session.User.ID); err != nil {
		return fmt.Errorf("decode session user ID: %w", err)
	}
	return service.pool.WithTx(ctx, func(tx pgx.Tx) error {
		queries := service.queries.WithTx(tx)
		locked, err := queries.LockUser(ctx, id)
		if errors.Is(err, pgx.ErrNoRows) {
			return unauthenticated()
		}
		if err != nil {
			return fmt.Errorf("lock session user: %w", err)
		}
		if locked.Status != "active" {
			return unauthenticated()
		}
		now := service.now().UTC()
		_, err = queries.CheckSession(ctx, authsql.CheckSessionParams{IDHash: hash, UserID: id, Now: timestamp(now)})
		if errors.Is(err, pgx.ErrNoRows) {
			return unauthenticated()
		}
		if err != nil {
			return fmt.Errorf("check locked session: %w", err)
		}
		return fn(queries, locked, now)
	})
}

func (service *Service) ChangePassword(ctx context.Context, token, current, next string) error {
	if !validPassword(next) {
		return validation()
	}
	return service.withSession(ctx, token, func(queries *authsql.Queries, locked authsql.LockUserRow, now time.Time) error {
		match, err := service.passwords.Verify(ctx, current, locked.PasswordHash)
		if errors.Is(err, ErrPassword) {
			return credentials()
		}
		if err != nil {
			return passwordError(err)
		}
		if !match {
			return credentials()
		}
		if next == current {
			return validation()
		}
		hash, err := service.passwords.Hash(ctx, next)
		if err != nil {
			return passwordError(err)
		}
		if err := queries.ChangePassword(ctx, authsql.ChangePasswordParams{ID: locked.ID, PasswordHash: hash, UpdatedAt: timestamp(now)}); err != nil {
			return fmt.Errorf("change password: %w", err)
		}
		if err := queries.RevokeUserSessions(ctx, locked.ID); err != nil {
			return fmt.Errorf("revoke password sessions: %w", err)
		}
		return nil
	})
}

func (service *Service) UpdateProfile(ctx context.Context, token string, patch ProfilePatch) (User, error) {
	if patch.DisplayName == nil && patch.Locale == nil && patch.Theme == nil && !patch.TimezoneSet {
		return User{}, validation()
	}
	if patch.DisplayName != nil {
		value := *patch.DisplayName
		if !utf8.ValidString(value) || strings.TrimSpace(value) == "" || utf8.RuneCountInString(value) > 100 ||
			strings.ContainsFunc(value, unicode.IsControl) {
			return User{}, validation()
		}
	}
	if patch.Locale != nil && *patch.Locale != "en" && *patch.Locale != "zh-CN" {
		return User{}, validation()
	}
	if patch.Theme != nil && *patch.Theme != "light" && *patch.Theme != "dark" && *patch.Theme != "system" {
		return User{}, validation()
	}
	if patch.TimezoneSet && patch.Timezone != nil && (len(*patch.Timezone) > 128 || !utf8.ValidString(*patch.Timezone)) {
		return User{}, validation()
	}
	var user User
	err := service.withSession(ctx, token, func(queries *authsql.Queries, locked authsql.LockUserRow, now time.Time) error {
		row, err := queries.GetUser(ctx, locked.ID)
		if err != nil {
			return fmt.Errorf("read editable profile: %w", err)
		}
		if row.MustChangePassword {
			return apperr.New(403, "ERR_PASSWORD_CHANGE_REQUIRED", "Password change required")
		}
		if patch.DisplayName != nil {
			row.DisplayName = *patch.DisplayName
		}
		if patch.Locale != nil {
			row.Locale = *patch.Locale
		}
		if patch.Theme != nil {
			row.Theme = *patch.Theme
		}
		if patch.TimezoneSet {
			if patch.Timezone != nil {
				valid, err := queries.ValidTimezone(ctx, *patch.Timezone)
				if err != nil {
					return fmt.Errorf("validate profile timezone: %w", err)
				}
				if !valid {
					return validation()
				}
			}
			row.Timezone = text(patch.Timezone)
		}
		user, err = userDTO(ctx, queries, row)
		if err != nil {
			return err
		}
		if err := queries.UpdateProfile(ctx, authsql.UpdateProfileParams{ID: locked.ID, DisplayName: row.DisplayName,
			Locale: row.Locale, Theme: row.Theme, Timezone: row.Timezone, UpdatedAt: timestamp(now)}); err != nil {
			return fmt.Errorf("update profile: %w", err)
		}
		return nil
	})
	return user, err
}

func (service *Service) Timezones(ctx context.Context) ([]string, error) {
	names, err := service.queries.Timezones(ctx)
	if err != nil {
		return nil, fmt.Errorf("list timezones: %w", err)
	}
	return names, nil
}
