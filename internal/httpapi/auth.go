package httpapi

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"mime"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"

	"pandatable/internal/acl"
	"pandatable/internal/admin"
	"pandatable/internal/apperr"
	"pandatable/internal/auth"
	"pandatable/internal/structure"
)

const sessionCookie = "pt_session"

type AuthOptions struct {
	Service        *auth.Service
	ACL            *acl.Service
	Structure      *structure.Service
	Admin          *admin.Service
	System         *admin.SystemMonitor
	BaseURL        string
	TrustedProxies []netip.Prefix
}

type authHTTP struct {
	service   *auth.Service
	acl       *acl.Service
	structure *structure.Service
	admin     *admin.Service
	system    *admin.SystemMonitor
	logger    *slog.Logger
	origin    string
	secure    bool
	proxies   []netip.Prefix
}

func newAuthHTTP(logger *slog.Logger, options AuthOptions) (*authHTTP, error) {
	if options.Service == nil {
		return nil, errors.New("auth service is required")
	}
	origin, secure, err := authOrigin(options.BaseURL)
	if err != nil {
		return nil, err
	}
	if !secure {
		logger.Warn("local HTTP authentication uses non-Secure cookies")
	}
	return &authHTTP{service: options.Service, acl: options.ACL, structure: options.Structure, admin: options.Admin, system: options.System, logger: logger, origin: origin,
		secure: secure, proxies: options.TrustedProxies}, nil
}

func authOrigin(baseURL string) (string, bool, error) {
	parsed, err := url.Parse(baseURL)
	if err != nil || parsed.Hostname() == "" || parsed.User != nil ||
		parsed.RawQuery != "" || parsed.Fragment != "" || (parsed.Scheme != "http" && parsed.Scheme != "https") {
		return "", false, errors.New("auth requires a valid HTTP(S) base URL")
	}
	host := strings.ToLower(parsed.Hostname())
	ip, ipErr := netip.ParseAddr(host)
	secure := parsed.Scheme == "https"
	if !secure && host != "localhost" && (ipErr != nil || !ip.IsLoopback() || ip.Zone() != "") {
		return "", false, errors.New("HTTP auth requires literal localhost or loopback PT_BASE_URL; use HTTPS in production")
	}
	port := parsed.Port()
	if port == "80" && !secure || port == "443" && secure {
		port = ""
	}
	if strings.Contains(host, ":") {
		host = "[" + host + "]"
	}
	if port != "" {
		host += ":" + port
	}
	return parsed.Scheme + "://" + host, secure, nil
}

func trusted(ip netip.Addr, proxies []netip.Prefix) bool {
	for _, prefix := range proxies {
		if prefix.Contains(ip.Unmap()) {
			return true
		}
	}
	return false
}

func clientIP(request *http.Request, proxies []netip.Prefix) (netip.Addr, error) {
	host, _, err := net.SplitHostPort(request.RemoteAddr)
	if err != nil {
		return netip.Addr{}, errors.New("invalid remote address")
	}
	peer, err := netip.ParseAddr(host)
	if err != nil || peer.Zone() != "" {
		return netip.Addr{}, errors.New("invalid remote IP")
	}
	peer = peer.Unmap()
	if !trusted(peer, proxies) {
		return peer, nil
	}
	forwarded := strings.Join(request.Header.Values("X-Forwarded-For"), ",")
	if forwarded == "" {
		return peer, nil
	}
	parts := strings.Split(forwarded, ",")
	if len(forwarded) > 4096 || len(parts) > 32 {
		return netip.Addr{}, errors.New("forwarded IP chain too large")
	}
	chain := make([]netip.Addr, len(parts))
	for index, part := range parts {
		ip, err := netip.ParseAddr(strings.TrimSpace(part))
		if err != nil || ip.Zone() != "" {
			return netip.Addr{}, errors.New("invalid forwarded IP")
		}
		chain[index] = ip.Unmap()
	}
	for index := len(chain) - 1; index >= 0 && trusted(peer, proxies); index-- {
		peer = chain[index]
	}
	return peer, nil
}

func (api *authHTTP) protect(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		if r.Method != http.MethodGet && r.Method != http.MethodHead && r.Method != http.MethodOptions {
			origins, csrf := r.Header.Values("Origin"), r.Header.Values("X-PT-CSRF")
			if len(origins) != 1 || origins[0] != api.origin || len(csrf) != 1 || csrf[0] != "1" {
				api.fail(w, r, apperr.New(403, "ERR_CSRF", "CSRF header and matching Origin required"))
				return
			}
		}
		ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
		defer cancel()
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func (api *authHTTP) cookie(w http.ResponseWriter, token string, expires time.Time) {
	maxAge := int(time.Until(expires).Seconds())
	if maxAge < 1 {
		maxAge = 1
	}
	http.SetCookie(w, &http.Cookie{Name: sessionCookie, Value: token, Path: "/",
		HttpOnly: true, Secure: api.secure, SameSite: http.SameSiteLaxMode,
		Expires: expires.UTC(), MaxAge: maxAge})
}

func (api *authHTTP) clearCookie(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{Name: sessionCookie, Path: "/", HttpOnly: true,
		Secure: api.secure, SameSite: http.SameSiteLaxMode, MaxAge: -1, Expires: time.Unix(1, 0).UTC()})
}

