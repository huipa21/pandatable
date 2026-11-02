-- +goose Up
ALTER TABLE meta.users
  ADD COLUMN mention_email boolean NOT NULL DEFAULT false;

CREATE TABLE meta.smtp_settings (
  singleton       boolean PRIMARY KEY DEFAULT true CHECK (singleton),
  enabled         boolean NOT NULL DEFAULT false,
  host            text NOT NULL DEFAULT '',
  port            integer NOT NULL DEFAULT 587 CHECK (port BETWEEN 1 AND 65535),
  security        text NOT NULL DEFAULT 'starttls' CHECK (security IN ('starttls', 'tls', 'none')),
  username        text NOT NULL DEFAULT '',
  password_cipher bytea,
  from_address    text NOT NULL DEFAULT ''
);
INSERT INTO meta.smtp_settings (singleton) VALUES (true);

CREATE TABLE meta.password_resets (
  user_id    uuid PRIMARY KEY REFERENCES meta.users(id) ON DELETE CASCADE,
  token_hash bytea NOT NULL UNIQUE CHECK (octet_length(token_hash) = 32),
  expires_at timestamptz NOT NULL
);

-- +goose StatementBegin
CREATE FUNCTION meta.invalidate_password_reset() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
  IF OLD.password_hash IS DISTINCT FROM NEW.password_hash
     OR OLD.email IS DISTINCT FROM NEW.email
     OR OLD.status IS DISTINCT FROM NEW.status THEN
    DELETE FROM meta.password_resets WHERE user_id = OLD.id;
  END IF;
  RETURN NEW;
END;
$$;
-- +goose StatementEnd
CREATE TRIGGER invalidate_password_reset
  AFTER UPDATE OF password_hash, email, status ON meta.users
  FOR EACH ROW EXECUTE FUNCTION meta.invalidate_password_reset();

CREATE TABLE meta.mention_mail_jobs (
  notification_id uuid PRIMARY KEY REFERENCES meta.notifications(id) ON DELETE CASCADE,
  attempts        integer NOT NULL DEFAULT 0 CHECK (attempts >= 0),
  next_attempt_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX mention_mail_jobs_due_idx ON meta.mention_mail_jobs(next_attempt_at);

-- +goose Down
DROP TABLE meta.mention_mail_jobs;
DROP TRIGGER invalidate_password_reset ON meta.users;
DROP FUNCTION meta.invalidate_password_reset();
DROP TABLE meta.password_resets;
DROP TABLE meta.smtp_settings;
ALTER TABLE meta.users DROP COLUMN mention_email;
