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
	"time"
)

type Metadata struct {
	AppVersion      string    `json:"appVersion"`
	SchemaMigration int64     `json:"schemaMigration"`
	CreatedAt       time.Time `json:"createdAt"`
	PgVersion       string    `json:"pgVersion"`
	FileCount       int       `json:"fileCount"`
}

func writeArchive(ctx context.Context, destination, dumpPath, filesDir string, metadata Metadata,
	progress func(int)) (size int64, result error) {
	filesBytes, _, err := fileSize(ctx, filesDir)
	if err != nil {
		return 0, fmt.Errorf("scan attachments: %w", err)
	}
	dump, err := os.Open(dumpPath)
	if err != nil {
		return 0, fmt.Errorf("open database dump: %w", err)
	}
	defer func() { result = errors.Join(result, dump.Close()) }()
	dumpInfo, err := dump.Stat()
	if err != nil || !dumpInfo.Mode().IsRegular() || dumpInfo.Size() <= 0 {
		return 0, errors.New("database dump is missing or empty")
	}
	if math.MaxUint64-filesBytes < uint64(dumpInfo.Size()) {
		return 0, errors.New("backup input size overflow")
	}
	total := filesBytes + uint64(dumpInfo.Size())
	var copied uint64
	update := func(n int) {
		copied += uint64(n)
		if total > 0 {
			percent := 10 + int(float64(copied)*85/float64(total))
			if percent > 95 {
				percent = 95
			}
			progress(percent)
		}
	}
	output, err := os.OpenFile(destination, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return 0, fmt.Errorf("create partial backup: %w", err)
	}
	defer func() { result = errors.Join(result, output.Close()) }()
	writer := zip.NewWriter(output)
	closed := false
	defer func() {
		if !closed {
			result = errors.Join(result, writer.Close())
		}
	}()
	dumpEntry, err := writer.CreateHeader(archiveHeader("db.dump", dumpInfo.ModTime(), false))
	if err != nil {
		return 0, fmt.Errorf("create dump entry: %w", err)
	}
	if _, err := io.Copy(dumpEntry, contextReader{ctx: ctx, reader: dump, progress: update}); err != nil {
		return 0, fmt.Errorf("write dump entry: %w", err)
	}
	if _, err := writer.CreateHeader(archiveHeader("files/", metadata.CreatedAt, true)); err != nil {
		return 0, fmt.Errorf("create attachments entry: %w", err)
	}
	root, err := os.OpenRoot(filesDir)
	if err != nil {
		return 0, fmt.Errorf("open attachment storage: %w", err)
	}
	defer func() { result = errors.Join(result, root.Close()) }()
	metadata.FileCount = 0
	err = walkFiles(ctx, filesDir, func(name string, expected fs.FileInfo) (entryErr error) {
		file, info, err := openRegular(root, name)
		if err != nil {
			return fmt.Errorf("open attachment: %w", err)
		}
		defer func() { entryErr = errors.Join(entryErr, file.Close()) }()
		if !os.SameFile(expected, info) || expected.Size() != info.Size() {
			return errors.New("attachment changed during backup")
		}
		entry, err := writer.CreateHeader(archiveHeader("files/"+name, info.ModTime(), false))
		if err != nil {
			return fmt.Errorf("create attachment entry: %w", err)
		}
		written, err := io.Copy(entry, contextReader{ctx: ctx, reader: file, progress: update})
		if err != nil {
			return fmt.Errorf("write attachment entry: %w", err)
		}
		after, err := file.Stat()
		if err != nil || written != info.Size() || after.Size() != info.Size() || !after.ModTime().Equal(info.ModTime()) {
			return errors.New("attachment changed during backup")
		}
		metadata.FileCount++
		return nil
	})
	if err != nil {
		return 0, fmt.Errorf("archive attachments: %w", err)
	}
	manifest, err := writer.CreateHeader(archiveHeader("backup.json", metadata.CreatedAt, false))
	if err != nil {
		return 0, fmt.Errorf("create backup metadata: %w", err)
	}
	if err := json.NewEncoder(manifest).Encode(metadata); err != nil {
		return 0, fmt.Errorf("write backup metadata: %w", err)
	}
	if err := writer.Close(); err != nil {
		closed = true
		return 0, fmt.Errorf("finish backup archive: %w", err)
	}
	closed = true
	if err := output.Sync(); err != nil {
		return 0, fmt.Errorf("sync backup archive: %w", err)
	}
	info, err := output.Stat()
	if err != nil || info.Size() <= 0 {
		return 0, errors.New("completed backup archive is empty")
	}
	return info.Size(), nil
}

func archiveHeader(name string, modified time.Time, directory bool) *zip.FileHeader {
	header := &zip.FileHeader{Name: name, Method: zip.Deflate, Modified: modified.UTC()}
	if directory {
		header.SetMode(fs.ModeDir | 0o700)
		header.Method = zip.Store
	} else {
		header.SetMode(0o600)
	}
	return header
}
