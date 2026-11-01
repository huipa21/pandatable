package backup

import (
	"archive/zip"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"math"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/shirou/gopsutil/v4/disk"

	"pandatable/migrations"
)

const maxManifestBytes = 64 << 10

var semverPattern = regexp.MustCompile(`^v?(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)(?:-([0-9A-Za-z.-]+))?(?:\+[0-9A-Za-z.-]+)?$`)
var postgresVersionPattern = regexp.MustCompile(`^([0-9]+)(?:\.|$)`)

type appVersion struct {
	major, minor, patch uint64
	prerelease          string
}

func parseAppVersion(value string) (appVersion, error) {
	matches := semverPattern.FindStringSubmatch(value)
	if matches == nil {
		return appVersion{}, errors.New("invalid application version")
	}
	parts := [3]uint64{}
	for index := range parts {
		number, err := strconv.ParseUint(matches[index+1], 10, 64)
		if err != nil {
			return appVersion{}, errors.New("invalid application version")
		}
		parts[index] = number
	}
	for _, identifier := range strings.Split(matches[4], ".") {
		if matches[4] != "" && identifier == "" {
			return appVersion{}, errors.New("invalid application prerelease")
		}
	}
	return appVersion{major: parts[0], minor: parts[1], patch: parts[2], prerelease: matches[4]}, nil
}

func compatibleAppVersion(source, target string) error {
	from, err := parseAppVersion(source)
	if err != nil {
		return fmt.Errorf("backup %w", err)
	}
	to, err := parseAppVersion(target)
	if err != nil {
		return fmt.Errorf("current %w", err)
	}
	if from.major != to.major || from.minor > to.minor ||
		from.minor == to.minor && from.patch > to.patch ||
		from.minor == to.minor && from.patch == to.patch && to.prerelease != "" && from.prerelease != to.prerelease {
		return errors.New("backup application version is not compatible with this application")
	}
	return nil
}

func postgresMajor(value string) (uint64, error) {
	match := postgresVersionPattern.FindStringSubmatch(strings.TrimSpace(value))
	if match == nil {
		return 0, errors.New("invalid PostgreSQL version")
	}
	major, err := strconv.ParseUint(match[1], 10, 64)
	if err != nil || major < 1 {
		return 0, errors.New("invalid PostgreSQL version")
	}
	return major, nil
}

func latestMigration() (int64, error) {
	entries, err := fs.ReadDir(migrations.FS, ".")
	if err != nil {
		return 0, fmt.Errorf("read embedded migrations: %w", err)
	}
	var latest int64
	for _, entry := range entries {
		prefix, _, ok := strings.Cut(entry.Name(), "_")
		if !ok || !strings.HasSuffix(entry.Name(), ".sql") {
			continue
		}
		version, err := strconv.ParseInt(prefix, 10, 64)
		if err != nil || version < 1 {
			return 0, errors.New("invalid embedded migration filename")
		}
		if version > latest {
			latest = version
		}
	}
	if latest == 0 {
		return 0, errors.New("no embedded migrations found")
	}
	return latest, nil
}

func validateMetadata(metadata Metadata, targetVersion, targetPostgres string) error {
	if metadata.CreatedAt.IsZero() || metadata.FileCount < 0 || metadata.SchemaMigration < 1 {
		return errors.New("invalid backup metadata")
	}
	if err := compatibleAppVersion(metadata.AppVersion, targetVersion); err != nil {
		return err
	}
	latest, err := latestMigration()
	if err != nil {
		return err
	}
	if metadata.SchemaMigration > latest {
		return errors.New("backup schema is newer than the embedded migrations")
	}
	sourceMajor, err := postgresMajor(metadata.PgVersion)
	if err != nil {
		return err
	}
	targetMajor, err := postgresMajor(targetPostgres)
	if err != nil {
		return err
	}
	if sourceMajor > targetMajor {
		return errors.New("backup PostgreSQL version is newer than the target server")
	}
	return nil
}

