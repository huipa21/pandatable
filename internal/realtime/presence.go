package realtime

import (
	"context"
	"errors"
	"log/slog"
	"sort"
	"strconv"
	"time"

	"github.com/jackc/pgx/v5"

	"pandatable/internal/acl"
	"pandatable/internal/events"
	"pandatable/internal/structure/structuresql"
)

const (
	presenceBroadcastInterval = 250 * time.Millisecond
	presenceTimeout           = 60 * time.Second
)

type presenceCell struct {
	RowID   string `json:"rowId"`
	FieldID string `json:"fieldId"`
}

type presenceEntry struct {
	viewID   string
	cell     *presenceCell
	lastSeen time.Time
	activity uint64
}

type presenceTable struct {
	entries       map[*client]presenceEntry
	lastBroadcast time.Time
	dirty         bool
}

type presenceUser struct {
	UserID string        `json:"userId"`
	Name   string        `json:"name"`
	Cell   *presenceCell `json:"cell,omitempty"`
}

type presenceSnapshot struct {
	Type    string         `json:"t"`
	TableID string         `json:"tableId"`
	ViewID  string         `json:"viewId,omitempty"`
	Users   []presenceUser `json:"users"`
}

func normalizedPresence(message incoming) (string, *presenceCell, bool) {
	viewID := ""
	if message.ViewID != "" {
		view, ok := acl.ParseUUID(message.ViewID)
		if !ok {
			return "", nil, false
		}
		viewID = view.String()
	}
	if message.Cell == nil {
		return viewID, nil, true
	}
	if viewID == "" {
		return "", nil, false
	}
	field, ok := acl.ParseUUID(message.Cell.FieldID)
	if !ok {
		return "", nil, false
	}
	row, err := strconv.ParseInt(message.Cell.RowID, 10, 64)
	if err != nil || row <= 0 || strconv.FormatInt(row, 10) != message.Cell.RowID {
		return "", nil, false
	}
	return viewID, &presenceCell{RowID: message.Cell.RowID, FieldID: field.String()}, true
}

func (c *client) reportPresence(message incoming) {
	viewID, cell, valid := normalizedPresence(message)
	if !valid || !c.subscribed(message.TableID) {
		c.send(events.Event{Type: "error", TableID: message.TableID, Code: "ERR_VALIDATION"}, false)
		return
	}
	if !c.checkAccess(message.TableID) {
		return
	}
	if !c.checkPresenceView(message.TableID, viewID) {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if _, ok := c.subs[message.TableID]; !ok || c.ctx.Err() != nil {
		return
	}
	c.hub.setPresence(c, message.TableID, viewID, cell, time.Now())
}

func (c *client) checkPresenceView(tableID, viewID string) bool {
	if viewID == "" {
		return true
	}
	table, tableOK := acl.ParseUUID(tableID)
	viewIDValue, viewOK := acl.ParseUUID(viewID)
	if !tableOK || !viewOK {
		c.send(events.Event{Type: "error", TableID: tableID, Code: "ERR_VALIDATION"}, false)
		return false
	}
	ctx, cancel := context.WithTimeout(c.ctx, operationTimeout)
	view, err := structuresql.New(c.hub.pool).GetViewForQuery(ctx, viewIDValue)
	cancel()
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			c.send(events.Event{Type: "error", TableID: tableID, Code: "ERR_NOT_FOUND"}, false)
		} else if c.ctx.Err() == nil {
			c.hub.logger.Log(c.ctx, slog.LevelError, "WebSocket presence view check failed", "user_id", c.user.ID, "error", err)
			c.cancel()
		}
		return false
	}
	if view.TableID != table ||
		view.Scope != "shared" && (view.Scope != "personal" || !view.OwnerID.Valid || view.OwnerID.String() != c.user.ID) {
		c.send(events.Event{Type: "error", TableID: tableID, Code: "ERR_NOT_FOUND"}, false)
		return false
	}
	return true
}

func samePresenceCell(a, b *presenceCell) bool {
	return a == nil && b == nil || a != nil && b != nil && *a == *b
}

func (h *Hub) setPresence(c *client, tableID, viewID string, cell *presenceCell, now time.Time) {
	h.mu.Lock()
	table := h.presence[tableID]
	if table == nil {
		table = &presenceTable{entries: make(map[*client]presenceEntry)}
		h.presence[tableID] = table
	}
	previous, found := table.entries[c]
	changed := !found || previous.viewID != viewID || !samePresenceCell(previous.cell, cell)
	if changed {
		h.presenceSeq++
		previous.activity = h.presenceSeq
		table.dirty = true
	}
	previous.viewID, previous.cell, previous.lastSeen = viewID, cell, now
	table.entries[c] = previous
	h.mu.Unlock()
	h.wakePresence()
}

func (h *Hub) removePresence(c *client, tableID string) {
	h.mu.Lock()
	table := h.presence[tableID]
	removed := false
	if table != nil {
		if _, removed = table.entries[c]; removed {
			delete(table.entries, c)
			table.dirty = true
		}
	}
	h.mu.Unlock()
	if removed {
		h.wakePresence()
	}
}

