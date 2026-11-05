package db_test

import (
	"context"
	"reflect"
	"sort"
	"testing"
	"time"
)

type columnSpec struct {
	name, dataType string
	nullable       bool
	defaultValue   string
}

var metadataColumns = map[string][]columnSpec{
	"deleted_users": {
		{"id", "uuid", false, ""},
	},
	"groups": {
		{"id", "uuid", false, "uuidv7()"},
		{"name", "text", false, ""},
		{"description", "text", false, "''::text"},
		{"created_at", "timestamp with time zone", false, "now()"},
	},
	"group_members": {
		{"group_id", "uuid", false, ""},
		{"user_id", "uuid", false, ""},
	},
	"workspaces": {
		{"id", "uuid", false, "uuidv7()"},
		{"name", "text", false, ""},
		{"icon", "text", true, ""},
		{"color", "text", true, ""},
		{"created_by", "uuid", true, ""},
		{"created_at", "timestamp with time zone", false, "now()"},
		{"deleted_at", "timestamp with time zone", true, ""},
		{"deleted_by", "uuid", true, ""},
	},
	"user_workspace_preferences": {
		{"user_id", "uuid", false, ""},
		{"workspace_id", "uuid", true, ""},
	},
	"books": {
		{"id", "uuid", false, "uuidv7()"},
		{"workspace_id", "uuid", false, ""},
		{"name", "text", false, ""},
		{"pg_schema", "text", false, ""},
		{"position", "double precision", false, ""},
		{"created_by", "uuid", true, ""},
		{"created_at", "timestamp with time zone", false, "now()"},
		{"deleted_at", "timestamp with time zone", true, ""},
		{"deleted_by", "uuid", true, ""},
	},
	"tables": {
		{"id", "uuid", false, "uuidv7()"},
		{"book_id", "uuid", false, ""},
		{"name", "text", false, ""},
		{"pg_name", "text", false, ""},
		{"display_field_id", "uuid", true, ""},
		{"position", "double precision", false, ""},
		{"schema_version", "integer", false, "1"},
		{"created_by", "uuid", true, ""},
		{"created_at", "timestamp with time zone", false, "now()"},
		{"deleted_at", "timestamp with time zone", true, ""},
		{"deleted_by", "uuid", true, ""},
	},
	"fields": {
		{"id", "uuid", false, "uuidv7()"},
		{"table_id", "uuid", false, ""},
		{"name", "text", false, ""},
		{"description", "text", false, "''::text"},
		{"ui_type", "text", false, ""},
		{"pg_column", "text", true, ""},
		{"pg_type", "text", true, ""},
		{"options", "jsonb", false, "'{}'::jsonb"},
		{"is_system", "boolean", false, "false"},
		{"is_required", "boolean", false, "false"},
		{"is_unique", "boolean", false, "false"},
		{"default_json", "jsonb", true, ""},
		{"position", "double precision", false, ""},
		{"version", "integer", false, "1"},
		{"created_at", "timestamp with time zone", false, "now()"},
		{"deleted_at", "timestamp with time zone", true, ""},
		{"trash_required", "boolean", true, ""},
	},
	"relations": {
		{"id", "uuid", false, "uuidv7()"},
		{"book_id", "uuid", false, ""},
		{"kind", "text", false, ""},
		{"parent_table_id", "uuid", false, ""},
		{"child_table_id", "uuid", false, ""},
		{"parent_field_id", "uuid", false, ""},
		{"child_field_id", "uuid", false, ""},
		{"fk_column", "text", true, ""},
		{"junction_table", "text", true, ""},
		{"on_delete", "text", false, "'set_null'::text"},
		{"created_at", "timestamp with time zone", false, "now()"},
	},
	"views": {
		{"id", "uuid", false, "uuidv7()"},
		{"table_id", "uuid", false, ""},
		{"name", "text", false, ""},
		{"type", "text", false, ""},
		{"scope", "text", false, ""},
		{"owner_id", "uuid", true, ""},
		{"locked", "boolean", false, "false"},
		{"position", "double precision", false, ""},
		{"config", "jsonb", false, "'{}'::jsonb"},
		{"version", "integer", false, "1"},
		{"created_by", "uuid", true, ""},
		{"created_at", "timestamp with time zone", false, "now()"},
		{"updated_at", "timestamp with time zone", false, "now()"},
		{"deleted_at", "timestamp with time zone", true, ""},
	},
	"grants": {
		{"id", "uuid", false, "uuidv7()"},
		{"resource_type", "text", false, ""},
		{"resource_id", "uuid", false, ""},
		{"principal_type", "text", false, ""},
		{"principal_id", "uuid", false, ""},
		{"level", "text", false, ""},
		{"granted_by", "uuid", true, ""},
		{"created_at", "timestamp with time zone", false, "now()"},
	},
	"comments": {
		{"id", "uuid", false, "uuidv7()"},
		{"table_id", "uuid", false, ""},
		{"row_id", "bigint", false, ""},
		{"user_id", "uuid", true, ""},
		{"body", "text", false, ""},
		{"mentions", "uuid[]", false, "'{}'::uuid[]"},
		{"created_at", "timestamp with time zone", false, "now()"},
		{"edited_at", "timestamp with time zone", true, ""},
		{"deleted_at", "timestamp with time zone", true, ""},
	},
	"notifications": {
		{"id", "uuid", false, "uuidv7()"},
		{"user_id", "uuid", false, ""},
		{"kind", "text", false, ""},
		{"payload", "jsonb", false, ""},
		{"read_at", "timestamp with time zone", true, ""},
		{"created_at", "timestamp with time zone", false, "now()"},
	},
	"smtp_settings": {
		{"singleton", "boolean", false, "true"},
		{"enabled", "boolean", false, "false"},
		{"host", "text", false, "''::text"},
		{"port", "integer", false, "587"},
		{"security", "text", false, "'starttls'::text"},
		{"username", "text", false, "''::text"},
		{"password_cipher", "bytea", true, ""},
		{"from_address", "text", false, "''::text"},
	},
	"password_resets": {
		{"user_id", "uuid", false, ""},
		{"token_hash", "bytea", false, ""},
		{"expires_at", "timestamp with time zone", false, ""},
	},
	"mention_mail_jobs": {
		{"notification_id", "uuid", false, ""},
		{"attempts", "integer", false, "0"},
		{"next_attempt_at", "timestamp with time zone", false, "now()"},
	},
	"row_history": {
		{"id", "bigint", false, ""},
		{"table_id", "uuid", false, ""},
		{"row_id", "bigint", true, ""},
		{"user_id", "uuid", true, ""},
		{"action", "text", false, ""},
		{"changes", "jsonb", false, ""},
		{"created_at", "timestamp with time zone", false, "now()"},
		{"restore_system", "jsonb", true, ""},
	},
	"attachments": {
		{"id", "uuid", false, "uuidv7()"},
		{"table_id", "uuid", false, ""},
		{"field_id", "uuid", false, ""},
		{"file_name", "text", false, ""},
		{"mime_type", "text", false, ""},
		{"size_bytes", "bigint", false, ""},
		{"sha256", "bytea", false, ""},
		{"storage_path", "text", false, ""},
		{"has_thumb", "boolean", false, "false"},
		{"uploaded_by", "uuid", true, ""},
		{"created_at", "timestamp with time zone", false, "now()"},
	},
	"attachment_purge_queue": {
		{"id", "uuid", false, ""},
		{"storage_path", "text", false, ""},
		{"has_thumb", "boolean", false, ""},
		{"cleanup_user_id", "uuid", true, ""},
	},
	"backups": {
		{"id", "uuid", false, "uuidv7()"},
		{"file_name", "text", false, ""},
		{"size_bytes", "bigint", true, ""},
		{"status", "text", false, ""},
		{"error", "text", true, ""},
		{"created_by", "uuid", true, ""},
		{"created_at", "timestamp with time zone", false, "now()"},
		{"finished_at", "timestamp with time zone", true, ""},
	},
}

