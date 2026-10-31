package realtime

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"sync"
	"time"

	"github.com/coder/websocket"
	"github.com/coder/websocket/wsjson"

	"pandatable/internal/acl"
	"pandatable/internal/apperr"
	"pandatable/internal/auth"
	"pandatable/internal/events"
)

const (
	heartbeatInterval = 25 * time.Second
	sessionInterval   = 4 * time.Second
	operationTimeout  = 5 * time.Second
)

type outgoing struct {
	event    events.Event
	presence *presenceSnapshot
	close    bool
}

type client struct {
	hub    *Hub
	conn   *websocket.Conn
	user   auth.User
	token  string
	ctx    context.Context
	cancel context.CancelFunc
	out    chan outgoing
	mu     sync.Mutex
	subs   map[string]struct{}
}

type incoming struct {
	Type    string        `json:"t"`
	TableID string        `json:"tableId"`
	ViewID  string        `json:"viewId"`
	Cell    *presenceCell `json:"cell"`
}

// Serve authenticates before calling this method; origin is checked both here
// and by the WebSocket library.
func (h *Hub) Serve(w http.ResponseWriter, r *http.Request, user auth.User, token, origin string) {
	origins := r.Header.Values("Origin")
	if len(origins) != 1 || origins[0] != origin {
		socketError(w, http.StatusForbidden, "ERR_CSRF")
		return
	}
	h.mu.Lock()
	ready := h.ready
	h.mu.Unlock()
	if !ready {
		socketError(w, http.StatusServiceUnavailable, "ERR_SYSTEM_UNAVAILABLE")
		return
	}
	conn, err := websocket.Accept(w, r, &websocket.AcceptOptions{OriginPatterns: []string{origin}})
	if err != nil {
		h.logger.Warn("WebSocket upgrade failed", "error", err)
		return
	}
	conn.SetReadLimit(4096)
	h.mu.Lock()
	if !h.ready {
		h.mu.Unlock()
		_ = conn.Close(websocket.StatusTryAgainLater, "Realtime listener unavailable")
		return
	}
	ctx, cancel := context.WithCancel(h.ctx)
	c := &client{hub: h, conn: conn, user: user, token: token, ctx: ctx, cancel: cancel,
		out: make(chan outgoing, 64), subs: make(map[string]struct{})}
	h.clients[c] = struct{}{}
	h.mu.Unlock()

	done := make(chan struct{})
	go func() {
		defer close(done)
		c.writeLoop()
	}()
	c.readLoop()
	cancel()
	<-done
	_ = conn.CloseNow()
	h.mu.Lock()
	delete(h.clients, c)
	for _, table := range h.presence {
		if _, ok := table.entries[c]; ok {
			delete(table.entries, c)
			table.dirty = true
		}
	}
	h.mu.Unlock()
	h.wakePresence()
}

func socketError(w http.ResponseWriter, status int, code string) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]any{"error": map[string]string{"code": code}})
}

func (c *client) send(event events.Event, closeAfter bool) {
	event.Recipient = ""
	event.SourceTableID = ""
	c.enqueue(outgoing{event: event, close: closeAfter})
}

func (c *client) enqueue(message outgoing) {
	select {
	case c.out <- message:
	case <-c.ctx.Done():
	default:
		c.hub.logger.Warn("WebSocket client fell behind; reconnect required", "user_id", c.user.ID)
		c.cancel()
	}
}

func (c *client) readLoop() {
	for {
		var message incoming
		if err := wsjson.Read(c.ctx, c.conn, &message); err != nil {
			status := websocket.CloseStatus(err)
			if c.ctx.Err() == nil && status != websocket.StatusNormalClosure && status != websocket.StatusGoingAway {
				c.hub.logger.Warn("WebSocket read failed", "user_id", c.user.ID, "error", err)
			}
			return
		}
		switch message.Type {
		case "ping":
			c.send(events.Event{Type: "pong"}, false)
		case "sub":
			if c.checkAccess(message.TableID) {
				c.mu.Lock()
				c.subs[message.TableID] = struct{}{}
				c.mu.Unlock()
				c.send(events.Event{Type: "subscribed", TableID: message.TableID}, false)
			}
		case "unsub":
			c.unsubscribe(message.TableID)
		case "presence":
			c.reportPresence(message)
		default:
			c.send(events.Event{Type: "error", TableID: message.TableID, Code: "ERR_UNSUPPORTED"}, false)
		}
	}
}

