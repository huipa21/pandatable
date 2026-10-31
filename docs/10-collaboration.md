# 10 — Collaboration

## 1. Realtime (FR-C1, FR-C5)

```
handler tx ──► pg_notify('pt_events', '{"t":"row.updated","tableId":"…","ids":[…],"fields":[…],"by":"…","eventId":"…"}')
                     │ (delivered on commit only)
               LISTEN conn (1 dedicated pgx conn) ──► hub ──► subscribed WS clients
```

- NOTIFY payload stays below 7,900 bytes, with at most 500 row IDs. Larger
  events become `{"t":"row.bulk","tableId":"…"}`, meaning "reload view".
  Notifications are part of the mutation transaction and cannot escape a
  rollback.
- Clients batch-refetch visible affected rows via `POST /views/{id}/query`
  with an `ids` filter (at most 100 per request), merge them, and invalidate
  offscreen pages and potentially stale in-flight page reads.
- Echo suppression: each tab preallocates an `eventId` before a mutation and
  ignores matching events only when their actor is the current user (D-183).
- Subscription and delivery both enforce table read ACLs. Personal-view
  changes go only to their owner; sanitized `nav.changed` invalidations refresh
  authorized sidebars and trigger subscription rechecks. Disabled or revoked
  sessions receive `session.revoked`.
- Related row-value changes invalidate dependent tables for clients allowed to
  read both tables; the invalidation omits the source and actor. Type changes
  also notify tables whose computed-field schema version changes.
- **Conflict policy:** last write wins per cell. PATCH carries only changed fields, so concurrent edits of different cells in the same row never clash. If a remote update hits the cell the local user is currently editing, the editor shows a small "changed by X" badge; on save, local value wins (LWW) — user can press Esc to take the remote value.
- WS heartbeat 25 s; reconnect with exponential backoff (1 s → 30 s), on reconnect client resubscribes and reloads visible window (no event replay needed).

## 2. Presence (FR-C2)

- In-memory only (single instance). Client sends `presence` on table open, view switch, and cell focus change (throttled 200 ms).
- Hub broadcasts presence per table, throttled to 4/s.
- UI: avatar stack in table header; colored outline + name tag on cell another user edits.
- Stale presence removed after 60 s without heartbeat.

## 3. Comments & notifications (FR-C3)

- Comments per row (plain text + @mentions, rendered with links). Mention autocomplete via `/directory/users`, limited to users with read access to the table (others are shown as not-notified).
- Mention → `meta.notifications` row + `notification` WS event. Bell icon with unread count; clicking opens the row expand modal at the comment.
- Comment edit/delete by author; owners of the table can delete any comment. Row cells show a comment count badge (optional column indicator).
- Deleted rows: comments kept until the row's table is purged (restorable via undo).

## 4. Row history (FR-C4)

- Captured in the same transaction as the write using PG18 `RETURNING OLD.*, NEW.*` (or OLD/NEW for specific columns).
- `changes` jsonb: `{ "<fieldId>": { "o": <old>, "n": <new> } }` only for changed fields; inserts store `n` only, deletes store full `o` snapshot.
- Not recorded: system fields, virtual fields (formula/lookup/rollup).
- Field conversion records one table-level `type_change` event with NULL
  `row_id` (D-071), not one event per converted row. All row actions require a
  row ID; metadata-table purge cascades history, while physical row deletion
  does not. This schema exists now; capture/retention jobs remain later work.
- Link changes recorded on both rows as `link`/`unlink` with the other row's id + display value.
- Bulk paste over N rows → N history rows (one per row) in one INSERT.
- Retention: daily job deletes history older than `history_retention_days` (default 90).
- UI: "History" tab in row expand modal — timeline grouped by user + minute.
- This is **not** an audit log: not tamper-proof, no login/admin/action logs (out of scope).

## 5. Undo / redo (FR-G6)

Client-side per tab, last 50 actions. Each action stores an inverse operation:

| Action | Inverse |
|---|---|
| Cell edit / paste / fill-down | Bulk PATCH with previous values (captured from server response / local snapshot) |
| Insert rows | DELETE those ids |
| Delete rows | `rows:restore` with server-returned snapshots (same ids, incl. links & attachments refs) |
| Link/unlink | opposite link call |
| View config change | PATCH previous config |

Schema changes (fields/tables) are **not** undoable (use Trash). If a cell was changed by someone else after our edit, undo of that cell asks for confirmation ("X changed it since; overwrite?") — detected by comparing current value with our post-edit value.

## 6. Trash

Soft-deleted workspace/book/table/field/view appear in the scope's Trash (owner only), restorable for 30 days (setting). Rows are not trashed (undo + history cover them).
