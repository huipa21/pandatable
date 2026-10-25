# 06 — Permissions

## 1. Two independent axes

| Axis | Values | Controls |
|---|---|---|
| **App role** (on user) | `admin`, `user` | Access to Admin Panel only. Gives **no** data access by itself. |
| **Resource grant** | `read` < `write` < `owner` | Access to workspace/book/table and everything beneath. |

## 2. Grants

- Grant = (resource: workspace|book|table, principal: user|group, level).
- One grant per (resource, principal) pair; updating replaces level.
- Groups are flat (no nested groups).

## 3. Effective permission algorithm

```
effective(user, table T in book B in workspace W):
    principals = {user} ∪ groups(user)
    candidates = grants where principal ∈ principals
                 and (resource = W or resource = B or resource = T)
    return max(level of candidates)  or  NONE if empty

effective(user, book B in W)      = max over grants on {W, B}
effective(user, workspace W)      = max over grants on {W}
```

- **Only raise, never restrict** (D-013): a lower-level grant lower than an inherited one has no effect; UI warns "inherited level is higher".
- Disabled users have no access (sessions revoked on disable).
- Soft-deleted resources: only visible in Trash to owners of the parent scope.

### Navigation visibility

A user sees a workspace in the sidebar if they have *any* grant on it, or on any book/table within it. Same for books. Items without effective permission are hidden (e.g. a user with only a table grant sees W → B → that table only). Workspace/book shown as "path-only" (cannot open settings, cannot list siblings).

The resolver reports this as `Access{Level, Visible}`: `Visible` is true when the level is above none **or** the user's principals hold a grant on any live descendant. Guards return **404** when `Visible` is false (hidden, missing, trashed, disabled user) and **403** when the resource is visible but the level is insufficient (including path-only workspaces/books). The sidebar/`/nav` consumer is T1.10/T1.11.

### Implementation

- `acl.Resolver` (D-079) resolves with one SQL statement per lookup: the user's flat group memberships are joined in SQL (no separate per-session group cache), against the grants on the live path W → B → T. Disabled users, trashed targets and trashed ancestors produce no access.
- Per-process LRU cache keyed by (user, resource), 4096 entries, 5 s TTL. D-045 (single instance) makes the NOTIFY broadcast unnecessary: an in-process **epoch** replaces it. Every service that commits a change to grants, group membership, user status/role-disable, or resource trash/restore/delete **must call `Resolver.Invalidate()` after commit** (also when the commit outcome is uncertain). Invalidate drops all entries and bumps the epoch; a lookup that began before the bump is never stored. The TTL only bounds staleness from out-of-band SQL; it is not the invalidation mechanism.
- The grant service never trusts the cache: ownership checks for mutations run uncached inside the workspace-locked transaction.

## 4. Action matrix

| Action | read | write | owner |
|---|:-:|:-:|:-:|
| See workspace/book/table, read rows, read shared views | ✓ | ✓ | ✓ |
| Create/edit/delete **personal** views | ✓ | ✓ | ✓ |
| Export view (CSV/XLSX) | ✓ | ✓ | ✓ |
| Read & write comments | ✓ | ✓ | ✓ |
| View row history | ✓ | ✓ | ✓ |
| Create/update/delete rows, paste, fill-down, link/unlink, upload attachments | | ✓ | ✓ |
| Submit form view | | ✓ | ✓ |
| Create/edit/delete **shared** views (unless locked) | | ✓ | ✓ |
| Import: append rows into existing table | | ✓ | ✓ |
| Lock/unlock shared views; edit locked views | | | ✓ |
| Create/rename/delete tables in a book (need owner on book) | | | ✓ |
| Fields: add/alter/delete/type change; relations; display field | | | ✓ |
| Import as new table | | | ✓ (book) |
| Create/rename/delete books (need owner on workspace) | | | ✓ |
| Rename/delete workspace | | | ✓ (workspace) |
| Manage grants on the scope and its sub-levels (incl. grant owner) | | | ✓ |
| Restore from Trash within scope | | | ✓ |

Anyone (role `user`) can create a new workspace and becomes its owner.

## 5. Grant management rules (D-014)

1. An owner of scope S (effective, possibly inherited) can create/update/delete grants on S and any descendant of S.
2. An owner can grant `owner`.
3. **Last-owner rule:** grant removal/downgrade must keep ≥ 1 *active user* with a direct `owner` grant on the workspace (group-only ownership doesn't count, so it can't vanish through membership changes). Removing/downgrading the last one is rejected (`ERR_LAST_OWNER`). User disable follows the orphaning exception below.
4. Disabling a user who is the last owner of a workspace is allowed, but the workspace then becomes **orphaned** and appears in Admin → Workspaces for ownership transfer.
5. Owners cannot remove their own *inherited* owner level via a lower-level grant (only-raise rule).
6. Only an effective owner may *list* a scope's grants (D-080); `read`/`write` users get 403, hidden scopes 404.

T1.5 implements disable/enable under the user row lock, revokes all sessions in
the same transaction and invalidates the resolver immediately after commit
(also on uncertain commits). Role changes likewise revoke sessions/invalidate,
but do not create resource grants. D-082's last-active-**admin** protection is
independent of workspace ownership; disabling a last direct workspace owner is
allowed and retains the workspace/grants/authorship.

## 6. Relations & cross-table visibility

Relations are within one book, but table-level grants can differ inside a book.

| Situation | Behaviour |
|---|---|
| User reads table A with link to table B, has no access to B | Link cell shows linked rows' **display value only** (primary field); cannot open linked row. |
| Lookup/rollup on B, no access to B | Value masked (`🔒`), excluded from filter/sort. |
| Linking rows | Requires `write` on the table being edited and `read` on the target table (to pick rows). |
| `on_delete=cascade` deleting rows in B | Executes regardless of user's access to B (it's the relation's defined behaviour, set by an owner). |

## 7. Admin powers (data-related)

- See all workspaces with owners, sizes, orphaned flag (names only — no data).
- Transfer ownership: grants `owner` on workspace to a chosen user.
- Hard delete user: see FR-U5 and [10-collaboration.md](10-collaboration.md)/admin section in implementation plan.
- Admin cannot read workspace data unless granted like any user.