func requestToken(request *http.Request) (string, error) {
	var token string
	count := 0
	for _, cookie := range request.Cookies() {
		if cookie.Name == sessionCookie {
			count++
			token = cookie.Value
		}
	}
	if count != 1 {
		return "", apperr.New(401, "ERR_UNAUTHENTICATED", "Authentication required")
	}
	return token, nil
}

func (api *authHTTP) session(w http.ResponseWriter, r *http.Request, allowForced bool) (auth.Session, string, bool) {
	token, err := requestToken(r)
	if err != nil {
		api.fail(w, r, err)
		return auth.Session{}, "", false
	}
	session, err := api.service.Authenticate(r.Context(), token)
	if err != nil {
		api.fail(w, r, err)
		return auth.Session{}, "", false
	}
	if session.User.MustChangePassword && !allowForced {
		api.fail(w, r, apperr.New(403, "ERR_PASSWORD_CHANGE_REQUIRED", "Password change required"))
		return auth.Session{}, "", false
	}
	return session, token, true
}

func (api *authHTTP) fail(w http.ResponseWriter, r *http.Request, err error) {
	var app *apperr.Error
	if !errors.As(err, &app) {
		// Driver/hash errors can contain sensitive values; log only correlation.
		api.logger.ErrorContext(r.Context(), "auth operation failed", "request_id", middleware.GetReqID(r.Context()))
		app = apperr.New(500, "ERR_INTERNAL", "Internal server error")
	}
	if app.Code == "ERR_RATE_LIMITED" {
		w.Header().Set("Retry-After", "60")
	} else if app.Code == "ERR_AUTH_BUSY" {
		w.Header().Set("Retry-After", "1")
	}
	writeJSON(w, app.Status, struct {
		Error *apperr.Error `json:"error"`
	}{app})
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func decodeJSON(w http.ResponseWriter, r *http.Request, value any) error {
	return decodeJSONLimit(w, r, value, 4096)
}

// decodeJSONLimit is decodeJSON with an explicit body bound; only field bodies
// use a larger one (D-125).
func decodeJSONLimit(w http.ResponseWriter, r *http.Request, value any, limit int64) error {
	contentType, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err != nil || contentType != "application/json" {
		return apperr.New(415, "ERR_CONTENT_TYPE", "Content-Type must be application/json")
	}
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, limit))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(value); err != nil {
		return apperr.New(400, "ERR_VALIDATION", "Invalid JSON request")
	}
	var extra any
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		return apperr.New(400, "ERR_VALIDATION", "Request must contain one JSON value")
	}
	return nil
}

func (api *authHTTP) mount(router chi.Router) {
	router.Use(api.protect)
	router.Get("/setup/status", api.setupStatus)
	router.Post("/setup", api.setup)
	router.Post("/auth/login", api.login)
	router.Post("/auth/logout", api.logout)
	router.Get("/auth/me", api.me)
	router.Patch("/auth/me", api.profile)
	router.Post("/auth/password", api.password)
	router.Get("/meta/timezones", api.timezones)
	if api.acl != nil {
		api.mountGrants(router)
	}
	if api.structure != nil {
		api.mountStructure(router)
	}
	if api.admin != nil {
		router.Get("/meta/app", api.appSettings)
		router.Get("/meta/currencies", api.currencies)
		api.mountAdmin(router)
	}
}

func (api *authHTTP) setupStatus(w http.ResponseWriter, r *http.Request) {
	needed, err := api.service.SetupNeeded(r.Context())
	if err != nil {
		api.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, struct {
		Needed bool `json:"needed"`
	}{needed})
}

func (api *authHTTP) setup(w http.ResponseWriter, r *http.Request) {
	ip, err := clientIP(r, api.proxies)
	if err != nil {
		api.fail(w, r, apperr.New(400, "ERR_VALIDATION", "Invalid client IP chain"))
		return
	}
	var input struct {
		Token       string `json:"token"`
		Username    string `json:"username"`
		Email       string `json:"email"`
		DisplayName string `json:"displayName"`
		Password    string `json:"password"`
		Locale      string `json:"locale"`
	}
	if err := decodeJSON(w, r, &input); err != nil {
		api.fail(w, r, err)
		return
	}
	session, token, err := api.service.Setup(r.Context(), auth.SetupInput{
		Token: input.Token, Username: input.Username, Email: input.Email,
		DisplayName: input.DisplayName, Password: input.Password, Locale: input.Locale,
	}, ip, r.UserAgent())
	if err != nil {
		api.fail(w, r, err)
		return
	}
	api.cookie(w, token, session.ExpiresAt)
	writeJSON(w, http.StatusOK, struct {
		User    auth.User `json:"user"`
		EventID string    `json:"eventId"`
	}{session.User, rand.Text()})
}

