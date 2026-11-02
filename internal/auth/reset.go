package auth

import (
	"context"
	"errors"
	"fmt"
	"net/netip"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"pandatable/internal/apperr"
	"pandatable/internal/auth/authsql"
	"pandatable/internal/events"
)

const ResetLifetime = 30 * time.Minute

type ResetMail struct {
	Email  string
	Locale string
	Token  string
}

func (service *Service) CheckResetIP(ip netip.Addr) error {
	if !ip.IsValid() {
		return validation()
	}
	if !service.resetLimits.ip(ip.Unmap(), service.now()) {
		return limited()
	}
	return nil
}

func (service *Service) CheckResetEmail(email string) error {
	if !service.resetLimits.email(email, service.now()) {
		return limited()
	}
	return nil
}

func (service *Service) IssuePasswordReset(ctx context.Context, email string) (ResetMail, bool, error) {
	users, err := service.queries.FindResetUsers(ctx, email)
	if err != nil {
		return ResetMail{}, false, fmt.Errorf("find reset account: %w", err)
	}
	if len(users) != 1 {
		return ResetMail{}, false, nil
	}
	token, hash, err := newToken()
	if err != nil {
		return ResetMail{}, false, fmt.Errorf("generate reset token: %w", err)
	}
	var result ResetMail
	err = service.pool.WithTx(ctx, func(tx pgx.Tx) error {
		queries := service.queries.WithTx(tx)
		locked, err := queries.LockUser(ctx, users[0])
		if errors.Is(err, pgx.ErrNoRows) || err == nil && locked.Status != "active" {
			return nil
		}
		if err != nil {
			return fmt.Errorf("lock reset account: %w", err)
		}
		user, err := queries.GetUser(ctx, users[0])
		if errors.Is(err, pgx.ErrNoRows) {
			return nil
		}
		if err != nil {
			return fmt.Errorf("read reset account: %w", err)
		}
		if !user.Email.Valid || !strings.EqualFold(user.Email.String, email) {
			return nil
		}
		if err := queries.StorePasswordReset(ctx, authsql.StorePasswordResetParams{
			UserID: users[0], TokenHash: hash, ExpiresAt: timestamp(service.now().Add(ResetLifetime)),
		}); err != nil {
			return fmt.Errorf("store reset token: %w", err)
		}
		result = ResetMail{Email: user.Email.String, Locale: user.Locale, Token: token}
		return nil
	})
	return result, result.Token != "", err
}

func invalidReset() error {
	return apperr.New(400, "ERR_RESET_TOKEN", "Reset link is invalid or expired")
}

func (service *Service) CompletePasswordReset(ctx context.Context, token, password string) error {
	if !validPassword(password) {
		return validation()
	}
	hash, err := tokenHash(token)
	if err != nil {
		return invalidReset()
	}
	id, err := service.queries.FindPasswordReset(ctx, authsql.FindPasswordResetParams{
		TokenHash: hash, Now: timestamp(service.now()),
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return invalidReset()
	}
	if err != nil {
		return fmt.Errorf("find reset token: %w", err)
	}
	passwordHash, err := service.HashPassword(ctx, password)
	if err != nil {
		return err
	}
	return service.pool.WithTx(ctx, func(tx pgx.Tx) error {
		queries := service.queries.WithTx(tx)
		locked, err := queries.LockUser(ctx, id)
		if errors.Is(err, pgx.ErrNoRows) || err == nil && locked.Status != "active" {
			return invalidReset()
		}
		if err != nil {
			return fmt.Errorf("lock reset account: %w", err)
		}
		now := timestamp(service.now())
		_, err = queries.LockPasswordReset(ctx, authsql.LockPasswordResetParams{TokenHash: hash, UserID: id, Now: now})
		if errors.Is(err, pgx.ErrNoRows) {
			return invalidReset()
		}
		if err != nil {
			return fmt.Errorf("lock reset token: %w", err)
		}
		if err := queries.ChangePassword(ctx, authsql.ChangePasswordParams{ID: id, PasswordHash: passwordHash, UpdatedAt: now}); err != nil {
			return fmt.Errorf("reset password: %w", err)
		}
		if err := queries.RevokeUserSessions(ctx, id); err != nil {
			return fmt.Errorf("revoke reset sessions: %w", err)
		}
		return events.Emit(ctx, tx, events.Event{Type: "session.check", Recipient: id.String()})
	})
}
