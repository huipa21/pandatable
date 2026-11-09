// Package realtime bridges committed PostgreSQL notifications to WebSocket clients.
package realtime

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/jackc/pgx/v5"

	"pandatable/internal/acl"
	"pandatable/internal/apperr"
	"pandatable/internal/auth"
	"pandatable/internal/db"
	"pandatable/internal/events"
)

type authorizer interface {
	Require(context.Context, string, acl.Level, acl.Resource) error
	Invalidate()
}

type sessionChecker interface {
	CheckSession(context.Context, string, string) error
}

type Hub struct {
	pool     *db.Pool
	acl      authorizer
	sessions sessionChecker
	logger   *slog.Logger

	mu           sync.Mutex
	clients      map[*client]struct{}
	presence     map[string]*presenceTable
	presenceSeq  uint64
	presenceWake chan struct{}
	presenceDone chan struct{}
	ready        bool
	ctx          context.Context
	cancel       context.CancelFunc
	done         chan struct{}
}

func New(pool *db.Pool, resolver *acl.Resolver, sessions *auth.Service, logger *slog.Logger) (*Hub, error) {
	if pool == nil || pool.Pool == nil || resolver == nil || sessions == nil || logger == nil {
		return nil, errors.New("realtime hub requires database, ACL, sessions and logger")
	}
	return &Hub{
		pool: pool, acl: resolver, sessions: sessions, logger: logger,
		clients: make(map[*client]struct{}), presence: make(map[string]*presenceTable),
		presenceWake: make(chan struct{}, 1),
	}, nil
}

func (h *Hub) connect(ctx context.Context) (*pgx.Conn, error) {
	conn, err := pgx.ConnectConfig(ctx, h.pool.Config().ConnConfig.Copy())
	if err != nil {
		return nil, fmt.Errorf("connect realtime listener: %w", err)
	}
	if _, err := conn.Exec(ctx, "LISTEN "+events.Channel); err != nil {
		_ = conn.Close(context.Background())
		return nil, fmt.Errorf("listen for realtime events: %w", err)
	}
	return conn, nil
}

// Start establishes LISTEN before accepting sockets, so the first subscription
// cannot race listener startup.
func (h *Hub) Start(ctx context.Context) error {
	conn, err := h.connect(ctx)
	if err != nil {
		return err
	}
	h.mu.Lock()
	if h.done != nil {
		h.mu.Unlock()
		_ = conn.Close(context.Background())
		return errors.New("realtime hub is already started")
	}
	h.ctx, h.cancel = context.WithCancel(ctx)
	h.done = make(chan struct{})
	h.presenceDone = make(chan struct{})
	h.ready = true
	h.mu.Unlock()
	go h.listen(conn)
	go h.runPresence()
	return nil
}

func (h *Hub) listen(conn *pgx.Conn) {
	defer close(h.done)
	defer h.disconnectAll()
	defer func() {
		if conn == nil {
			return
		}
		closeCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := conn.Close(closeCtx); err != nil {
			h.logger.Error("realtime listener close failed", "error", err)
		}
	}()
	backoff := time.Second
	for {
		notification, err := conn.WaitForNotification(h.ctx)
		if err == nil {
			var event events.Event
			if err := json.Unmarshal([]byte(notification.Payload), &event); err != nil || event.Type == "" {
				h.logger.Error("invalid realtime notification")
				continue
			}
			h.deliver(event)
			continue
		}
		if h.ctx.Err() != nil {
			return
		}
		h.logger.Error("realtime listener stopped", "error", err)
		h.setReady(false)
		h.disconnectAll()
		closeCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		if closeErr := conn.Close(closeCtx); closeErr != nil {
			h.logger.Error("realtime listener close failed", "error", closeErr)
		}
		cancel()
		for {
			timer := time.NewTimer(backoff)
			select {
			case <-h.ctx.Done():
				timer.Stop()
				return
			case <-timer.C:
			}
			connectCtx, stop := context.WithTimeout(h.ctx, 5*time.Second)
			conn, err = h.connect(connectCtx)
			stop()
			if err == nil {
				h.setReady(true)
				h.logger.Info("realtime listener reconnected")
				backoff = time.Second
				break
			}
			h.logger.Error("realtime listener reconnect failed", "error", err)
			backoff = min(backoff*2, 30*time.Second)
		}
	}
}

func (h *Hub) setReady(ready bool) {
	h.mu.Lock()
	h.ready = ready
	h.mu.Unlock()
}

func (h *Hub) snapshot() []*client {
	h.mu.Lock()
	defer h.mu.Unlock()
	result := make([]*client, 0, len(h.clients))
	for c := range h.clients {
		result = append(result, c)
	}
	return result
}

func (h *Hub) disconnectAll() {
	for _, c := range h.snapshot() {
		c.cancel()
	}
}

func (h *Hub) Close() {
	h.mu.Lock()
	cancel, done, presenceDone := h.cancel, h.done, h.presenceDone
	h.ready = false
	h.mu.Unlock()
	if cancel != nil {
		cancel()
	}
	if done != nil {
		<-done
	}
	if presenceDone != nil {
		<-presenceDone
	}
}

func (h *Hub) deliver(event events.Event) {
	if event.Type == "nav.changed" {
		event.BookID = ""
		event.Description = nil
		h.acl.Invalidate()
	}
	if event.Type == "book.changed" {
		if _, valid := acl.ParseUUID(event.BookID); !valid || event.Description == nil {
			h.logger.Error("invalid realtime book notification")
			return
		}
	}
	for _, c := range h.snapshot() {
		if event.Recipient != "" && event.Recipient != c.user.ID {
			continue
		}
		if event.Type == "session.check" {
			c.checkSession()
			continue
		}
		if event.Type == "nav.changed" {
			c.send(event, false)
			c.recheckSubscriptions()
			continue
		}
		if event.Type == "book.changed" {
			if h.canReceiveResource(c, acl.Book, event.BookID) {
				c.send(event, false)
			}
			continue
		}
		if event.Type == "notification.changed" {
			if event.Recipient != "" {
				c.send(event, false)
			}
			continue
		}
		if event.Type == "notification" {
			if event.Recipient == "" || event.TableID == "" || !h.canReceiveResource(c, acl.Table, event.TableID) {
				continue
			}
			c.send(event, false)
			continue
		}
		if event.TableID == "" || !c.subscribed(event.TableID) {
			continue
		}
		if !c.checkAccess(event.TableID) {
			continue
		}
		if event.SourceTableID != "" && event.SourceTableID != event.TableID &&
			!c.authorized(event.SourceTableID, false) {
			continue
		}
		c.send(event, false)
	}
}

func (h *Hub) canReceiveResource(c *client, kind acl.ResourceType, id string) bool {
	resource, err := acl.NewResource(string(kind), id)
	user, valid := acl.ParseUUID(c.user.ID)
	if err != nil || !valid {
		return false
	}
	ctx, cancel := context.WithTimeout(c.ctx, operationTimeout)
	defer cancel()
	access, err := acl.AccessUncached(ctx, h.pool, user, resource)
	if err != nil {
		if c.ctx.Err() == nil {
			h.logger.Error("realtime resource ACL check failed", "user_id", c.user.ID, "error", err)
			c.cancel()
		}
		return false
	}
	return access.Check(acl.Read) == nil
}

func isUnauthenticated(err error) bool {
	var app *apperr.Error
	return errors.As(err, &app) && app.Code == "ERR_UNAUTHENTICATED"
}
