package store

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

const (
	OCILayoutVersion = "1.0.0"
	BlobsDir         = "blobs"
	IndexFile        = "index.json"
	LayoutFile       = "oci-layout"
)

// Layout is an OCI Image Layout directory.
type Layout struct {
	root string
	mu   sync.RWMutex
}

// OCILayout is the oci-layout file content.
type OCILayout struct {
	ImageLayoutVersion string `json:"imageLayoutVersion"`
}

// Index is the index.json content.
type Index struct {
	SchemaVersion int          `json:"schemaVersion"`
	MediaType     string       `json:"mediaType,omitempty"`
	Manifests     []Descriptor `json:"manifests"`
}

// Descriptor describes a blob.
type Descriptor struct {
	MediaType   string            `json:"mediaType"`
	Digest      string            `json:"digest"`
	Size        int64             `json:"size"`
	Annotations map[string]string `json:"annotations,omitempty"`
	Platform    *Platform         `json:"platform,omitempty"`
}

// Platform describes a manifest's target platform.
type Platform struct {
	Architecture string `json:"architecture"`
	OS           string `json:"os"`
	Variant      string `json:"variant,omitempty"`
}

// Open opens or creates an OCI Image Layout.
func Open(root string) (*Layout, error) {
	l := &Layout{root: root}

	layoutPath := filepath.Join(root, LayoutFile)
	if _, err := os.Stat(layoutPath); err == nil {
		data, err := os.ReadFile(layoutPath)
		if err != nil {
			return nil, fmt.Errorf("read oci-layout: %w", err)
		}
		var layout OCILayout
		if err := json.Unmarshal(data, &layout); err != nil {
			return nil, fmt.Errorf("parse oci-layout: %w", err)
		}
		return l, nil
	}

	if err := l.init(); err != nil {
		return nil, err
	}
	return l, nil
}

func (l *Layout) init() error {
	dirs := []string{
		l.root,
		filepath.Join(l.root, BlobsDir, "sha256"),
	}
	for _, dir := range dirs {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return fmt.Errorf("create %s: %w", dir, err)
		}
	}

	layout := OCILayout{ImageLayoutVersion: OCILayoutVersion}
	data, err := json.Marshal(layout)
	if err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(l.root, LayoutFile), data, 0644); err != nil {
		return fmt.Errorf("write oci-layout: %w", err)
	}

	index := Index{
		SchemaVersion: 2,
		MediaType:     "application/vnd.oci.image.index.v1+json",
		Manifests:     []Descriptor{},
	}
	data, err = json.MarshalIndent(index, "", "  ")
	if err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(l.root, IndexFile), data, 0644); err != nil {
		return fmt.Errorf("write index.json: %w", err)
	}

	return nil
}

// Root returns the layout root directory.
func (l *Layout) Root() string {
	return l.root
}

// HasBlob reports whether a blob exists.
func (l *Layout) HasBlob(digest string) bool {
	_, err := os.Stat(l.blobPath(digest))
	return err == nil
}

// BlobSize returns the size of a blob, or -1 if not found.
func (l *Layout) BlobSize(digest string) int64 {
	info, err := os.Stat(l.blobPath(digest))
	if err != nil {
		return -1
	}
	return info.Size()
}

// OpenBlob opens a blob for reading.
func (l *Layout) OpenBlob(digest string) (io.ReadCloser, error) {
	return os.Open(l.blobPath(digest))
}

// ReadBlob reads the entire blob into memory.
func (l *Layout) ReadBlob(digest string) ([]byte, error) {
	return os.ReadFile(l.blobPath(digest))
}

// WriteBlob writes a blob. Returns 0 if blob already exists (deduplication).
func (l *Layout) WriteBlob(digest string, r io.Reader) (int64, error) {
	l.mu.Lock()
	defer l.mu.Unlock()

	path := l.blobPath(digest)

	if _, err := os.Stat(path); err == nil {
		return 0, nil
	}

	dir := filepath.Dir(path)
	tmp, err := os.CreateTemp(dir, ".blob-*")
	if err != nil {
		return 0, fmt.Errorf("create temp: %w", err)
	}
	tmpPath := tmp.Name()

	success := false
	defer func() {
		if !success {
			tmp.Close()
			os.Remove(tmpPath)
		}
	}()

	n, err := io.Copy(tmp, r)
	if err != nil {
		return 0, fmt.Errorf("write blob: %w", err)
	}

	if err := tmp.Close(); err != nil {
		return 0, fmt.Errorf("close temp: %w", err)
	}

	if err := os.Rename(tmpPath, path); err != nil {
		return 0, fmt.Errorf("rename blob: %w", err)
	}

	success = true
	return n, nil
}

