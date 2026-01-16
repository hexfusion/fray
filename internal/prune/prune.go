package prune

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

var ErrDirNotFound = errors.New("directory not found")

// Result contains the outcome of a prune operation.
type Result struct {
	Files int
	Bytes int64
}

// Item represents a file or directory to be pruned.
type Item struct {
	Path  string
	Bytes int64
	// Files is the count of files in a directory.
	Files int
	IsDir bool
}

// Options configures the prune operation.
type Options struct {
	DryRun bool
	// OnItem is called for each item found.
	OnItem func(Item)
	// OnDelete is called after each delete attempt. Error is nil on dry-run.
	OnDelete func(Item, error)
}

// Run prunes incomplete downloads and state from an OCI layout directory.
func Run(dir string, opts Options) (*Result, error) {
	if _, err := os.Stat(dir); os.IsNotExist(err) {
		return nil, fmt.Errorf("%w: %s", ErrDirNotFound, dir)
	}

	result := &Result{}

	// clean partial blob downloads
	blobDir := filepath.Join(dir, "blobs", "sha256")
	if entries, err := os.ReadDir(blobDir); err == nil {
		for _, e := range entries {
			if e.IsDir() {
				continue
			}
			if !strings.HasSuffix(e.Name(), ".partial") {
				continue
			}

			path := filepath.Join(blobDir, e.Name())
			info, err := e.Info()
			if err != nil {
				continue
			}

			item := Item{
				Path:  path,
				Bytes: info.Size(),
				IsDir: false,
			}

			result.Files++
			result.Bytes += info.Size()

			if opts.OnItem != nil {
				opts.OnItem(item)
			}

			if !opts.DryRun {
				err := os.Remove(path)
				if opts.OnDelete != nil {
					opts.OnDelete(item, err)
				}
			}
		}
	}

	// clean layer state directories
	stateDir := filepath.Join(dir, ".fray")
	if entries, err := os.ReadDir(stateDir); err == nil {
		for _, e := range entries {
			if !e.IsDir() {
				continue
			}

			layerDir := filepath.Join(stateDir, e.Name())
			dirSize, dirFiles := calcDirSize(layerDir)

			item := Item{
				Path:  layerDir,
				Bytes: dirSize,
				Files: dirFiles,
				IsDir: true,
			}

			result.Files += dirFiles
			result.Bytes += dirSize

			if opts.OnItem != nil {
				opts.OnItem(item)
			}

			if !opts.DryRun {
				err := os.RemoveAll(layerDir)
				if opts.OnDelete != nil {
					opts.OnDelete(item, err)
				}
			}
		}
	}

	return result, nil
}

func calcDirSize(path string) (int64, int) {
	var size int64
	var count int

	_ = filepath.Walk(path, func(_ string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		if !info.IsDir() {
			size += info.Size()
			count++
		}
		return nil
	})

	return size, count
}

// HumanBytes formats bytes as human-readable string.
func HumanBytes(b int64) string {
	const unit = 1024
	if b < unit {
		return formatBytes(b, 0, 'B')
	}
	div, exp := int64(unit), 0
	for n := b / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return formatBytesFloat(float64(b)/float64(div), "KMGTPE"[exp])
}

func formatBytes(b int64, _ int, suffix byte) string {
	buf := make([]byte, 0, 8)
	buf = appendInt(buf, b)
	buf = append(buf, ' ', suffix)
	return string(buf)
}

func formatBytesFloat(f float64, suffix byte) string {
	buf := make([]byte, 0, 8)
	buf = appendFloat(buf, f, 1)
	buf = append(buf, ' ', suffix, 'B')
	return string(buf)
}

func appendInt(buf []byte, n int64) []byte {
	if n == 0 {
		return append(buf, '0')
	}
	if n < 0 {
		buf = append(buf, '-')
		n = -n
	}
	var tmp [20]byte
	i := len(tmp)
	for n > 0 {
		i--
		tmp[i] = byte('0' + n%10)
		n /= 10
	}
	return append(buf, tmp[i:]...)
}

func appendFloat(buf []byte, f float64, prec int) []byte {
	intPart := int64(f)
	buf = appendInt(buf, intPart)
	buf = append(buf, '.')
	frac := f - float64(intPart)
	for range prec {
		frac *= 10
		buf = append(buf, byte('0'+int(frac)%10))
	}
	return buf
}
