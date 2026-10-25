package auth

import (
	"context"
	"crypto/subtle"
	"errors"
	"fmt"
	"net/mail"
	"net/netip"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"pandatable/internal/apperr"
	"pandatable/internal/auth/authsql"
)

const SetupTokenLifetime = 24 * time.Hour

type SetupInput struct {
	Token       string
	Username    string
	Email       string
	DisplayName string
	Password    string
	Locale      string
}

func setupComplete() *apperr.Error {
	return apperr.New(410, "ERR_SETUP_COMPLETE", "Setup is already complete")
}

func invalidSetupToken() *apperr.Error {
	return apperr.New(401, "ERR_SETUP_TOKEN", "Invalid or expired setup token")
}

func (service *Service) SetupNeeded(ctx context.Context) (bool, error) {
	needed, err := service.queries.SetupNeeded(ctx)
	if err != nil {
		return false, fmt.Errorf("read setup state: %w", err)
	}
	if !needed.Valid {
		return false, errors.New("setup state is null")
	}
	return needed.Bool, nil
}

func (service *Service) InitializeSetup(ctx context.Context) (string, bool, error) {
	needed, err := service.SetupNeeded(ctx)
	if err != nil {
		return "", false, err
	}
	service.setupMu.Lock()
	defer service.setupMu.Unlock()
	if !needed {
		service.setupToken = ""
		service.setupExpiry = time.Time{}
		return "", false, nil
	}
	token, digest, err := newToken()
	clear(digest)
	if err != nil {
		return "", false, fmt.Errorf("generate setup token: %w", err)
	}
	service.setupToken = token
	service.setupExpiry = service.now().UTC().Add(SetupTokenLifetime)
	return token, true, nil
}

func (service *Service) hasSetupToken(token string, now time.Time) bool {
	if _, err := tokenHash(token); err != nil {
		return false
	}
	service.setupMu.RLock()
	defer service.setupMu.RUnlock()
	return service.setupToken != "" && now.Before(service.setupExpiry) &&
		subtle.ConstantTimeCompare([]byte(token), []byte(service.setupToken)) == 1
}

func normalizeSetupInput(input SetupInput) (SetupInput, error) {
	input.Username = strings.ToLower(strings.TrimSpace(input.Username))
	if len(input.Username) < 3 || len(input.Username) > 32 {
		return SetupInput{}, validation()
	}
	for _, character := range input.Username {
		if !(character >= 'a' && character <= 'z' || character >= '0' && character <= '9' ||
			character == '.' || character == '_' || character == '-') {
			return SetupInput{}, validation()
		}
	}
	input.Email = strings.ToLower(strings.TrimSpace(input.Email))
	if !utf8.ValidString(input.Email) || len(input.Email) > 254 {
		return SetupInput{}, validation()
	}
	address, err := mail.ParseAddress(input.Email)
	if err != nil || address.Address != input.Email || strings.ContainsAny(input.Email, "\r\n") {
		return SetupInput{}, validation()
	}
	if !utf8.ValidString(input.DisplayName) || strings.TrimSpace(input.DisplayName) == "" ||
		utf8.RuneCountInString(input.DisplayName) > 100 || strings.ContainsFunc(input.DisplayName, unicode.IsControl) ||
		!validPassword(input.Password) || input.Locale != "en" && input.Locale != "zh-CN" {
		return SetupInput{}, validation()
	}
	return input, nil
}

func (service *Service) Setup(ctx context.Context, input SetupInput, ip netip.Addr, userAgent string) (Session, string, error) {
	needed, err := service.SetupNeeded(ctx)
	if err != nil {
		return Session{}, "", err
	}
	if !needed {
		return Session{}, "", setupComplete()
	}
	if !service.hasSetupToken(input.Token, service.now()) {
		return Session{}, "", invalidSetupToken()
	}
	input, err = normalizeSetupInput(input)
	if err != nil {
		return Session{}, "", err
	}
	if !ip.IsValid() {
		return Session{}, "", validation()
	}
	passwordHash, err := service.passwords.Hash(ctx, input.Password)
	if err != nil {
		return Session{}, "", passwordError(err)
	}
	token, sessionHash, err := newToken()
	if err != nil {
		return Session{}, "", fmt.Errorf("generate session token: %w", err)
	}
	defer clear(sessionHash)

	var session Session
	err = service.pool.WithTx(ctx, func(tx pgx.Tx) error {
		queries := service.queries.WithTx(tx)
		if err := queries.LockSetup(ctx); err != nil {
			return fmt.Errorf("lock first-run setup: %w", err)
		}
		needed, err := queries.SetupNeeded(ctx)
		if err != nil {
			return fmt.Errorf("recheck setup state: %w", err)
		}
		if !needed.Valid {
			return errors.New("setup state is null")
		}
		if !needed.Bool {
			return setupComplete()
		}
		now := service.now().UTC()
		if !service.hasSetupToken(input.Token, now) {
			return invalidSetupToken()
		}
		userID, err := queries.CreateSetupUser(ctx, authsql.CreateSetupUserParams{
			Username: input.Username, Email: pgtype.Text{String: input.Email, Valid: true},
			DisplayName: input.DisplayName, PasswordHash: passwordHash, Locale: input.Locale,
			AvatarColor: "teal", Now: timestamp(now),
		})
		if err != nil {
			return fmt.Errorf("create first administrator: %w", err)
		}
		userAgent = strings.ToValidUTF8(userAgent, "")
		if len(userAgent) > 512 {
			userAgent = strings.ToValidUTF8(userAgent[:512], "")
		}
		ip = ip.Unmap()
		session.ExpiresAt = now.Add(IdleLifetime)
		if err := queries.CreateSession(ctx, authsql.CreateSessionParams{
			IDHash: sessionHash, UserID: userID, Now: timestamp(now), ExpiresAt: timestamp(session.ExpiresAt),
			Ip: &ip, UserAgent: pgtype.Text{String: userAgent, Valid: true},
		}); err != nil {
			return fmt.Errorf("create setup session: %w", err)
		}
		if err := queries.CompleteSetup(ctx); err != nil {
			return fmt.Errorf("record setup completion: %w", err)
		}
		row, err := queries.GetUser(ctx, userID)
		if err != nil {
			return fmt.Errorf("read first administrator: %w", err)
		}
		session.User, err = userDTO(ctx, queries, row)
		return err
	})
	if err != nil {
		return Session{}, "", err
	}
	service.setupMu.Lock()
	service.setupToken = ""
	service.setupExpiry = time.Time{}
	service.setupMu.Unlock()
	return session, token, nil
}