func (c *client) writeLoop() {
	heartbeat := time.NewTicker(heartbeatInterval)
	sessions := time.NewTicker(sessionInterval)
	defer heartbeat.Stop()
	defer sessions.Stop()
	for {
		select {
		case <-c.ctx.Done():
			return
		case msg := <-c.out:
			ctx, cancel := context.WithTimeout(c.ctx, operationTimeout)
			var payload any = msg.event
			if msg.presence != nil {
				payload = msg.presence
			}
			err := wsjson.Write(ctx, c.conn, payload)
			cancel()
			if err != nil {
				if c.ctx.Err() == nil {
					c.hub.logger.Warn("WebSocket write failed", "user_id", c.user.ID, "error", err)
				}
				c.cancel()
				return
			}
			if msg.close {
				_ = c.conn.Close(websocket.StatusPolicyViolation, "Session revoked")
				c.cancel()
				return
			}
		case <-heartbeat.C:
			ctx, cancel := context.WithTimeout(c.ctx, operationTimeout)
			err := c.conn.Ping(ctx)
			cancel()
			if err != nil {
				if c.ctx.Err() == nil {
					c.hub.logger.Warn("WebSocket heartbeat failed", "user_id", c.user.ID, "error", err)
				}
				c.cancel()
				return
			}
		case <-sessions.C:
			c.checkSession()
		}
	}
}

func (c *client) checkSession() {
	ctx, cancel := context.WithTimeout(c.ctx, operationTimeout)
	defer cancel()
	if err := c.hub.sessions.CheckSession(ctx, c.token, c.user.ID); err != nil {
		if isUnauthenticated(err) {
			c.send(events.Event{Type: "session.revoked"}, true)
		} else if c.ctx.Err() == nil {
			c.hub.logger.Error("WebSocket session check failed", "user_id", c.user.ID, "error", err)
			c.cancel()
		}
	}
}

func (c *client) subscribed(tableID string) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	_, ok := c.subs[tableID]
	return ok
}

func (c *client) unsubscribe(tableID string) {
	c.mu.Lock()
	delete(c.subs, tableID)
	c.hub.removePresence(c, tableID)
	c.mu.Unlock()
}

func (c *client) checkAccess(tableID string) bool {
	return c.authorized(tableID, true)
}

func (c *client) authorized(tableID string, reportDenial bool) bool {
	resource, err := acl.NewResource("table", tableID)
	if err == nil {
		ctx, cancel := context.WithTimeout(c.ctx, operationTimeout)
		err = c.hub.acl.Require(ctx, c.user.ID, acl.Read, resource)
		cancel()
	}
	if err == nil {
		return true
	}
	var app *apperr.Error
	if errors.As(err, &app) {
		if reportDenial {
			c.unsubscribe(tableID)
			c.send(events.Event{Type: "sub.denied", TableID: tableID, Code: app.Code}, false)
		}
	} else if c.ctx.Err() == nil {
		c.hub.logger.Log(c.ctx, slog.LevelError, "WebSocket ACL check failed", "user_id", c.user.ID, "error", err)
		c.cancel()
	}
	return false
}

func (c *client) recheckSubscriptions() {
	c.mu.Lock()
	tables := make([]string, 0, len(c.subs))
	for tableID := range c.subs {
		tables = append(tables, tableID)
	}
	c.mu.Unlock()
	for _, tableID := range tables {
		c.checkAccess(tableID)
	}
}
