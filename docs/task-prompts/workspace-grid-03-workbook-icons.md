# UX-03 - Workbook icon metadata and random fallback

Requires UX-01. Can run alongside UX-02 and UX-08; no navigation-source edits.

## Read (only these)

- `AGENTS.md`, `docs/README.md`, `docs/conventions.md`.
- [Execution index](workspace-grid-redesign.md).
- `docs/05-data-model.md` books; `docs/09-api.md` Structure.
- Search decisions D-023, D-094..D-096, D-131, D-217.
- Search acceptance AT-201, AT-202, AT-208, AT-212.

## Scope

Provide a workbook's icon, including stable random assignment when unset.
No card layout, sidebar controls or new icon picker/upload system.

- First inspect book DTOs, SQL and validators. Workspace `icon` already exists,
  but `NavBook` currently has no icon. Do not confuse workspace/field icons with
  book metadata or store this in a field definition.
- Expose a nullable user-selected book icon in structure reads/mutations and
  `/nav`, reusing the existing local icon vocabulary and validation conventions.
  Preserve existing callers that send only `name`/`index`.
- Give books without a user icon a stable randomly assigned fallback icon from
  a bounded local palette. Assign once, persist with the book, and handle existing
  books via a safe forward migration/backfill. No per-render randomization,
  external image URLs, global localStorage map or reads that rewrite every book.
- A custom icon overrides the fallback; clearing the custom icon returns to
  the same fallback. Sharing a book should not produce different icons per user.

## Ownership and handoff

Own book-related migration/SQL/generated bindings, `internal/structure/books.go`,
DTO/nav serialization, relevant HTTP decoding/validation, Go tests and API types.
UX-01 has finished schema/default changes before this task starts. In this wave
UX-02 owns `stores/nav.ts` and navigation components; do not edit them.

Publish exact icon field names, allowed values and null/clear semantics for UX-04.
If a tiny pure frontend icon resolver is useful, keep it isolated from navigation
and document it. Do not add a package just for random icon assignment.
Use authoritative parent-scope authorization for book edits.

## Acceptance scenarios

1. Create a name-only book: a fallback is assigned and remains identical on read,
   refresh, login by another authorized user and server restart.
2. Existing books receive valid stable fallbacks without name/order/physical
   schema/grant changes; upgrade is safe for a populated database.
3. Set a valid custom icon; reads/nav return it. Clear it; the original fallback
   reappears. Unknown icon identifiers fail explicitly.
4. Readers and unauthorized users cannot change book metadata; no hidden book
   or icon leaks through navigation. Rename/reorder preserve icon values.
5. Concurrent updates and migration reruns do not continually re-randomize icons.

## Verification and wrap-up

Focused required-PG18 tests for book CRUD/nav/upgrade under the coordinator's DB
slot; pinned sqlc regeneration if applicable; Go build and DTO type checks.
Add focused coverage to be exercised from UX-04's workbook-card browser spec.
Central docs, gates and commits follow the execution index.

## Record

Not started.
