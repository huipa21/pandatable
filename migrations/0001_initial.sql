-- +goose Up
CREATE SCHEMA meta;

CREATE EXTENSION IF NOT EXISTS pg_trgm;
CREATE EXTENSION IF NOT EXISTS pg_stat_statements;

CREATE TABLE meta.users (
  id                    uuid PRIMARY KEY DEFAULT uuidv7(),
  username              text NOT NULL UNIQUE,
  email                 text UNIQUE,
  display_name          text NOT NULL,
  password_hash         text NOT NULL,
  role                  text NOT NULL CHECK (role IN ('admin','user')),
  status                text NOT NULL DEFAULT 'active' CHECK (status IN ('active','disabled')),
  must_change_password  boolean NOT NULL DEFAULT false,
  locale                text NOT NULL DEFAULT 'en' CHECK (locale IN ('en','zh-CN')),
  theme                 text NOT NULL DEFAULT 'system' CHECK (theme IN ('light','dark','system')),
  timezone              text,
  avatar_color          text NOT NULL,
  last_login_at         timestamptz,
  created_at            timestamptz NOT NULL DEFAULT now(),
  updated_at            timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE meta.sessions (
  id_hash       bytea PRIMARY KEY,
  user_id       uuid NOT NULL REFERENCES meta.users(id) ON DELETE CASCADE,
  created_at    timestamptz NOT NULL DEFAULT now(),
  last_seen_at  timestamptz NOT NULL DEFAULT now(),
  expires_at    timestamptz NOT NULL,
  ip            inet,
  user_agent    text
);
CREATE INDEX sessions_user_id_idx ON meta.sessions(user_id);

CREATE TABLE meta.settings (
  key    text PRIMARY KEY,
  value  jsonb NOT NULL
);

-- +goose Down
DROP SCHEMA meta CASCADE;
