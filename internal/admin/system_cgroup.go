package admin

import (
	"context"
	"errors"
	"io"
	"io/fs"
	"path"
	"strconv"
	"strings"
	"time"
)

var errCgroupUnsupported = errors.New("cgroup v2 unavailable")

func boundedRead(ctx context.Context, files fs.FS, name string, limit int64) (text string, result error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	file, err := files.Open(name)
	if err != nil {
		return "", err
	}
	defer func() { result = errors.Join(result, file.Close()) }()
	data, err := io.ReadAll(io.LimitReader(file, limit+1))
	if err != nil {
		return "", err
	}
	if int64(len(data)) > limit {
		return "", errScanLimit
	}
	return strings.TrimSpace(string(data)), nil
}

func cgroupLocation(membership, mounts string) (string, string, error) {
	var group string
	for _, line := range strings.Split(membership, "\n") {
		if strings.HasPrefix(line, "0::/") {
			group = strings.TrimPrefix(line, "0::")
		}
	}
	if group == "" {
		return "", "", errCgroupUnsupported
	}
	for _, line := range strings.Split(mounts, "\n") {
		before, after, ok := strings.Cut(line, " - ")
		if !ok || !strings.HasPrefix(after, "cgroup2 ") {
			continue
		}
		fields := strings.Fields(before)
		if len(fields) < 6 {
			return "", "", errFileType
		}
		unescape := strings.NewReplacer(`\040`, " ", `\011`, "\t", `\012`, "\n", `\134`, `\`)
		root, mount := unescape.Replace(fields[3]), unescape.Replace(fields[4])
		if !strings.HasPrefix(mount, "/") || path.Clean(group) != group || path.Clean(root) != root {
			return "", "", errFileType
		}
		if group == root {
			return strings.TrimPrefix(mount, "/"), ".", nil
		}
		prefix := strings.TrimSuffix(root, "/") + "/"
		if strings.HasPrefix(group, prefix) {
			return strings.TrimPrefix(mount, "/"), strings.TrimPrefix(group, prefix), nil
		}
	}
	return "", "", errCgroupUnsupported
}

func readContainerMemory(ctx context.Context, at time.Time, files fs.FS) Metric[ContainerMemory] {
	collect := func() (ContainerMemory, error) {
		membership, err := boundedRead(ctx, files, "proc/self/cgroup", 4096)
		if err != nil {
			return ContainerMemory{}, err
		}
		mounts, err := boundedRead(ctx, files, "proc/self/mountinfo", 65536)
		if err != nil {
			return ContainerMemory{}, err
		}
		mount, relative, err := cgroupLocation(membership, mounts)
		if err != nil {
			return ContainerMemory{}, err
		}
		current, err := boundedRead(ctx, files, path.Join(mount, relative, "memory.current"), 4096)
		if err != nil {
			return ContainerMemory{}, err
		}
		used, err := strconv.ParseUint(current, 10, 64)
		if err != nil {
			return ContainerMemory{}, errFileType
		}
		result := ContainerMemory{CurrentBytes: used, Unlimited: true}
		for depth := 0; ; depth++ {
			if depth > 64 {
				return ContainerMemory{}, errScanLimit
			}
			value, err := boundedRead(ctx, files, path.Join(mount, relative, "memory.max"), 4096)
			// The actual hierarchy root has no limit knob; namespace roots may have one.
			if errors.Is(err, fs.ErrNotExist) && relative == "." {
				return result, nil
			}
			if err != nil {
				return ContainerMemory{}, err
			}
			if value != "max" {
				limit, err := strconv.ParseUint(value, 10, 64)
				if err != nil || limit == 0 {
					return ContainerMemory{}, errFileType
				}
				if result.LimitBytes == nil || limit < *result.LimitBytes {
					result.LimitBytes = &limit
					result.Unlimited = false
				}
			}
			if relative == "." {
				return result, nil
			}
			relative = path.Dir(relative)
		}
	}
	result, err := collect()
	if errors.Is(err, errCgroupUnsupported) {
		return unavailable[ContainerMemory](at, "unsupported")
	}
	if err != nil {
		return unavailable[ContainerMemory](at, filesystemReason(err))
	}
	return ready(at, result)
}
