package backup

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"io/fs"
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestBackupSpaceBoundaries(t *testing.T) {
	for _, test := range []struct {
		free, database, files uint64
		want                  bool
	}{
		{29, 10, 5, false}, {30, 10, 5, true}, {31, 10, 5, true},
		{math.MaxUint64, math.MaxUint64, 1, false},
		{math.MaxUint64, math.MaxUint64/2 + 1, 0, false},
	} {
		if got := hasBackupSpace(test.free, test.database, test.files); got != test.want {
			t.Errorf("hasBackupSpace(%d, %d, %d) = %v", test.free, test.database, test.files, got)
		}
	}
}

func TestBackupArchiveStreamsContentsAndMetadata(t *testing.T) {
	dir := t.TempDir()
	files := filepath.Join(dir, "files")
	if err := os.MkdirAll(filepath.Join(files, "2026", "11"), 0o700); err != nil {
		t.Fatal(err)
	}
	for name, content := range map[string]string{
		"attachment.txt":  "attachment contents",
		".upload-pending": "incomplete upload",
	} {
		if err := os.WriteFile(filepath.Join(files, "2026", "11", name), []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	dump := filepath.Join(dir, "database.dump")
	if err := os.WriteFile(dump, []byte("custom dump"), 0o600); err != nil {
		t.Fatal(err)
	}
	archive := filepath.Join(dir, "backup.zip")
	metadata := Metadata{AppVersion: "1.1.0", SchemaMigration: 4, CreatedAt: time.Now().UTC(), PgVersion: "18.6"}
	var progress []int
	size, err := writeArchive(context.Background(), archive, dump, files, metadata, func(value int) {
		progress = append(progress, value)
	})
	if err != nil || size <= 0 || len(progress) == 0 || progress[len(progress)-1] != 95 {
		t.Fatalf("write archive: size=%d progress=%v err=%v", size, progress, err)
	}
	reader, err := zip.OpenReader(archive)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = reader.Close() }()
	entries := make(map[string]*zip.File)
	for _, entry := range reader.File {
		entries[entry.Name] = entry
	}
	for _, name := range []string{"db.dump", "files/", "files/2026/11/attachment.txt", "backup.json"} {
		if entries[name] == nil {
			t.Errorf("missing archive entry %s", name)
		}
	}
	if entries["files/2026/11/.upload-pending"] != nil {
		t.Fatal("incomplete upload included in backup")
	}
	if entries["db.dump"].Method != zip.Deflate || entries["files/2026/11/attachment.txt"].Method != zip.Deflate {
		t.Fatal("backup files are not deflated")
	}
	stage := filepath.Join(dir, "stage")
	if err := os.Mkdir(stage, 0o700); err != nil {
		t.Fatal(err)
	}
	got, err := stageArchive(context.Background(), archive, stage, "1.2.0", "18.6")
	if err != nil || got.FileCount != 1 || got.SchemaMigration != 4 {
		t.Fatalf("stage archive: metadata=%+v err=%v", got, err)
	}
	content, err := os.ReadFile(filepath.Join(stage, "files", "2026", "11", "attachment.txt"))
	if err != nil || string(content) != "attachment contents" {
		t.Fatalf("staged attachment=%q err=%v", content, err)
	}
}

type zipTestEntry struct {
	name string
	body string
	mode fs.FileMode
}

func makeTestArchive(t *testing.T, entries []zipTestEntry) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "backup.zip")
	file, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	writer := zip.NewWriter(file)
	for _, entry := range entries {
		header := &zip.FileHeader{Name: entry.name, Method: zip.Deflate}
		if entry.mode != 0 {
			header.SetMode(entry.mode)
		}
		output, err := writer.CreateHeader(header)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := io.WriteString(output, entry.body); err != nil {
			t.Fatal(err)
		}
	}
	if err := errors.Join(writer.Close(), file.Close()); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestRestoreArchiveRejectsInvalidInputsBeforeExtraction(t *testing.T) {
	metadata := Metadata{AppVersion: "1.2.0", SchemaMigration: 4,
		CreatedAt: time.Now().UTC(), PgVersion: "18.6", FileCount: 0}
	manifest, err := json.Marshal(metadata)
	if err != nil {
		t.Fatal(err)
	}
	base := []zipTestEntry{{name: "db.dump", body: "dump"}, {name: "files/", mode: fs.ModeDir | 0o700},
		{name: "backup.json", body: string(manifest)}}
	for _, test := range []struct {
		name    string
		entries []zipTestEntry
	}{
		{"missing dump", base[1:]},
		{"missing files", []zipTestEntry{base[0], base[2]}},
		{"duplicate", append(append([]zipTestEntry{}, base...), base[0])},
		{"traversal", append(append([]zipTestEntry{}, base...), zipTestEntry{name: "files/../escape", body: "bad"})},
		{"windows separator", append(append([]zipTestEntry{}, base...), zipTestEntry{name: `files\escape`, body: "bad"})},
		{"symlink", append(append([]zipTestEntry{}, base...), zipTestEntry{name: "files/link", mode: fs.ModeSymlink | 0o777})},
		{"count mismatch", append(append([]zipTestEntry{}, base...), zipTestEntry{name: "files/a", body: "bad"})},
		{"missing file count", []zipTestEntry{base[0], base[1], {name: "backup.json",
			body: `{"appVersion":"1.2.0","schemaMigration":4,"createdAt":"2026-11-01T00:00:00Z","pgVersion":"18.6"}`}}},
		{"unknown metadata", []zipTestEntry{base[0], base[1], {name: "backup.json",
			body: strings.TrimSuffix(string(manifest), "}") + `,"unexpected":true}`}}},
	} {
		t.Run(test.name, func(t *testing.T) {
			archive := makeTestArchive(t, test.entries)
			stage := t.TempDir()
			if _, err := stageArchive(context.Background(), archive, stage, "1.2.0", "18.6"); err == nil {
				t.Fatal("invalid archive accepted")
			}
			entries, err := os.ReadDir(stage)
			if err != nil || len(entries) != 0 {
				t.Fatalf("invalid archive was extracted: %v %v", entries, err)
			}
		})
	}
}

