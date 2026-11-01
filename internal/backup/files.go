package backup

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"math"
	"os"
	"path"
	"path/filepath"
	"strings"
)

func storageDirectory(dataDir, name string) (string, error) {
	dir := filepath.Join(dataDir, name)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", fmt.Errorf("create %s directory: %w", name, err)
	}
	info, err := os.Lstat(dir)
	if err != nil {
		return "", fmt.Errorf("inspect %s directory: %w", name, err)
	}
	if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return "", fmt.Errorf("%s must be a non-symlink directory", name)
	}
	return dir, nil
}

func openRegular(root *os.Root, name string) (*os.File, os.FileInfo, error) {
	before, err := root.Lstat(name)
	if err != nil {
		return nil, nil, err
	}
	if !before.Mode().IsRegular() {
		return nil, nil, errors.New("storage entry is not a regular file")
	}
	file, err := root.Open(name)
	if err != nil {
		return nil, nil, err
	}
	after, err := file.Stat()
	if err != nil || !after.Mode().IsRegular() || !os.SameFile(before, after) {
		closeErr := file.Close()
		if err == nil {
			err = errors.New("storage entry changed while opening")
		}
		return nil, nil, errors.Join(err, closeErr)
	}
	return file, after, nil
}

func walkFiles(ctx context.Context, dir string, visit func(string, fs.FileInfo) error) (result error) {
	info, err := os.Lstat(dir)
	if err != nil {
		return err
	}
	if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return errors.New("attachment directory is not a non-symlink directory")
	}
	root, err := os.OpenRoot(dir)
	if err != nil {
		return err
	}
	defer func() { result = errors.Join(result, root.Close()) }()
	opened, err := root.Stat(".")
	if err != nil || !os.SameFile(info, opened) {
		if err != nil {
			return err
		}
		return errors.New("attachment directory changed while opening")
	}
	return fs.WalkDir(root.FS(), ".", func(name string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		if name == "." {
			return nil
		}
		if entry.IsDir() {
			return nil
		}
		if entry.Type()&fs.ModeSymlink != 0 {
			return errors.New("attachment storage contains a symlink")
		}
		base := path.Base(name)
		if strings.HasPrefix(base, ".upload-") || strings.HasPrefix(base, ".thumb-") {
			return nil
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		if !info.Mode().IsRegular() || info.Size() < 0 {
			return errors.New("attachment storage contains a non-regular file")
		}
		return visit(name, info)
	})
}

func fileSize(ctx context.Context, dir string) (total uint64, count int, err error) {
	err = walkFiles(ctx, dir, func(_ string, info fs.FileInfo) error {
		size := uint64(info.Size())
		if math.MaxUint64-total < size || count == math.MaxInt {
			return errors.New("attachment storage size overflow")
		}
		total += size
		count++
		return nil
	})
	return total, count, err
}

type contextReader struct {
	ctx      context.Context
	reader   io.Reader
	progress func(int)
}

func (reader contextReader) Read(buffer []byte) (int, error) {
	if err := reader.ctx.Err(); err != nil {
		return 0, err
	}
	n, err := reader.reader.Read(buffer)
	if reader.progress != nil && n > 0 {
		reader.progress(n)
	}
	return n, err
}
