package admin

import (
	"context"
	"encoding/json"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"pandatable/internal/admin/adminsql"
	"pandatable/internal/auth"
)

func intPtr(value int) *int { return &value }

func settingsFixture(t *testing.T) *fixture {
	t.Helper()
	f := newFixture(t)
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		f.exec(ctx, "DELETE FROM meta.settings WHERE key=ANY($1::text[])", settingKeys)
	})
	return f
}

func TestSettingsValidationCatalogsAndDefaults(t *testing.T) {
	f := settingsFixture(t)
	defaults, err := f.service.Settings(f.ctx, f.token)
	if err != nil || defaults != defaultSettings() {
		t.Fatalf("defaults=%+v err=%v", defaults, err)
	}
	currencies := Currencies()
	if len(currencies) < 170 || !slices.IsSorted(currencies) || len(slices.Compact(slices.Clone(currencies))) != len(currencies) {
		t.Fatal("currency catalog must be bounded, sorted and unique")
	}
	for _, value := range currencies {
		if len(value) != 3 || strings.Trim(value, "ABCDEFGHIJKLMNOPQRSTUVWXYZ") != "" {
			t.Fatal("invalid currency catalog code")
		}
	}
	for _, patch := range []SettingsPatch{
		{}, {AppName: ptr("")}, {AppName: ptr(" \n ")}, {AppName: ptr("bad\x00name")},
		{AppName: ptr(strings.Repeat("中", 101))}, {DefaultTimezone: ptr("posix/UTC")},
		{DefaultTimezone: ptr("right/UTC")}, {DefaultTimezone: ptr("Not/AZone")},
		{DefaultTimezone: ptr(" Europe/London")}, {DefaultTimezone: ptr(strings.Repeat("x", 129))},
		{DefaultCurrency: ptr("cny")}, {DefaultCurrency: ptr("ZZZ")}, {DefaultLocale: ptr("en-US")},
		{UploadMaxMb: intPtr(0)}, {UploadMaxMb: intPtr(101)}, {HistoryRetentionDays: intPtr(0)},
		{HistoryRetentionDays: intPtr(3651)}, {TrashRetentionDays: intPtr(0)}, {TrashRetentionDays: intPtr(3651)},
		{AppName: ptr("must not persist"), DefaultLocale: ptr("invalid")},
	} {
		_, err := f.service.UpdateSettings(f.ctx, f.token, patch)
		code(t, err, "ERR_VALIDATION")
	}
	unchanged, err := f.service.Settings(f.ctx, f.token)
	if err != nil || unchanged != defaults {
		t.Fatalf("invalid patch changed defaults: %+v %v", unchanged, err)
	}
	for _, bound := range []int{1, 100} {
		result, err := f.service.UpdateSettings(f.ctx, f.token, SettingsPatch{UploadMaxMb: intPtr(bound),
			HistoryRetentionDays: intPtr(1), TrashRetentionDays: intPtr(3650)})
		if err != nil || result.UploadMaxMb != bound || result.HistoryRetentionDays != 1 || result.TrashRetentionDays != 3650 {
			t.Fatalf("boundary failed: %+v %v", result, err)
		}
	}
	result, err := f.service.UpdateSettings(f.ctx, f.token, SettingsPatch{AppName: ptr("  " + strings.Repeat("中", 100) + "  "),
		DefaultCurrency: ptr("USD"), DefaultTimezone: ptr("Europe/London"), DefaultLocale: ptr("zh-CN"),
		HistoryRetentionDays: intPtr(3650), TrashRetentionDays: intPtr(1)})
	if err != nil || result.AppName != strings.Repeat("中", 100) || result.UploadMaxMb != 100 {
		t.Fatalf("valid patch=%+v %v", result, err)
	}
	f.exec(f.ctx, "INSERT INTO meta.settings(key,value) VALUES ('setup_completed','true')")
	t.Cleanup(func() {
		f.exec(context.Background(), "DELETE FROM meta.settings WHERE key='setup_completed'")
	})
	public, err := f.service.AppSettings(f.ctx)
	raw, marshalErr := json.Marshal(public)
	if err != nil || marshalErr != nil || public.AppName != result.AppName || public.DefaultLocale != "zh-CN" ||
		strings.Contains(string(raw), "setup") || strings.Contains(string(raw), "retention") {
		t.Fatalf("public settings=%+v %v", public, err)
	}
}