// WriteBlobAt writes data at offset for resumable downloads.
func (l *Layout) WriteBlobAt(digest string, offset int64, data []byte) error {
	l.mu.Lock()
	defer l.mu.Unlock()

	path := l.blobPath(digest) + ".partial"

	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return fmt.Errorf("open partial: %w", err)
	}
	defer f.Close()

	if _, err := f.WriteAt(data, offset); err != nil {
		return fmt.Errorf("write at %d: %w", offset, err)
	}

	return nil
}

// ReadBlobAt reads data from a partial blob at the given offset.
func (l *Layout) ReadBlobAt(digest string, offset int64, length int) ([]byte, error) {
	l.mu.RLock()
	defer l.mu.RUnlock()

	path := l.blobPath(digest) + ".partial"

	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	data := make([]byte, length)
	n, err := f.ReadAt(data, offset)
	if err != nil && !errors.Is(err, io.EOF) {
		return nil, err
	}

	return data[:n], nil
}

// FinalizeBlob moves a partial blob to its final location.
func (l *Layout) FinalizeBlob(digest string) error {
	l.mu.Lock()
	defer l.mu.Unlock()

	partialPath := l.blobPath(digest) + ".partial"
	finalPath := l.blobPath(digest)

	if _, err := os.Stat(partialPath); err != nil {
		return fmt.Errorf("partial not found: %w", err)
	}

	if _, err := os.Stat(finalPath); err == nil {
		os.Remove(partialPath)
		return nil
	}

	if err := os.Rename(partialPath, finalPath); err != nil {
		return fmt.Errorf("finalize: %w", err)
	}

	return nil
}

// AddManifest adds or updates a manifest in the index.
func (l *Layout) AddManifest(desc Descriptor) error {
	l.mu.Lock()
	defer l.mu.Unlock()

	index, err := l.readIndex()
	if err != nil {
		return err
	}

	for i, m := range index.Manifests {
		if m.Digest == desc.Digest {
			index.Manifests[i] = desc
			return l.writeIndex(index)
		}
	}

	index.Manifests = append(index.Manifests, desc)
	return l.writeIndex(index)
}

// GetIndex returns the current index.
func (l *Layout) GetIndex() (*Index, error) {
	l.mu.RLock()
	defer l.mu.RUnlock()
	return l.readIndex()
}

func (l *Layout) readIndex() (*Index, error) {
	data, err := os.ReadFile(filepath.Join(l.root, IndexFile))
	if err != nil {
		return nil, fmt.Errorf("read index: %w", err)
	}

	var index Index
	if err := json.Unmarshal(data, &index); err != nil {
		return nil, fmt.Errorf("parse index: %w", err)
	}

	return &index, nil
}

func (l *Layout) writeIndex(index *Index) error {
	data, err := json.MarshalIndent(index, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(l.root, IndexFile), data, 0644)
}

func (l *Layout) blobPath(digest string) string {
	parts := strings.SplitN(digest, ":", 2)
	if len(parts) != 2 {
		return filepath.Join(l.root, BlobsDir, "sha256", digest)
	}
	return filepath.Join(l.root, BlobsDir, parts[0], parts[1])
}

// Stats contains storage statistics.
type Stats struct {
	BlobCount     int
	TotalSize     int64
	UniqueDigests int
}

// GetStats returns storage statistics.
func (l *Layout) GetStats() (Stats, error) {
	var stats Stats

	blobDir := filepath.Join(l.root, BlobsDir, "sha256")
	entries, err := os.ReadDir(blobDir)
	if err != nil {
		if os.IsNotExist(err) {
			return stats, nil
		}
		return stats, err
	}

	seen := make(map[string]bool, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || strings.HasSuffix(entry.Name(), ".partial") {
			continue
		}

		info, err := entry.Info()
		if err != nil {
			continue
		}

		stats.BlobCount++
		stats.TotalSize += info.Size()

		if !seen[entry.Name()] {
			seen[entry.Name()] = true
			stats.UniqueDigests++
		}
	}

	return stats, nil
}