func TestRestoreDetectsCorruptZIPData(t *testing.T) {
	metadata := `{"appVersion":"1.2.0","schemaMigration":4,"createdAt":"2026-11-01T00:00:00Z","pgVersion":"18.6","fileCount":0}`
	path := makeTestArchive(t, []zipTestEntry{
		{name: "db.dump", body: strings.Repeat("dump payload", 100)},
		{name: "files/", mode: fs.ModeDir | 0o700},
		{name: "backup.json", body: metadata},
	})
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	reader, err := zip.NewReader(bytes.NewReader(raw), int64(len(raw)))
	if err != nil {
		t.Fatal(err)
	}
	offset, err := reader.File[0].DataOffset()
	if err != nil {
		t.Fatal(err)
	}
	raw[offset+2] ^= 0xff
	if err := os.WriteFile(path, raw, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := stageArchive(context.Background(), path, t.TempDir(), "1.2.0", "18.6"); err == nil {
		t.Fatal("corrupted archive accepted")
	}
}

func TestRestoreVersionCompatibility(t *testing.T) {
	for _, test := range []struct {
		source, target string
		compatible     bool
	}{
		{"1.1.9", "1.2.0", true}, {"1.2.0-dev+old", "1.2.0-dev+new", true},
		{"1.2.0-dev", "1.2.0", true}, {"1.2.1", "1.2.0", false},
		{"2.0.0", "1.9.9", false}, {"1.2.0", "1.2.0-dev", false},
		{"1.2.0-alpha", "1.2.0-beta", false}, {"invalid", "1.2.0", false},
	} {
		err := compatibleAppVersion(test.source, test.target)
		if (err == nil) != test.compatible {
			t.Errorf("compatibleAppVersion(%q, %q) = %v", test.source, test.target, err)
		}
	}
	latest, err := latestMigration()
	if err != nil {
		t.Fatal(err)
	}
	metadata := Metadata{AppVersion: "1.2.0", SchemaMigration: latest,
		CreatedAt: time.Now().UTC(), PgVersion: "19.0", FileCount: 0}
	if err := validateMetadata(metadata, "1.2.0", "18.6"); err == nil {
		t.Fatal("newer PostgreSQL backup accepted")
	}
	metadata.PgVersion = "18.6"
	if err := validateMetadata(metadata, "1.2.0", "18.6"); err != nil {
		t.Fatalf("current migration rejected: %v", err)
	}
	metadata.SchemaMigration = latest + 1
	if err := validateMetadata(metadata, "1.2.0", "18.6"); err == nil {
		t.Fatal("newer migration accepted")
	}
}

func TestPostgresCommandHidesPassword(t *testing.T) {
	connection, env, err := postgresConnection("postgres://user:secret%40pass@localhost/db?sslmode=disable")
	if err != nil || strings.Contains(connection, "secret") || strings.Contains(connection, "pass") {
		t.Fatalf("connection was not sanitized: %v", err)
	}
	found := false
	for _, value := range env {
		found = found || value == "PGPASSWORD=secret@pass"
	}
	if !found {
		t.Fatal("password was not supplied through the subprocess environment")
	}
	if _, _, err := postgresConnection("postgres://localhost/db"); err != nil {
		t.Fatalf("connection without explicit username rejected: %v", err)
	}
}
