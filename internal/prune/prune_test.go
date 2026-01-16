package prune

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestRun(t *testing.T) {
	require := require.New(t)

	dir := t.TempDir()
	setupLayout(t, dir)

	// create partial files
	blobDir := filepath.Join(dir, "blobs", "sha256")
	require.NoError(os.WriteFile(filepath.Join(blobDir, "abc.partial"), []byte("partial"), 0644))

	// create state dir
	stateDir := filepath.Join(dir, ".fray", "layer1")
	require.NoError(os.MkdirAll(stateDir, 0755))
	require.NoError(os.WriteFile(filepath.Join(stateDir, "chunk"), []byte("data"), 0644))

	result, err := Run(dir, Options{})
	require.NoError(err)
	require.Equal(2, result.Files)
	require.Equal(int64(11), result.Bytes) // 7 + 4

	// verify files removed
	_, err = os.Stat(filepath.Join(blobDir, "abc.partial"))
	require.True(os.IsNotExist(err))
}

func TestRunDryRun(t *testing.T) {
	require := require.New(t)

	dir := t.TempDir()
	setupLayout(t, dir)

	blobDir := filepath.Join(dir, "blobs", "sha256")
	partialFile := filepath.Join(blobDir, "test.partial")
	require.NoError(os.WriteFile(partialFile, []byte("data"), 0644))

	result, err := Run(dir, Options{DryRun: true})
	require.NoError(err)
	require.Equal(1, result.Files)

	// file should still exist
	_, err = os.Stat(partialFile)
	require.NoError(err)
}

func TestRunCallbacks(t *testing.T) {
	require := require.New(t)

	dir := t.TempDir()
	setupLayout(t, dir)

	blobDir := filepath.Join(dir, "blobs", "sha256")
	require.NoError(os.WriteFile(filepath.Join(blobDir, "cb.partial"), []byte("x"), 0644))

	var items []Item
	var deleted []Item

	opts := Options{
		OnItem: func(item Item) {
			items = append(items, item)
		},
		OnDelete: func(item Item, err error) {
			require.NoError(err)
			deleted = append(deleted, item)
		},
	}

	_, err := Run(dir, opts)
	require.NoError(err)
	require.Len(items, 1)
	require.Len(deleted, 1)
}

func TestRunMissingDir(t *testing.T) {
	require := require.New(t)

	result, err := Run("/nonexistent/path", Options{})
	require.Nil(result)
	require.Error(err)
	require.True(errors.Is(err, ErrDirNotFound))
}

func TestHumanBytes(t *testing.T) {
	tests := []struct {
		bytes int64
		want  string
	}{
		{0, "0 B"},
		{100, "100 B"},
		{1024, "1.0 KB"},
		{1536, "1.5 KB"},
		{1048576, "1.0 MB"},
		{1073741824, "1.0 GB"},
	}

	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			require := require.New(t)
			require.Equal(tt.want, HumanBytes(tt.bytes))
		})
	}
}

func setupLayout(t *testing.T, dir string) {
	t.Helper()
	require := require.New(t)

	require.NoError(os.MkdirAll(filepath.Join(dir, "blobs", "sha256"), 0755))
	require.NoError(os.WriteFile(filepath.Join(dir, "oci-layout"), []byte(`{"imageLayoutVersion":"1.0.0"}`), 0644))
	require.NoError(os.WriteFile(filepath.Join(dir, "index.json"), []byte(`{"schemaVersion":2,"manifests":[]}`), 0644))
}