func (api *authHTTP) login(w http.ResponseWriter, r *http.Request) {
	ip, err := clientIP(r, api.proxies)
	if err != nil {
		api.fail(w, r, apperr.New(400, "ERR_VALIDATION", "Invalid client IP chain"))
		return
	}
	var input struct {
		Login    string `json:"login"`
		Password string `json:"password"`
	}
	if err := decodeJSON(w, r, &input); err != nil {
		if rateErr := api.service.CheckLoginIP(ip); rateErr != nil {
			err = rateErr
		}
		api.fail(w, r, err)
		return
	}
	session, token, err := api.service.Login(r.Context(), input.Login, input.Password, ip, r.UserAgent())
	if err != nil {
		api.fail(w, r, err)
		return
	}
	api.cookie(w, token, session.ExpiresAt)
	writeJSON(w, 200, struct {
		User    auth.User `json:"user"`
		EventID string    `json:"eventId"`
	}{session.User, rand.Text()})
}

func (api *authHTTP) logout(w http.ResponseWriter, r *http.Request) {
	token, err := requestToken(r)
	if err == nil {
		err = api.service.Logout(r.Context(), token)
	}
	if err != nil {
		api.fail(w, r, err)
		return
	}
	api.clearCookie(w)
	writeJSON(w, 200, map[string]string{"eventId": rand.Text()})
}

func (api *authHTTP) me(w http.ResponseWriter, r *http.Request) {
	session, token, ok := api.session(w, r, true)
	if !ok {
		return
	}
	api.cookie(w, token, session.ExpiresAt)
	writeJSON(w, 200, session.User)
}

func (api *authHTTP) password(w http.ResponseWriter, r *http.Request) {
	var input struct {
		Current string `json:"current"`
		New     string `json:"new"`
	}
	if err := decodeJSON(w, r, &input); err != nil {
		api.fail(w, r, err)
		return
	}
	token, err := requestToken(r)
	if err == nil {
		err = api.service.ChangePassword(r.Context(), token, input.Current, input.New)
	}
	if err != nil {
		api.fail(w, r, err)
		return
	}
	api.clearCookie(w)
	writeJSON(w, 200, map[string]string{"eventId": rand.Text()})
}

func patchString(raw json.RawMessage, nullable bool) (*string, error) {
	if len(raw) == 0 || nullable && string(raw) == "null" {
		return nil, nil
	}
	var value string
	if string(raw) == "null" || json.Unmarshal(raw, &value) != nil {
		return nil, apperr.New(400, "ERR_VALIDATION", "Invalid string or null value")
	}
	return &value, nil
}

func (api *authHTTP) profile(w http.ResponseWriter, r *http.Request) {
	var input struct {
		DisplayName json.RawMessage `json:"displayName"`
		Locale      json.RawMessage `json:"locale"`
		Theme       json.RawMessage `json:"theme"`
		Timezone    json.RawMessage `json:"timezone"`
	}
	if err := decodeJSON(w, r, &input); err != nil {
		api.fail(w, r, err)
		return
	}
	session, token, ok := api.session(w, r, false)
	if !ok {
		return
	}
	patch := auth.ProfilePatch{TimezoneSet: len(input.Timezone) != 0}
	for _, field := range []struct {
		raw      json.RawMessage
		dest     **string
		nullable bool
	}{{input.DisplayName, &patch.DisplayName, false}, {input.Locale, &patch.Locale, false},
		{input.Theme, &patch.Theme, false}, {input.Timezone, &patch.Timezone, true}} {
		value, err := patchString(field.raw, field.nullable)
		if err != nil {
			api.fail(w, r, err)
			return
		}
		*field.dest = value
	}
	user, err := api.service.UpdateProfile(r.Context(), token, patch)
	if err != nil {
		api.fail(w, r, err)
		return
	}
	api.cookie(w, token, session.ExpiresAt)
	writeJSON(w, 200, struct {
		User    auth.User `json:"user"`
		EventID string    `json:"eventId"`
	}{user, rand.Text()})
}

func (api *authHTTP) timezones(w http.ResponseWriter, r *http.Request) {
	session, token, ok := api.session(w, r, false)
	if !ok {
		return
	}
	names, err := api.service.Timezones(r.Context())
	if err != nil {
		api.fail(w, r, err)
		return
	}
	api.cookie(w, token, session.ExpiresAt)
	writeJSON(w, 200, struct {
		Items []string `json:"items"`
	}{names})
}