func TestSettingsActualReadersAndStoredCorruption(t *testing.T) {
	f := settingsFixture(t)
	id, token := f.account("user", "active", false)
	explicit, explicitToken := f.account("user", "active", false)
	f.exec(f.ctx, "UPDATE meta.users SET timezone='Asia/Tokyo',locale='en' WHERE id=$1", explicit)
	_, err := f.service.UpdateSettings(f.ctx, f.token, SettingsPatch{
		AppName: ptr("Team"), DefaultTimezone: ptr("Europe/London"), DefaultLocale: ptr("zh-CN")})
	if err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct{ token, timezone, locale string }{
		{token, "Europe/London", "en"}, {explicitToken, "Asia/Tokyo", "en"},
	} {
		session, err := f.auth.Authenticate(f.ctx, test.token)
		if err != nil || session.User.EffectiveTimezone != test.timezone || session.User.Locale != test.locale {
			t.Fatalf("existing profile changed: %+v %v", session, err)
		}
	}
	user, err := f.service.Create(f.ctx, f.token, CreateInput{Username: "default-locale", DisplayName: "Default",
		Role: "user", Password: password})
	if err != nil {
		t.Fatal(err)
	}
	f.users = append(f.users, user.ID)
	var locale string
	var timezone *string
	if err := f.pool.QueryRow(f.ctx, "SELECT locale,timezone FROM meta.users WHERE id=$1", user.ID).Scan(&locale, &timezone); err != nil ||
		locale != "zh-CN" || timezone != nil {
		t.Fatal("new users must use team locale and NULL timezone", err)
	}
	profile, err := f.auth.UpdateProfile(f.ctx, token, auth.ProfilePatch{Timezone: ptr("Asia/Tokyo"), TimezoneSet: true})
	if err != nil || profile.EffectiveTimezone != "Asia/Tokyo" {
		t.Fatal("profile timezone override failed", err)
	}
	_, err = f.auth.UpdateProfile(f.ctx, token, auth.ProfilePatch{TimezoneSet: true})
	if err != nil {
		t.Fatal(err)
	}
	f.exec(f.ctx, "UPDATE meta.settings SET value='123' WHERE key='default_timezone'")
	if _, err := f.auth.Authenticate(f.ctx, token); err == nil {
		t.Fatal("invalid stored timezone must fail")
	}
	// Public branding must not read or depend on privileged settings.
	if _, err := f.service.AppSettings(f.ctx); err != nil {
		t.Fatal("public settings read unrelated timezone", err)
	}
	f.exec(f.ctx, "UPDATE meta.settings SET value='\"Asia/Shanghai\"' WHERE key='default_timezone'")
	for _, bad := range []string{`null`, `123`, `""`, `"   "`, `"bad\u0001name"`, `{}`} {
		f.exec(f.ctx, "UPDATE meta.settings SET value=$1::jsonb WHERE key='app_name'", bad)
		if _, err := f.service.Settings(f.ctx, f.token); err == nil {
			t.Fatal("stored corruption must not silently default", bad)
		}
		if _, err := f.service.AppSettings(f.ctx); err == nil {
			t.Fatal("public corruption must fail", bad)
		}
	}
	f.exec(f.ctx, "UPDATE meta.settings SET value='\"Team\"' WHERE key='app_name'")
	f.exec(f.ctx, "UPDATE meta.settings SET value='\"invalid\"' WHERE key='default_locale'")
	if _, err := f.service.Create(f.ctx, f.token, CreateInput{Username: "bad-default", DisplayName: "Bad", Role: "user",
		Password: password}); err == nil {
		t.Fatal("creation must not ignore invalid locale")
	}
	f.countSessions(id, 1)
}

