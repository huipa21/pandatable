-- +goose Up
ALTER TABLE meta.workspaces
  ADD COLUMN description text NOT NULL DEFAULT '',
  ADD COLUMN fallback_icon text;

ALTER TABLE meta.workspaces
  ALTER COLUMN fallback_icon SET DEFAULT (
    (ARRAY['book', 'table', 'view', 'fields', 'calendar', 'star', 'group', 'link'])
      [floor(random() * 8)::integer + 1]
  );

UPDATE meta.workspaces
SET fallback_icon = DEFAULT
WHERE fallback_icon IS NULL;

UPDATE meta.workspaces
SET icon = NULL
WHERE icon IS NOT NULL AND icon NOT IN (
  'panelClose', 'panelOpen', 'search', 'sun', 'moon', 'undo', 'redo',
  'table', 'book', 'view', 'chevronDown', 'chevronRight', 'plus', 'more',
  'fields', 'filter', 'group', 'sort', 'colour', 'height', 'trash', 'eye',
  'expand', 'fullscreen', 'close', 'duplicate', 'chevronUp', 'comments',
  'history', 'compact', 'edit', 'calendar', 'clock', 'check', 'link',
  'text', 'number', 'formula', 'lookup', 'rollup', 'attachment', 'mail',
  'star', 'phone', 'code', 'user', 'percent', 'currency'
);

ALTER TABLE meta.workspaces
  ALTER COLUMN fallback_icon SET NOT NULL,
  ADD CONSTRAINT workspaces_icon_vocabulary_check CHECK (
    icon IS NULL OR icon IN (
      'panelClose', 'panelOpen', 'search', 'sun', 'moon', 'undo', 'redo',
      'table', 'book', 'view', 'chevronDown', 'chevronRight', 'plus', 'more',
      'fields', 'filter', 'group', 'sort', 'colour', 'height', 'trash', 'eye',
      'expand', 'fullscreen', 'close', 'duplicate', 'chevronUp', 'comments',
      'history', 'compact', 'edit', 'calendar', 'clock', 'check', 'link',
      'text', 'number', 'formula', 'lookup', 'rollup', 'attachment', 'mail',
      'star', 'phone', 'code', 'user', 'percent', 'currency'
    )
  ),
  ADD CONSTRAINT workspaces_fallback_icon_vocabulary_check CHECK (
    fallback_icon IN (
      'book', 'table', 'view', 'fields', 'calendar', 'star', 'group', 'link'
    )
  );

-- +goose Down
ALTER TABLE meta.workspaces
  DROP CONSTRAINT workspaces_fallback_icon_vocabulary_check,
  DROP CONSTRAINT workspaces_icon_vocabulary_check,
  DROP COLUMN fallback_icon,
  DROP COLUMN description;
