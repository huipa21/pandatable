-- +goose Up
ALTER TABLE meta.books
  ADD COLUMN description text NOT NULL DEFAULT '',
  ALTER COLUMN fallback_icon SET DEFAULT (
    (ARRAY['book', 'table', 'view', 'fields', 'calendar', 'star', 'group'])
      [floor(random() * 7)::integer + 1]
  ),
  DROP CONSTRAINT books_fallback_icon_vocabulary_check;

ALTER TABLE meta.workspaces
  ALTER COLUMN fallback_icon SET DEFAULT (
    (ARRAY['book', 'table', 'view', 'fields', 'calendar', 'star', 'group'])
      [floor(random() * 7)::integer + 1]
  ),
  DROP CONSTRAINT workspaces_fallback_icon_vocabulary_check;

UPDATE meta.books SET fallback_icon = DEFAULT WHERE fallback_icon = 'link';
UPDATE meta.workspaces SET fallback_icon = DEFAULT WHERE fallback_icon = 'link';

ALTER TABLE meta.books
  ADD CONSTRAINT books_fallback_icon_vocabulary_check CHECK (
    fallback_icon IN ('book', 'table', 'view', 'fields', 'calendar', 'star', 'group')
  );
ALTER TABLE meta.workspaces
  ADD CONSTRAINT workspaces_fallback_icon_vocabulary_check CHECK (
    fallback_icon IN ('book', 'table', 'view', 'fields', 'calendar', 'star', 'group')
  );

-- +goose Down
ALTER TABLE meta.books
  DROP COLUMN description,
  ALTER COLUMN fallback_icon SET DEFAULT (
    (ARRAY['book', 'table', 'view', 'fields', 'calendar', 'star', 'group', 'link'])
      [floor(random() * 8)::integer + 1]
  ),
  DROP CONSTRAINT books_fallback_icon_vocabulary_check,
  ADD CONSTRAINT books_fallback_icon_vocabulary_check CHECK (
    fallback_icon IN ('book', 'table', 'view', 'fields', 'calendar', 'star', 'group', 'link')
  );

ALTER TABLE meta.workspaces
  ALTER COLUMN fallback_icon SET DEFAULT (
    (ARRAY['book', 'table', 'view', 'fields', 'calendar', 'star', 'group', 'link'])
      [floor(random() * 8)::integer + 1]
  ),
  DROP CONSTRAINT workspaces_fallback_icon_vocabulary_check,
  ADD CONSTRAINT workspaces_fallback_icon_vocabulary_check CHECK (
    fallback_icon IN ('book', 'table', 'view', 'fields', 'calendar', 'star', 'group', 'link')
  );
