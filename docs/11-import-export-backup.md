# 11 — Import / Export / Backup

## 1. Export (FR-X1)

CSV exports the **current view** (its visible fields, filter, sort; groups flattened).
The PandaTable package exports the **full table** and shared views. Both are streamed
without buffering the complete output in memory (D-189).

### CSV
- UTF-8 with BOM (Excel-friendly for Chinese), comma separator, RFC 4180 quoting.
- Values formatted via codec `FormatText` (select labels, `a, b` for multi-select, user display names, link display values, attachment file names). Datetimes formatted in the exporting user's timezone (ISO-like `YYYY-MM-DD HH:mm:ss`).
- Data only — no types.
- Values are not formula-escaped because the CSV preserves the formatted data. Spreadsheet applications may interpret untrusted values beginning with formula characters as formulas; open exports only from trusted sources.

### XLSX (PandaTable package)
Generated with `excelize` streaming writer.

| Sheet | Content |
|---|---|
| `Data` | First column `_id` holds original row IDs; subsequent headers are field names. Cells use numeric, date and boolean types where representable, otherwise text. |
| `_pandatable` (very-hidden state) | JSON manifest in `A1..An`, split at 32,000 UTF-16 code units without splitting a character. |

Manifest:
```jsonc
{
  "format": "pandatable.table",
  "formatVersion": 1,
  "appVersion": "1.0.0",
  "exportedAt": "2026-10-24T03:00:00Z",
  "table": { "name": "Tasks", "displayField": "f1" },
  "fields": [                                  // order = Data sheet column order (all fields of table, incl. hidden in view)
    { "key": "f1", "name": "Title", "uiType": "text", "pgType": "text", "options": {}, "required": true, "unique": false, "default": null },
    { "key": "f2", "name": "Status", "uiType": "single_select", "pgType": "text", "options": { "choices": [ … ] } },
    { "key": "f3", "name": "Project", "uiType": "link", "options": { "kind": "one_many", "side": "child", "targetTable": "Projects", "onDelete": "set_null" } },
    { "key": "f4", "name": "Total", "uiType": "formula", "options": { "expr": "{Price} * {Qty}" } }
  ],
  "views": [ { "name": "Grid", "type": "grid", "config": { /* field ids replaced by keys */ } } ],
  "rowIdColumn": true                          // first column "_id" holds original row ids (for link re-mapping)
}
```

Notes:
- The package contains **all fields and all rows of the table** (not only the view), plus all shared views; the CSV respects the view. The export control offers *CSV (current view)* and *XLSX PandaTable package (full table + views + types)*. A data-only current-view XLSX is not part of T3.7.
- List-valued `multi_select`, `text_array`, `int_array`, and `multi_user` cells contain JSON arrays of their canonical string values. Link cells contain JSON arrays of target display values; the manifest keeps the relation definition. Package import must decode these arrays rather than splitting on commas. Formulas are stored as expressions (values also written for readability).
- Attachments: file names only (files are not embedded; backups cover files). Shared-view locks are not included.
- User fields in packages contain UUIDs rather than display names, allowing identity resolution on import. Datetimes are UTC Excel date cells, displayed to milliseconds for Excel compatibility; imports must read the raw serial before any formatted-cell access and account for Excel's floating-point precision. Dates before 1900 are ISO text. Numbers with more than 15 significant digits and padded autonumbers are text to avoid truncation.
- XLSX's row, column and cell limits apply. Each uncompressed ZIP entry is limited to less than 4 GiB; an oversized or failed stream is aborted rather than returned as a successful partial file.

## 2. Import (FR-X2)

Flow: upload → `imports:analyze` → wizard → `imports` job → progress → done.

### Plain CSV/XLSX
1. Analyze: detect encoding (UTF-8/UTF-8-BOM/GBK), delimiter, header row; sample first 1,000 rows; guess type per column (integer → decimal → date → datetime → checkbox → single select if ≤ 20 distinct values & repeated → text).
2. Wizard: for new table — rename columns, choose type, skip column; for existing table — map each column to an existing field or skip.
3. Execute: stream rows, parse with codec `ParsePaste`, `COPY` into the table in batches of 5,000; invalid cells → NULL and reported (row, column, value) in the result (first 1,000 issues downloadable as CSV). If a required or codec-enforced non-null column cannot store NULL, the batch fails explicitly instead. Reports retain the original values and carry the same spreadsheet-formula risk as CSV exports.

