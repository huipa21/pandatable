package admin

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/jackc/pgx/v5/pgtype"

	"pandatable/internal/admin/adminsql"
	"pandatable/internal/apperr"
	"pandatable/internal/auth"
	"pandatable/internal/smtpmail"
)

type SMTPSettings struct {
	Enabled     bool   `json:"enabled"`
	Host        string `json:"host"`
	Port        int    `json:"port"`
	Security    string `json:"security"`
	HasUsername bool   `json:"hasUsername"`
	HasPassword bool   `json:"hasPassword"`
	FromAddress string `json:"fromAddress"`
}

type SMTPPatch struct {
	Enabled                            *bool
	Host, Security, Username, Password *string
	FromAddress                        *string
	Port                               *int
}

func smtpSettings(row adminsql.ReadSMTPSettingsRow) SMTPSettings {
	return SMTPSettings{Enabled: row.Enabled, Host: row.Host, Port: int(row.Port), Security: row.Security,
		HasUsername: row.Username != "", HasPassword: len(row.PasswordCipher) != 0, FromAddress: row.FromAddress}
}

func smtpConfig(row adminsql.ReadSMTPSettingsRow) smtpmail.Config {
	return smtpmail.Config{Enabled: row.Enabled, Host: row.Host, Port: int(row.Port), Security: row.Security,
		Username: row.Username, FromAddress: row.FromAddress}
}

func (service *Service) SMTPSettings(ctx context.Context, token string) (SMTPSettings, error) {
	var result SMTPSettings
	err := service.groupSnapshot(ctx, token, func(queries *adminsql.Queries) error {
		row, err := queries.ReadSMTPSettings(ctx)
		if err != nil {
			return fmt.Errorf("read SMTP settings: %w", err)
		}
		if err := smtpmail.Validate(smtpConfig(row), len(row.PasswordCipher) != 0); err != nil {
			return errors.New("invalid stored SMTP settings")
		}
		result = smtpSettings(row)
		return nil
	})
	return result, err
}

func (service *Service) UpdateSMTPSettings(ctx context.Context, token string, patch SMTPPatch) (SMTPSettings, error) {
	if patch.Enabled == nil && patch.Host == nil && patch.Port == nil && patch.Security == nil &&
		patch.Username == nil && patch.Password == nil && patch.FromAddress == nil {
		return SMTPSettings{}, validation()
	}
	var result SMTPSettings
	err := service.adminMutation(ctx, token, pgtype.UUID{}, true, func(queries *adminsql.Queries, _ *bool) error {
		row, err := queries.ReadSMTPSettings(ctx)
		if err != nil {
			return fmt.Errorf("read SMTP settings: %w", err)
		}
		if patch.Enabled != nil {
			row.Enabled = *patch.Enabled
		}
		if patch.Host != nil {
			row.Host = strings.TrimSpace(*patch.Host)
		}
		if patch.Port != nil {
			if *patch.Port < 1 || *patch.Port > 65535 {
				return validation()
			}
			row.Port = int32(*patch.Port)
		}
		if patch.Security != nil {
			row.Security = *patch.Security
		}
		if patch.Username != nil {
			row.Username = strings.TrimSpace(*patch.Username)
		}
		if patch.FromAddress != nil {
			if *patch.FromAddress == "" {
				row.FromAddress = ""
			} else {
				row.FromAddress, err = auth.NormalizeEmail(*patch.FromAddress)
				if err != nil {
					return validation()
				}
			}
		}
		if patch.Password != nil {
			if !utf8.ValidString(*patch.Password) || len(*patch.Password) > 1024 ||
				strings.ContainsFunc(*patch.Password, unicode.IsControl) {
				return validation()
			}
			if *patch.Password == "" {
				row.PasswordCipher = nil
			} else {
				row.PasswordCipher, err = smtpmail.Seal(service.smtpKey, *patch.Password)
				if errors.Is(err, smtpmail.ErrKey) {
					return apperr.New(503, "ERR_SMTP_KEY", "SMTP encryption key is unavailable")
				}
				if err != nil {
					return fmt.Errorf("encrypt SMTP password: %w", err)
				}
			}
		}
		if err := smtpmail.Validate(smtpConfig(row), len(row.PasswordCipher) != 0); err != nil {
			return validation()
		}
		if err := queries.WriteSMTPSettings(ctx, adminsql.WriteSMTPSettingsParams{
			Enabled: row.Enabled, Host: row.Host, Port: row.Port, Security: row.Security,
			Username: row.Username, PasswordCipher: row.PasswordCipher, FromAddress: row.FromAddress,
		}); err != nil {
			return fmt.Errorf("write SMTP settings: %w", err)
		}
		result = smtpSettings(row)
		return nil
	})
	return result, err
}

func (service *Service) DeliverySMTP(ctx context.Context) (smtpmail.Config, error) {
	row, err := service.queries.ReadSMTPSettings(ctx)
	if err != nil {
		return smtpmail.Config{}, fmt.Errorf("read SMTP settings: %w", err)
	}
	config := smtpConfig(row)
	if err := smtpmail.Validate(config, len(row.PasswordCipher) != 0); err != nil {
		return smtpmail.Config{}, errors.New("invalid stored SMTP settings")
	}
	if len(row.PasswordCipher) != 0 && config.Username != "" {
		config.Password, err = smtpmail.Open(service.smtpKey, row.PasswordCipher)
		if err != nil {
			return smtpmail.Config{}, apperr.New(503, "ERR_SMTP_KEY", "SMTP encryption key is unavailable")
		}
	}
	return config, nil
}
