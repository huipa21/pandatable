package admin

import (
	"context"
	"errors"
	"io"
	"io/fs"
	"math"
	"os"
	"time"
)

var errScanLimit = errors.New("directory scan bound reached")
var errFileType = errors.New("unsupported storage entry")

type scanBounds struct {
	entries int
	depth   int
}

type directoryCache struct {
	expires        time.Time
	files, backups Metric[uint64]
}

func (cache *directoryCache) sizes(ctx context.Context, dataDir string) (Metric[uint64], Metric[uint64]) {
	now := time.Now().UTC()
	if now.Before(cache.expires) {
		return cache.files, cache.backups
	}
	for _, item := range []struct {
		name string
		dest *Metric[uint64]
	}{{"files", &cache.files}, {"backups", &cache.backups}} {
		started := time.Now().UTC()
		scanCtx, cancel := context.WithTimeout(ctx, time.Second)
		size, err := directorySize(scanCtx, dataDir, item.name, scanBounds{entries: 100_000, depth: 64})
		cancel()
		if err != nil {
			*item.dest = unavailable[uint64](started, filesystemReason(err))
		} else {
			*item.dest = ready(started, size)
		}
	}
	cache.expires = time.Now().Add(time.Hour)
	return cache.files, cache.backups
}

func filesystemReason(err error) string {
	switch {
	case errors.Is(err, context.Canceled), errors.Is(err, context.DeadlineExceeded):
		return "timeout"
	case errors.Is(err, fs.ErrNotExist):
		return "notFound"
	case errors.Is(err, fs.ErrPermission):
		return "permission"
	case errors.Is(err, errScanLimit):
		return "limit"
	case errors.Is(err, errFileType):
		return "invalid"
	default:
		return "io"
	}
}

func directorySize(ctx context.Context, dataDir, name string, bounds scanBounds) (size uint64, result error) {
	info, err := os.Lstat(dataDir)
	if err != nil {
		return 0, err
	}
	if !info.IsDir() {
		return 0, errFileType
	}
	root, err := os.OpenRoot(dataDir)
	if err != nil {
		return 0, err
	}
	defer func() { result = errors.Join(result, root.Close()) }()
	opened, err := root.Stat(".")
	if err != nil {
		return 0, err
	}
	if !os.SameFile(info, opened) {
		return 0, errFileType
	}
	child, err := openDirectory(root, name)
	if err != nil {
		return 0, err
	}
	defer func() { result = errors.Join(result, child.Close()) }()
	return walkSize(ctx, child, &bounds, 0)
}

func openDirectory(parent *os.Root, name string) (*os.Root, error) {
	info, err := parent.Lstat(name)
	if err != nil {
		return nil, err
	}
	if !info.IsDir() {
		return nil, errFileType
	}
	child, err := parent.OpenRoot(name)
	if err != nil {
		return nil, err
	}
	actual, err := child.Stat(".")
	if err != nil || !os.SameFile(info, actual) {
		_ = child.Close()
		if err != nil {
			return nil, err
		}
		return nil, errFileType
	}
	return child, nil
}

func walkSize(ctx context.Context, root *os.Root, bounds *scanBounds, depth int) (total uint64, result error) {
	if err := ctx.Err(); err != nil {
		return 0, err
	}
	if depth > bounds.depth {
		return 0, errScanLimit
	}
	dir, err := root.Open(".")
	if err != nil {
		return 0, err
	}
	defer func() { result = errors.Join(result, dir.Close()) }()
	for {
		if err := ctx.Err(); err != nil {
			return 0, err
		}
		entries, readErr := dir.ReadDir(128)
		if readErr != nil && !errors.Is(readErr, io.EOF) {
			return 0, readErr
		}
		for _, entry := range entries {
			bounds.entries--
			if bounds.entries < 0 {
				return 0, errScanLimit
			}
			if err := ctx.Err(); err != nil {
				return 0, err
			}
			info, err := root.Lstat(entry.Name())
			if err != nil {
				return 0, err
			}
			switch {
			case info.Mode()&fs.ModeSymlink != 0:
				continue
			case info.IsDir():
				child, err := openDirectory(root, entry.Name())
				if err != nil {
					return 0, err
				}
				size, walkErr := walkSize(ctx, child, bounds, depth+1)
				if err := errors.Join(walkErr, child.Close()); err != nil {
					return 0, err
				}
				if math.MaxUint64-total < size {
					return 0, errScanLimit
				}
				total += size
			case info.Mode().IsRegular() && info.Size() >= 0:
				size := uint64(info.Size())
				if math.MaxUint64-total < size {
					return 0, errScanLimit
				}
				total += size
			default:
				return 0, errFileType
			}
		}
		if errors.Is(readErr, io.EOF) {
			return total, nil
		}
	}
}