func (h *Hub) wakePresence() {
	select {
	case h.presenceWake <- struct{}{}:
	default:
	}
}

func nextExpiry(table *presenceTable) time.Time {
	var earliest time.Time
	for _, entry := range table.entries {
		expiry := entry.lastSeen.Add(presenceTimeout)
		if earliest.IsZero() || expiry.Before(earliest) {
			earliest = expiry
		}
	}
	return earliest
}

func expirePresence(table *presenceTable, now time.Time) {
	for c := range table.entries {
		if c.ctx.Err() != nil {
			delete(table.entries, c)
			table.dirty = true
		}
	}
	if expiry := nextExpiry(table); !expiry.IsZero() && !expiry.After(now) {
		// Expirations within one broadcast interval share the same snapshot.
		cutoff := now.Add(presenceBroadcastInterval)
		for c, entry := range table.entries {
			if !entry.lastSeen.Add(presenceTimeout).After(cutoff) {
				delete(table.entries, c)
				table.dirty = true
			}
		}
	}
}

func nextBroadcast(table *presenceTable, now time.Time) time.Time {
	at := now
	if allowed := table.lastBroadcast.Add(presenceBroadcastInterval); allowed.After(at) {
		at = allowed
	}
	if expiry := nextExpiry(table); expiry.After(at) && expiry.Sub(at) < presenceBroadcastInterval {
		at = expiry
	}
	return at
}

func earlier(current, candidate time.Time) time.Time {
	if candidate.IsZero() || !current.IsZero() && !candidate.Before(current) {
		return current
	}
	return candidate
}

func (h *Hub) duePresence(now time.Time) ([]string, time.Time) {
	h.mu.Lock()
	defer h.mu.Unlock()
	var due []string
	var next time.Time
	for id, table := range h.presence {
		expirePresence(table, now)
		if table.dirty {
			at := nextBroadcast(table, now)
			if !at.After(now) {
				due = append(due, id)
			} else {
				next = earlier(next, at)
			}
		}
		if expiry := nextExpiry(table); !expiry.IsZero() {
			next = earlier(next, expiry)
		} else if !table.dirty {
			cleanup := table.lastBroadcast.Add(presenceBroadcastInterval)
			if !cleanup.After(now) {
				delete(h.presence, id)
			} else {
				next = earlier(next, cleanup)
			}
		}
	}
	sort.Strings(due)
	return due, next
}

func (h *Hub) runPresence() {
	defer close(h.presenceDone)
	timer := time.NewTimer(time.Hour)
	if !timer.Stop() {
		<-timer.C
	}
	defer timer.Stop()
	for {
		due, next := h.duePresence(time.Now())
		if len(due) > 0 {
			for _, tableID := range due {
				h.broadcastPresence(tableID)
			}
			continue
		}
		var timerC <-chan time.Time
		if !next.IsZero() {
			timer.Reset(max(time.Until(next), 0))
			timerC = timer.C
		}
		select {
		case <-h.ctx.Done():
			return
		case <-h.presenceWake:
		case <-timerC:
		}
		if timerC != nil && !timer.Stop() {
			select {
			case <-timer.C:
			default:
			}
		}
	}
}

func (h *Hub) broadcastPresence(tableID string) {
	recipients := make([]*client, 0)
	authorized := make(map[*client]bool)
	for _, c := range h.snapshot() {
		if c.ctx.Err() == nil && c.subscribed(tableID) && c.checkAccess(tableID) {
			authorized[c] = true
			recipients = append(recipients, c)
		}
	}

	h.mu.Lock()
	table := h.presence[tableID]
	if table == nil {
		h.mu.Unlock()
		return
	}
	now := time.Now()
	expirePresence(table, now)
	if !table.dirty || nextBroadcast(table, now).After(now) {
		h.mu.Unlock()
		return
	}
	latest := make(map[string]presenceEntry)
	owners := make(map[string]*client)
	views := make(map[*client]string)
	for c, entry := range table.entries {
		if !authorized[c] {
			continue
		}
		views[c] = entry.viewID
		if previous, ok := latest[c.user.ID]; !ok || entry.activity > previous.activity {
			latest[c.user.ID] = entry
			owners[c.user.ID] = c
		}
	}
	ids := make([]string, 0, len(latest))
	for id := range latest {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	table.dirty = false
	table.lastBroadcast = now
	h.mu.Unlock()

	for _, c := range recipients {
		if c.ctx.Err() != nil || !c.subscribed(tableID) {
			continue
		}
		viewID := views[c]
		users := make([]presenceUser, 0, len(ids))
		for _, id := range ids {
			entry := latest[id]
			user := presenceUser{UserID: id, Name: owners[id].user.DisplayName}
			if viewID != "" && entry.viewID == viewID {
				user.Cell = entry.cell
			}
			users = append(users, user)
		}
		c.enqueue(outgoing{presence: &presenceSnapshot{
			Type: "presence", TableID: tableID, ViewID: viewID, Users: users,
		}})
	}
}
