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
- Reports repeat every 20 s and are tied to the authenticated connection and
  table subscription. Unsubscribe, access denial, and disconnect remove that
  connection promptly. Concurrent tabs aggregate to one user; the most
  recently changed tab supplies focus.
- Reports validate that the view belongs to the subscribed table and is shared
  or owned by the sender. Snapshots are ACL-checked and contain only user IDs,
  display names, and same-view cell coordinates. Another user's view ID and
  cell values are never sent. A deadline-aware scheduler reserves expiry
  broadcasts and may group expirations up to 250 ms early to meet the 60 s
  bound without exceeding 4/s.

## 3. Comments & notifications (FR-C3)

- Comments are plain text, at most 4,096 UTF-8 bytes and 50 distinct
  `@username` mentions. Table readers may comment. Autocomplete uses the
  read-authorized `/tables/{id}/directory/mentions`, which returns only active
  users with effective read access; unmatched mentions are marked as not
  notified. Resolved mentions link to the comment deep link.
- Newly added eligible mentions other than the author create
  `meta.notifications` rows and recipient-only `notification` WebSocket events
  in the same transaction. The hub checks current access even if the recipient
  is not subscribed to the table. Removing a mention or deleting a comment
  removes its notifications; listings also hide revoked access and deleted
  comments.
- The bell shows the persisted unread count. Clicking resolves an accessible
  grid, opens the row modal with the comment focused and marks the notification
  read. Comment and notification lists load 50 at a time; a linked older
  comment is pinned first. Missing accessible grids are reported explicitly.
- Authors may edit or delete their comments; table owners may delete any
  comment. Desktop row headers and mobile cards show nonzero comment counts.
- Deleted rows: comments kept until the row's table is purged (restorable via undo).

## 4. Row history (FR-C4)

- Captured in the same transaction as the write using PG18 `RETURNING OLD.*, NEW.*` (or OLD/NEW for specific columns).
- `changes` jsonb: `{ "<fieldId>": { "o": <old>, "n": <new> } }` only for changed fields; inserts store `n` only, deletes store full `o` snapshot.
- Not recorded: system fields, virtual fields (formula/lookup/rollup).
- Field conversion records one table-level `type_change` event with NULL
  `row_id` (D-071), not one event per converted row. All row actions require a
  row ID; metadata-table purge cascades history, while physical row deletion
  does not. Type-change `o`/`n` values are the old/new UI types, keyed by field ID.
- Link changes are recorded on both rows as `link`/`unlink` with arrays of the
  other rows' IDs and display values, grouped per affected row and action.
  Replacement and deletion-triggered unlinking are included; deletion
  snapshots retain the logical links. No-op edits or links add no history.
- Bulk paste over N changed rows → N history rows (one per row) in one INSERT.
- The retention worker runs at startup and every 24 hours, using the current
  `history_retention_days` (default 90); it deletes entries strictly older than
  the cutoff and logs failures.
- UI: the "History" tab in the row expand modal groups the timeline by user
  and minute and refreshes through the existing realtime events.
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

T3.5 keeps at most 50 successful actions in memory per tab and clears redo
after a new action. Mutations use authoritative prior values and actual link
deltas; multi-part user actions remain one history entry. Guarded row patches
compare expected values under lock. After confirmation they compare against the
just-read value again, so a further race fails rather than overwriting silently;
redo restores the value actually overwritten. Oversized guarded patches are
split into bounded transactions, with compensation of completed steps on a
known failure. An uncertain result clears the stack and is reported to the user.
Delete inverses retain compact, versioned references to server-side snapshots;
history retention, incompatible schema/link changes, or cascades above the
restore limit can make a later undo unavailable.

## 6. Trash

Soft-deleted workspace/book/table/field/view appear in the scope's Trash (owner only), restorable for 30 days (setting). Rows are not trashed (undo + history cover them).