func TestSettingsAtomicConcurrentWritesAndAuthorization(t *testing.T) {
	f := settingsFixture(t)
	for _, state := range []struct {
		role, status, want string
		forced             bool
	}{{"user", "active", "ERR_PERMISSION_DENIED", false}, {"admin", "disabled", "ERR_UNAUTHENTICATED", false},
		{"admin", "active", "ERR_PASSWORD_CHANGE_REQUIRED", true}} {
		_, token := f.account(state.role, state.status, state.forced)
		_, err := f.service.Settings(f.ctx, token)
		code(t, err, state.want)
		_, err = f.service.UpdateSettings(f.ctx, token, SettingsPatch{AppName: ptr("Denied")})
		code(t, err, state.want)
		_, err = f.service.Workspaces(f.ctx, token, ListInput{Limit: 50})
		code(t, err, state.want)
	}
	_, err := f.service.Settings(f.ctx, "")
	code(t, err, "ERR_UNAUTHENTICATED")
	var wait sync.WaitGroup
	failures := make(chan error, 2)
	for _, patch := range []SettingsPatch{{AppName: ptr("Concurrent")}, {UploadMaxMb: intPtr(7)}} {
		wait.Add(1)
		go func() {
			defer wait.Done()
			_, err := f.service.UpdateSettings(f.ctx, f.token, patch)
			failures <- err
		}()
	}
	wait.Wait()
	close(failures)
	for err := range failures {
		if err != nil {
			t.Fatal(err)
		}
	}
	before, err := f.service.Settings(f.ctx, f.token)
	if err != nil || before.AppName != "Concurrent" || before.UploadMaxMb != 7 {
		t.Fatalf("lost disjoint update: %+v %v", before, err)
	}
	f.exec(f.ctx, `CREATE FUNCTION meta.fail_setting_commit() RETURNS trigger LANGUAGE plpgsql AS
		$$ BEGIN RAISE EXCEPTION 'fixture commit failure'; END $$`)
	f.exec(f.ctx, `CREATE CONSTRAINT TRIGGER fail_setting_commit AFTER INSERT OR UPDATE ON meta.settings
		DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION meta.fail_setting_commit()`)
	defer f.exec(f.ctx, "DROP FUNCTION meta.fail_setting_commit() CASCADE")
	_, err = f.service.UpdateSettings(f.ctx, f.token, SettingsPatch{AppName: ptr("Rollback"),
		DefaultLocale: ptr("zh-CN"), UploadMaxMb: intPtr(9)})
	if err == nil {
		t.Fatal("commit error must not report success")
	}
	after, err := f.service.Settings(f.ctx, f.token)
	if err != nil || before != after {
		t.Fatalf("partial setting commit: %+v %v", after, err)
	}
}

func TestSettingsRecheckActorAfterLock(t *testing.T) {
	f := settingsFixture(t)
	tx, err := f.pool.Begin(f.ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = tx.Rollback(context.Background()) }()
	if err := adminsql.New(tx).LockUserAdministration(f.ctx); err != nil {
		t.Fatal(err)
	}
	result := make(chan error, 1)
	go func() {
		_, err := f.service.UpdateSettings(f.ctx, f.token, SettingsPatch{AppName: ptr("Denied")})
		result <- err
	}()
	awaitBlocked(t, f)
	f.exec(f.ctx, "UPDATE meta.users SET role='user' WHERE id=$1", f.admin)
	if err := tx.Commit(f.ctx); err != nil {
		t.Fatal(err)
	}

	code(t, <-result, "ERR_PERMISSION_DENIED")
	public, err := f.service.AppSettings(f.ctx)
	if err != nil || public.AppName != "PandaTable" {
		t.Fatal("stale admin write committed", err)
	}
}

func TestSettingsPureBounds(t *testing.T) {
	for _, value := range []Settings{
		{"", "Asia/Shanghai", "CNY", "en", 20, 90, 30},
		{"Team", "Asia/Shanghai", "XXX-invalid", "en", 20, 90, 30},
		{"Team", "Asia/Shanghai", "CNY", "zh", 20, 90, 30},
		{"Team", "Asia/Shanghai", "CNY", "en", 101, 90, 30},
	} {
		code(t, validateSettings(context.Background(), nil, value, false), "ERR_VALIDATION")
	}
	if err := validateSettings(context.Background(), nil, defaultSettings(), false); err != nil {
		t.Fatal(err)
	}
}