func stageArchive(ctx context.Context, archivePath, stage, targetVersion, targetPostgres string) (metadata Metadata, result error) {
	info, err := os.Lstat(archivePath)
	if err != nil {
		return Metadata{}, fmt.Errorf("inspect backup archive: %w", err)
	}
	if !info.Mode().IsRegular() || info.Size() <= 0 {
		return Metadata{}, errors.New("backup archive must be a non-empty regular file")
	}
	input, err := os.Open(archivePath)
	if err != nil {
		return Metadata{}, fmt.Errorf("open backup archive: %w", err)
	}
	defer func() { result = errors.Join(result, input.Close()) }()
	actual, err := input.Stat()
	if err != nil || !os.SameFile(info, actual) || actual.Size() != info.Size() {
		return Metadata{}, errors.New("backup archive changed while opening")
	}
	reader, err := zip.NewReader(input, info.Size())
	if err != nil {
		return Metadata{}, fmt.Errorf("read backup ZIP: %w", err)
	}
	seen := make(map[string]bool, len(reader.File))
	var manifest *zip.File
	var hasDump, hasFiles bool
	var total uint64
	fileCount := 0
	for _, entry := range reader.File {
		name := entry.Name
		isDir := strings.HasSuffix(name, "/")
		clean := strings.TrimSuffix(name, "/")
		if name == "" || !fs.ValidPath(clean) || strings.ContainsAny(name, `\:`) || !filepath.IsLocal(filepath.FromSlash(clean)) ||
			strings.Count(clean, "/") > 64 || entry.Flags&1 != 0 ||
			(entry.Method != zip.Store && entry.Method != zip.Deflate) {
			return Metadata{}, errors.New("backup ZIP contains an invalid entry")
		}
		key := strings.ToLower(clean)
		if seen[key] {
			return Metadata{}, errors.New("backup ZIP contains duplicate entries")
		}
		seen[key] = true
		mode := entry.Mode()
		if isDir {
			if mode&fs.ModeType != 0 && !mode.IsDir() {
				return Metadata{}, errors.New("backup ZIP contains an invalid directory")
			}
		} else if !mode.IsRegular() {
			return Metadata{}, errors.New("backup ZIP contains a non-regular file")
		}
		switch {
		case name == "backup.json" && !isDir:
			manifest = entry
			if entry.UncompressedSize64 > maxManifestBytes {
				return Metadata{}, errors.New("backup metadata is too large")
			}
		case name == "db.dump" && !isDir:
			hasDump = entry.UncompressedSize64 > 0
		case name == "files/" && isDir:
			hasFiles = true
		case strings.HasPrefix(name, "files/"):
			if !isDir {
				fileCount++
			}
		default:
			return Metadata{}, errors.New("backup ZIP contains an unexpected entry")
		}
		if math.MaxUint64-total < entry.UncompressedSize64 {
			return Metadata{}, errors.New("backup ZIP size overflow")
		}
		total += entry.UncompressedSize64
	}
	if manifest == nil || !hasDump || !hasFiles {
		return Metadata{}, errors.New("backup ZIP is missing required entries")
	}
	source, err := manifest.Open()
	if err != nil {
		return Metadata{}, fmt.Errorf("open backup metadata: %w", err)
	}
	var manifestData struct {
		AppVersion      *string    `json:"appVersion"`
		SchemaMigration *int64     `json:"schemaMigration"`
		CreatedAt       *time.Time `json:"createdAt"`
		PgVersion       *string    `json:"pgVersion"`
		FileCount       *int       `json:"fileCount"`
	}
	decoder := json.NewDecoder(io.LimitReader(source, maxManifestBytes+1))
	decoder.DisallowUnknownFields()
	err = decoder.Decode(&manifestData)
	if err == nil {
		var extra any
		if trailing := decoder.Decode(&extra); !errors.Is(trailing, io.EOF) {
			err = errors.New("backup metadata must contain one JSON object")
		}
	}
	err = errors.Join(err, source.Close())
	if err != nil {
		return Metadata{}, fmt.Errorf("read backup metadata: %w", err)
	}
	if manifestData.AppVersion == nil || manifestData.SchemaMigration == nil || manifestData.CreatedAt == nil ||
		manifestData.PgVersion == nil || manifestData.FileCount == nil {
		return Metadata{}, errors.New("backup metadata is missing required fields")
	}
	metadata = Metadata{AppVersion: *manifestData.AppVersion, SchemaMigration: *manifestData.SchemaMigration,
		CreatedAt: *manifestData.CreatedAt, PgVersion: *manifestData.PgVersion, FileCount: *manifestData.FileCount}
	if metadata.FileCount != fileCount {
		return Metadata{}, errors.New("backup attachment count does not match metadata")
	}
	if err := validateMetadata(metadata, targetVersion, targetPostgres); err != nil {
		return Metadata{}, err
	}
	usage, err := disk.UsageWithContext(ctx, stage)
	if err != nil {
		return Metadata{}, fmt.Errorf("measure restore disk space: %w", err)
	}
	if total > usage.Free {
		return Metadata{}, errors.New("insufficient disk space to stage restore")
	}
	for _, entry := range reader.File {
		if entry.Name == "backup.json" {
			continue
		}
		if err := extractEntry(ctx, stage, entry); err != nil {
			return Metadata{}, fmt.Errorf("extract backup entry: %w", err)
		}
	}
	return metadata, nil
}

func extractEntry(ctx context.Context, stage string, entry *zip.File) (result error) {
	destination := filepath.Join(stage, filepath.FromSlash(entry.Name))
	if strings.HasSuffix(entry.Name, "/") {
		return os.MkdirAll(destination, 0o700)
	}
	if err := os.MkdirAll(filepath.Dir(destination), 0o700); err != nil {
		return err
	}
	input, err := entry.Open()
	if err != nil {
		return err
	}
	defer func() { result = errors.Join(result, input.Close()) }()
	output, err := os.OpenFile(destination, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	defer func() { result = errors.Join(result, output.Close()) }()
	if _, err := io.Copy(output, contextReader{ctx: ctx, reader: input}); err != nil {
		return err
	}
	return output.Sync()
}
