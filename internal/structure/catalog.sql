-- sqlc-only catalog declarations. Never executed as a migration.
CREATE VIEW pg_timezone_names AS SELECT ''::text AS name;