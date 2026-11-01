package httpapi

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"

	"pandatable/internal/acl"
	"pandatable/internal/apperr"
	"pandatable/internal/auth"
	"pandatable/internal/structure"
)

const (
	exportTimeout      = 10 * time.Minute
	exportTicketTTL    = 2 * time.Minute
	maxExportTickets   = 128
	maxUserTickets     = 4
	exportRequestLimit = 128 << 10
	xlsxContentType    = "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet"
)

type exportTicket struct {
	userID  string
	session [sha256.Size]byte
	viewID  string
	request structure.ExportRequest
	expires time.Time
}

type exportTickets struct {
	mu    sync.Mutex
	items map[string]exportTicket
}

func newExportTickets() *exportTickets {
	return &exportTickets{items: make(map[string]exportTicket)}
}

func (tickets *exportTickets) issue(ticket exportTicket, now time.Time) (string, error) {
	tickets.mu.Lock()
	defer tickets.mu.Unlock()
	userCount := 0
	for token, existing := range tickets.items {
		if !existing.expires.After(now) {
			delete(tickets.items, token)
		} else if existing.userID == ticket.userID {
			userCount++
		}
	}
	if len(tickets.items) >= maxExportTickets || userCount >= maxUserTickets {
		return "", apperr.New(429, "ERR_RATE_LIMITED", "Too many pending exports")
	}
	ticket.expires = now.Add(exportTicketTTL)
	for {
		token := rand.Text()
		if _, exists := tickets.items[token]; !exists {
			tickets.items[token] = ticket
			return token, nil
		}
	}
}

func (tickets *exportTickets) consume(token, userID string, session [sha256.Size]byte, now time.Time) (exportTicket, error) {
	tickets.mu.Lock()
	defer tickets.mu.Unlock()
	ticket, exists := tickets.items[token]
	if !exists {
		return exportTicket{}, acl.ErrNotFound()
	}
	if !ticket.expires.After(now) {
		delete(tickets.items, token)
		return exportTicket{}, acl.ErrNotFound()
	}
	if ticket.userID != userID || subtle.ConstantTimeCompare(ticket.session[:], session[:]) != 1 {
		return exportTicket{}, acl.ErrNotFound()
	}
	delete(tickets.items, token)
	return ticket, nil
}

func (api *authHTTP) prepareExport(w http.ResponseWriter, r *http.Request, user auth.User) {
	if err := noQuery(r); err != nil {
		api.fail(w, r, err)
		return
	}
	var input structure.ExportRequest
	if err := decodeJSONLimit(w, r, &input, exportRequestLimit); err != nil {
		api.fail(w, r, err)
		return
	}
	viewID := chi.URLParam(r, "id")
	if err := api.structure.CheckExport(r.Context(), actorOf(user), viewID, input, api.version); err != nil {
		api.fail(w, r, err)
		return
	}
	session, err := requestToken(r)
	if err != nil {
		api.fail(w, r, err)
		return
	}
	ticket, err := api.exports.issue(exportTicket{
		userID: user.ID, session: sha256.Sum256([]byte(session)), viewID: viewID, request: input,
	}, time.Now())
	if err != nil {
		api.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, struct {
		Ticket string `json:"ticket"`
	}{Ticket: ticket})
}

func (api *authHTTP) downloadExport(w http.ResponseWriter, r *http.Request, user auth.User) {
	if err := noQuery(r); err != nil {
		api.fail(w, r, err)
		return
	}
	session, err := requestToken(r)
	if err != nil {
		api.fail(w, r, err)
		return
	}
	ticket, err := api.exports.consume(chi.URLParam(r, "ticket"), user.ID, sha256.Sum256([]byte(session)), time.Now())
	if err != nil {
		api.fail(w, r, err)
		return
	}
	if err := http.NewResponseController(w).SetWriteDeadline(time.Now().Add(exportTimeout)); err != nil &&
		!errors.Is(err, http.ErrNotSupported) {
		api.fail(w, r, fmt.Errorf("set export write deadline: %w", err))
		return
	}
	writer := &exportResponseWriter{ResponseWriter: w}
	err = api.structure.Export(r.Context(), actorOf(user), ticket.viewID, ticket.request, api.version,
		func(info structure.ExportInfo, write func(io.Writer) error) error {
			contentType, extension := "text/csv; charset=utf-8", ".csv"
			if info.Format == structure.ExportPackage {
				contentType, extension = xlsxContentType, ".xlsx"
			}
			name := strings.NewReplacer("/", "_", `\`, "_").Replace(info.Name) + extension
			disposition := mime.FormatMediaType("attachment", map[string]string{"filename": name})
			if disposition == "" {
				return errors.New("export filename cannot be encoded")
			}
			w.Header().Set("Content-Type", contentType)
			w.Header().Set("Content-Disposition", disposition)
			writer.started = true
			w.WriteHeader(http.StatusOK)
			if err := http.NewResponseController(w).Flush(); err != nil && !errors.Is(err, http.ErrNotSupported) {
				return fmt.Errorf("start export stream: %w", err)
			}
			return write(writer)
		})
	if err == nil {
		return
	}
	if !writer.started {
		w.Header().Del("Content-Disposition")
		api.fail(w, r, err)
		return
	}
	api.logger.ErrorContext(r.Context(), "export stream failed", "request_id", middleware.GetReqID(r.Context()))
	panic(http.ErrAbortHandler)
}

type exportResponseWriter struct {
	http.ResponseWriter
	started bool
}

func (writer *exportResponseWriter) Write(p []byte) (int, error) {
	writer.started = true
	return writer.ResponseWriter.Write(p)
}