### PandaTable package (manifest detected)
- **New table:** recreate fields with exact types/options/constraints and views; insert rows. Computed fields recreated after data. Link fields: if a table with the manifest's `targetTable` name exists in the target book, user may choose to re-link by matching display values; otherwise converted to text fields (user informed).
- When links become text, dependent lookup, rollup and formula fields retain their definitions but display `#ERROR` until repaired. Source formulas are checked against possible missing types within a bounded search; an inconclusive check fails the import explicitly. Views using unavailable fields retain their saved settings, and the result names views to review; filters, highlights, sorting or grouping may need repair before use (D-192).
- **Append to existing table:** mapping auto-filled by field name + type compatibility.
- Import is one transaction for new tables (all-or-nothing); for appends, batches commit progressively with result report.
- Attachment names in the existing package format cannot recover file identities or contents; names mapped to attachment fields are reported invalid and stored as NULL. Exact timestamp round-trips are subject to Excel serial precision. The manifest omits shared-view locks, so restored views are unlocked. See D-190.

Limits: file ≤ the configured upload limit (maximum 100 MB), ≤ 500k rows. Import jobs and their status are process-local. Graceful shutdown cancels jobs and removes tracked temporary files; abrupt termination, a timeout or cleanup failure can leave files behind (D-191).

## 3. Backup (FR-X3)

### Create
Admin → Backups → **Create backup** → async job:

1. Export a repeatable-read database snapshot with `pg_dump --format=custom --no-owner` → temp `db.dump` (requires `pg_dump` 18 in the app image). Backup-catalog rows are excluded; they describe local files, not portable data.
2. Zip (deflate, streamed): `db.dump`, `files/**` (attachments), `backup.json` (`{appVersion, schemaMigration, createdAt, pgVersion, fileCount}`).
3. Write to `$PT_DATA_DIR/backups/pandatable-YYYYMMDD-HHMMSS.zip` (timestamp in app `default_timezone`), record in `meta.backups`.
4. A PostgreSQL advisory lock permits one backup or restore at a time, including across app instances. The job runs with `nice`/low priority (`ionice` is not available in containers). Backup and restore transactions disable the configured idle and statement timeouts locally so large jobs can finish; cancellation still stops them.

### List / download / delete
- List shows file, size, status, created by/at; free disk shown alongside.
- Download streamed with `Content-Length`; delete removes file + record.
- Pre-flight: refuse to start if free disk < 2 × (db size + files size).
- Failed or interrupted jobs are marked failed; incomplete files are removed. A staged deletion is reconciled with the database after interruption; conflicting files are preserved and reported rather than overwritten.

### Restore (CLI)
```
docker compose stop app
docker compose run --rm app restore /data/backups/pandatable-20261024-030000.zip --yes
docker compose up -d app
```
Stop **all** app instances before restoring. Restore only a trusted archive: PostgreSQL dumps can execute SQL. The archive is validated and staged before replacement, so its uncompressed contents must fit in the data volume's free space. The backup must have the same app major version, no newer app release or migration, and a PostgreSQL major version no newer than the target. At the same numeric app version, a prerelease target accepts only the same prerelease.

Restore drops and recreates database objects in a transaction using `pg_restore --clean --if-exists`, replaces `files/`, and runs migrations. Existing catalog entries for valid local backup files are retained; an archive restored from the local backups directory is registered there. A failure before database commit leaves existing data unchanged. If commit is uncertain or a later step fails, the command reports the recovery staging directory: keep the app stopped, retain the archive and staging files, inspect the error, and retry the restore before restarting. No UI restore is provided.

### Backup scope note
Backups are consistent snapshots of the DB (pg_dump uses a single snapshot). Attachments copied after the dump may include files newer than the dump — harmless (orphan files are cleaned by the purge job).
