-- sqlc-only catalog declarations. Never executed as a migration.
CREATE VIEW pg_stat_activity AS SELECT ''::name AS datname, ''::text AS state;
CREATE TABLE public.goose_db_version (
    id bigint NOT NULL,
    version_id bigint NOT NULL,
    is_applied boolean NOT NULL
);