func TestMetadataCatalogPostgreSQL(t *testing.T) {
	pool := database.Open(t)
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	var names []string
	if err := pool.QueryRow(ctx, `SELECT array_agg(tablename::text ORDER BY tablename)
		FROM pg_tables WHERE schemaname = 'meta'`).Scan(&names); err != nil {
		t.Fatal(err)
	}
	wantNames := []string{"users", "sessions", "settings"}
	for name := range metadataColumns {
		wantNames = append(wantNames, name)
	}
	sort.Strings(wantNames)
	if !reflect.DeepEqual(names, wantNames) {
		t.Fatalf("meta tables = %v, want %v", names, wantNames)
	}

	for table, want := range metadataColumns {
		t.Run(table, func(t *testing.T) {
			rows, err := pool.Query(ctx, `
				SELECT a.attname, format_type(a.atttypid, a.atttypmod), NOT a.attnotnull,
				       coalesce(pg_get_expr(d.adbin, d.adrelid), '')
				FROM pg_attribute a
				LEFT JOIN pg_attrdef d ON d.adrelid = a.attrelid AND d.adnum = a.attnum
				WHERE a.attrelid = ('meta.' || $1)::regclass AND a.attnum > 0 AND NOT a.attisdropped
				ORDER BY a.attnum`, table)
			if err != nil {
				t.Fatal(err)
			}
			defer rows.Close()
			var got []columnSpec
			for rows.Next() {
				var column columnSpec
				if err := rows.Scan(&column.name, &column.dataType, &column.nullable, &column.defaultValue); err != nil {
					t.Fatal(err)
				}
				got = append(got, column)
			}
			if err := rows.Err(); err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(got, want) {
				t.Errorf("columns (name/type/nullable/default):\ngot  %+v\nwant %+v", got, want)
			}
		})
	}
	var alwaysIdentity bool
	if err := pool.QueryRow(ctx, `SELECT attidentity = 'a' FROM pg_attribute
		WHERE attrelid = 'meta.row_history'::regclass AND attname = 'id'`).Scan(&alwaysIdentity); err != nil {
		t.Fatal(err)
	}
	if !alwaysIdentity {
		t.Error("row_history.id must be GENERATED ALWAYS AS IDENTITY")
	}
	var invalidConstraints int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM pg_constraint
		WHERE connamespace = 'meta'::regnamespace AND NOT convalidated`).Scan(&invalidConstraints); err != nil || invalidConstraints != 0 {
		t.Fatalf("unvalidated constraints = %d, error = %v", invalidConstraints, err)
	}

	indexes := map[string]string{
		"group_members_user_id_idx":                "CREATE INDEX group_members_user_id_idx ON meta.group_members USING btree (user_id)",
		"user_workspace_preferences_workspace_idx": "CREATE INDEX user_workspace_preferences_workspace_idx ON meta.user_workspace_preferences USING btree (workspace_id) WHERE (workspace_id IS NOT NULL)",
		"books_workspace_id_idx":                   "CREATE INDEX books_workspace_id_idx ON meta.books USING btree (workspace_id)",
		"tables_book_id_pg_name_key":               "CREATE UNIQUE INDEX tables_book_id_pg_name_key ON meta.tables USING btree (book_id, pg_name)",
		"tables_book_id_id_key":                    "CREATE UNIQUE INDEX tables_book_id_id_key ON meta.tables USING btree (book_id, id)",
		"fields_table_id_id_key":                   "CREATE UNIQUE INDEX fields_table_id_id_key ON meta.fields USING btree (table_id, id)",
		"fields_table_id_pg_column_key":            "CREATE UNIQUE INDEX fields_table_id_pg_column_key ON meta.fields USING btree (table_id, pg_column)",
		"fields_table_id_lower_name_idx":           "CREATE UNIQUE INDEX fields_table_id_lower_name_idx ON meta.fields USING btree (table_id, lower(name)) WHERE (deleted_at IS NULL)",
		"relations_book_id_junction_table_key":     "CREATE UNIQUE INDEX relations_book_id_junction_table_key ON meta.relations USING btree (book_id, junction_table)",
		"relations_child_table_id_fk_column_key":   "CREATE UNIQUE INDEX relations_child_table_id_fk_column_key ON meta.relations USING btree (child_table_id, fk_column)",
		"relations_parent_field_idx":               "CREATE INDEX relations_parent_field_idx ON meta.relations USING btree (parent_table_id, parent_field_id)",
		"relations_child_field_idx":                "CREATE INDEX relations_child_field_idx ON meta.relations USING btree (child_table_id, child_field_id)",
		"views_table_id_idx":                       "CREATE INDEX views_table_id_idx ON meta.views USING btree (table_id)",
		"views_owner_id_idx":                       "CREATE INDEX views_owner_id_idx ON meta.views USING btree (owner_id)",
		"grants_principal_idx":                     "CREATE INDEX grants_principal_idx ON meta.grants USING btree (principal_type, principal_id)",
		"comments_table_row_idx":                   "CREATE INDEX comments_table_row_idx ON meta.comments USING btree (table_id, row_id)",
		"notifications_user_read_idx":              "CREATE INDEX notifications_user_read_idx ON meta.notifications USING btree (user_id, read_at)",
		"mention_mail_jobs_due_idx":                "CREATE INDEX mention_mail_jobs_due_idx ON meta.mention_mail_jobs USING btree (next_attempt_at)",
		"row_history_table_row_id_idx":             "CREATE INDEX row_history_table_row_id_idx ON meta.row_history USING btree (table_id, row_id, id DESC)",
		"row_history_created_at_idx":               "CREATE INDEX row_history_created_at_idx ON meta.row_history USING btree (created_at)",
		"attachments_table_field_idx":              "CREATE INDEX attachments_table_field_idx ON meta.attachments USING btree (table_id, field_id)",
	}
	for name, want := range indexes {
		var got string
		var valid bool
		if err := pool.QueryRow(ctx, `SELECT pg_get_indexdef(indexrelid), indisvalid AND indisready
			FROM pg_index WHERE indexrelid = ('meta.' || $1)::regclass`, name).Scan(&got, &valid); err != nil {
			t.Fatalf("index %s: %v", name, err)
		}
		if !valid || got != want {
			t.Errorf("index %s: valid=%v, definition=%s; want %s", name, valid, got, want)
		}
	}
	t.Log("clean migration: all meta tables; exact new column types/nullability/defaults, identity and lookup indexes verified")
}
