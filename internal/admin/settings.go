package admin

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/jackc/pgx/v5/pgtype"

	"pandatable/internal/admin/adminsql"
	"pandatable/internal/apperr"
	"pandatable/internal/auth"
)

type Settings struct {
	AppName              string `json:"appName"`
	DefaultTimezone      string `json:"defaultTimezone"`
	DefaultCurrency      string `json:"defaultCurrency"`
	DefaultLocale        string `json:"defaultLocale"`
	UploadMaxMb          int    `json:"uploadMaxMb"`
	HistoryRetentionDays int    `json:"historyRetentionDays"`
	TrashRetentionDays   int    `json:"trashRetentionDays"`
}

type AppSettings struct {
	AppName       string `json:"appName"`
	DefaultLocale string `json:"defaultLocale"`
}

type SettingsPatch struct {
	AppName, DefaultTimezone, DefaultCurrency, DefaultLocale *string
	UploadMaxMb, HistoryRetentionDays, TrashRetentionDays    *int
}

var settingKeys = []string{"app_name", "default_timezone", "default_currency", "default_locale",
	"upload_max_mb", "history_retention_days", "trash_retention_days"}

// ISO 4217 maintenance agency SIX, list one, published 2026-09-17.
const currencyCodes = "AED AFN ALL AMD AOA ARS AUD AWG AZN BAM BBD BDT BHD BIF BMD BND BOB BOV BRL BSD BTN BWP BYN BZD CAD CDF CHE CHF CHW CLF CLP CNY COP COU CRC CUP CVE CZK DJF DKK DOP DZD EGP ERN ETB EUR FJD FKP GBP GEL GHS GIP GMD GNF GTQ GYD HKD HNL HTG HUF IDR ILS INR IQD IRR ISK JMD JOD JPY KES KGS KHR KMF KPW KRW KWD KYD KZT LAK LBP LKR LRD LSL LYD MAD MDL MGA MKD MMK MNT MOP MRU MUR MVR MWK MXN MXV MYR MZN NAD NGN NIO NOK NPR NZD OMR PAB PEN PGK PHP PKR PLN PYG QAR RON RSD RUB RWF SAR SBD SCR SDG SEK SGD SHP SLE SOS SRD SSP STN SVC SYP SZL THB TJS TMT TND TOP TRY TTD TWD TZS UAH UGX USD USN UYI UYU UYW UZS VED VES VND VUV WST XAD XAF XAG XAU XBA XBB XBC XBD XCD XCG XDR XOF XPD XPF XPT XSU XTS XUA XXX YER ZAR ZMW ZWG"

func Currencies() []string { return strings.Fields(currencyCodes) }

func defaultSettings() Settings {
	return Settings{"PandaTable", "Asia/Shanghai", "CNY", "en", 20, 90, 30}
}

func settingsValidation() error {
	return apperr.New(400, "ERR_VALIDATION", "Invalid settings request")
}

func validLocale(value string) bool { return value == "en" || value == "zh-CN" }

func readSettings(ctx context.Context, queries *adminsql.Queries, keys []string) (Settings, error) {
	result := defaultSettings()
	rows, err := queries.ReadSettings(ctx, keys)
	if err != nil {
		return Settings{}, fmt.Errorf("read settings: %w", err)
	}
	fields := map[string]any{
		"app_name": &result.AppName, "default_timezone": &result.DefaultTimezone,
		"default_currency": &result.DefaultCurrency, "default_locale": &result.DefaultLocale,
		"upload_max_mb": &result.UploadMaxMb, "history_retention_days": &result.HistoryRetentionDays,
		"trash_retention_days": &result.TrashRetentionDays,
	}
	for _, row := range rows {
		if string(row.Value) == "null" || json.Unmarshal(row.Value, fields[row.Key]) != nil {
			return Settings{}, errors.New("invalid stored settings type")
		}
	}
	if err := validateSettings(ctx, queries, result, slices.Contains(keys, "default_timezone")); err != nil {
		return Settings{}, errors.New("invalid stored settings")
	}
	return result, nil
}

func validateSettings(ctx context.Context, queries *adminsql.Queries, value Settings, timezone bool) error {
	if !auth.ValidDisplayName(value.AppName) || value.AppName != strings.TrimSpace(value.AppName) ||
		!validLocale(value.DefaultLocale) || !slices.Contains(Currencies(), value.DefaultCurrency) ||
		value.UploadMaxMb < 1 || value.UploadMaxMb > 100 ||
		value.HistoryRetentionDays < 1 || value.HistoryRetentionDays > 3650 ||
		value.TrashRetentionDays < 1 || value.TrashRetentionDays > 3650 ||
		len(value.DefaultTimezone) > 128 {
		return settingsValidation()
	}
	if timezone {
		valid, err := queries.ValidSettingTimezone(ctx, value.DefaultTimezone)
		if err != nil {
			return fmt.Errorf("validate settings timezone: %w", err)
		}
		if !valid {
			return settingsValidation()
		}
	}
	return nil
}

func (service *Service) Settings(ctx context.Context, token string) (Settings, error) {
	var result Settings
	err := service.groupSnapshot(ctx, token, func(queries *adminsql.Queries) error {
		var err error
		result, err = readSettings(ctx, queries, settingKeys)
		return err
	})
	return result, err
}

func (service *Service) AppSettings(ctx context.Context) (AppSettings, error) {
	value, err := readSettings(ctx, service.queries, []string{"app_name", "default_locale"})
	return AppSettings{AppName: value.AppName, DefaultLocale: value.DefaultLocale}, err
}

func (service *Service) UpdateSettings(ctx context.Context, token string, patch SettingsPatch) (Settings, error) {
	var result Settings
	err := service.adminMutation(ctx, token, pgtype.UUID{}, true, func(queries *adminsql.Queries, _ *bool) error {
		value, err := readSettings(ctx, queries, settingKeys)
		if err != nil {
			return err
		}
		changes := make(map[string]any)
		for _, field := range []struct {
			key   string
			input *string
			dest  *string
		}{
			{"app_name", patch.AppName, &value.AppName},
			{"default_timezone", patch.DefaultTimezone, &value.DefaultTimezone},
			{"default_currency", patch.DefaultCurrency, &value.DefaultCurrency},
			{"default_locale", patch.DefaultLocale, &value.DefaultLocale},
		} {
			if field.input != nil {
				*field.dest = *field.input
				if field.key == "app_name" {
					*field.dest = strings.TrimSpace(*field.dest)
				}
				changes[field.key] = *field.dest
			}
		}
		for _, field := range []struct {
			key   string
			input *int
			dest  *int
		}{
			{"upload_max_mb", patch.UploadMaxMb, &value.UploadMaxMb},
			{"history_retention_days", patch.HistoryRetentionDays, &value.HistoryRetentionDays},
			{"trash_retention_days", patch.TrashRetentionDays, &value.TrashRetentionDays},
		} {
			if field.input != nil {
				*field.dest = *field.input
				changes[field.key] = *field.dest
			}
		}
		if len(changes) == 0 {
			return settingsValidation()
		}
		if err := validateSettings(ctx, queries, value, true); err != nil {
			return err
		}
		for _, key := range settingKeys {
			if changed, ok := changes[key]; ok {
				raw, err := json.Marshal(changed)
				if err != nil {
					return fmt.Errorf("encode setting: %w", err)
				}
				if err := queries.WriteSetting(ctx, adminsql.WriteSettingParams{Key: key, Value: raw}); err != nil {
					return fmt.Errorf("write setting: %w", err)
				}
			}
		}
		result = value
		return nil
	})
	return result, err
}
