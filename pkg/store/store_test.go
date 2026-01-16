package store

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestNewStore(t *testing.T) {
	tests := []struct {
		name            string
		opts            []Option
		wantChunkSize   int
		wantParallelism int
	}{
		{
			name:            "defaults",
			opts:            nil,
			wantChunkSize:   DefaultChunkSize,
			wantParallelism: 1,
		},
		{
			name:            "custom chunk size",
			opts:            []Option{WithChunkSize(512 * 1024)},
			wantChunkSize:   512 * 1024,
			wantParallelism: 1,
		},
		{
			name:            "custom parallelism",
			opts:            []Option{WithParallelism(4)},
			wantChunkSize:   DefaultChunkSize,
			wantParallelism: 4,
		},
		{
			name:            "both options",
			opts:            []Option{WithChunkSize(2 * 1024 * 1024), WithParallelism(8)},
			wantChunkSize:   2 * 1024 * 1024,
			wantParallelism: 8,
		},
		{
			name:            "invalid chunk size ignored",
			opts:            []Option{WithChunkSize(0)},
			wantChunkSize:   DefaultChunkSize,
			wantParallelism: 1,
		},
		{
			name:            "invalid parallelism ignored",
			opts:            []Option{WithParallelism(-1)},
			wantChunkSize:   DefaultChunkSize,
			wantParallelism: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require := require.New(t)

			s := New(t.TempDir(), tt.opts...)

			require.Equal(tt.wantChunkSize, s.chunkSize)
			require.Equal(tt.wantParallelism, s.parallelism)
		})
	}
}

func TestGetOrCreateLayer(t *testing.T) {
	require := require.New(t)

	s := New(t.TempDir())
	digest := "sha256:abc123"
	size := int64(3 * 1024 * 1024)

	layer, err := s.GetOrCreateLayer(digest, size)
	require.NoError(err)
	require.Equal(digest, layer.Digest)
	require.Equal(size, layer.Size)
	require.NotNil(layer.Tree)
	require.Equal(3, layer.Tree.NumChunks)
}

func TestGetOrCreateLayerResume(t *testing.T) {
	require := require.New(t)

	root := t.TempDir()
	s := New(root)

	digest := "sha256:resume123"
	size := int64(2 * 1024 * 1024)

	layer1, err := s.GetOrCreateLayer(digest, size)
	require.NoError(err)

	require.NoError(layer1.Tree.SetChunk(0, []byte("chunk data")))
	require.NoError(s.SaveState(layer1))

	layer2, err := s.GetOrCreateLayer(digest, size)
	require.NoError(err)
	require.True(layer2.Tree.HasChunk(0))
	require.False(layer2.Tree.HasChunk(1))
}

func TestAssembleBlob(t *testing.T) {
	require := require.New(t)

	root := t.TempDir()
	s := New(root, WithChunkSize(10))

	content := "hello world test content"
	hasher := sha256.New()
	hasher.Write([]byte(content))
	digest := "sha256:" + hex.EncodeToString(hasher.Sum(nil))

	layer, err := s.GetOrCreateLayer(digest, int64(len(content)))
	require.NoError(err)

	// write chunks manually
	for i := 0; i < layer.Tree.NumChunks; i++ {
		start := i * 10
		end := min(start+10, len(content))
		chunk := []byte(content[start:end])

		chunkPath := filepath.Join(layer.StorePath, chunkfmt(i))
		require.NoError(os.WriteFile(chunkPath, chunk, 0644))
		require.NoError(layer.Tree.SetChunk(i, chunk))
	}

	blobPath, err := s.AssembleBlob(layer)
	require.NoError(err)

	data, err := os.ReadFile(blobPath)
	require.NoError(err)
	require.Equal(content, string(data))
}

func TestAssembleBlobIncomplete(t *testing.T) {
	require := require.New(t)

	s := New(t.TempDir(), WithChunkSize(10))
	digest := "sha256:incomplete"
	size := int64(30)

	layer, err := s.GetOrCreateLayer(digest, size)
	require.NoError(err)

	_, err = s.AssembleBlob(layer)
	require.Error(err)
	require.True(errors.Is(err, ErrLayerIncomplete))
}

func TestCleanupChunks(t *testing.T) {
	require := require.New(t)

	root := t.TempDir()
	s := New(root, WithChunkSize(10))

	digest := "sha256:cleanup"
	size := int64(25)

	layer, err := s.GetOrCreateLayer(digest, size)
	require.NoError(err)

	// create chunk files
	for i := 0; i < layer.Tree.NumChunks; i++ {
		chunkPath := filepath.Join(layer.StorePath, chunkfmt(i))
		require.NoError(os.WriteFile(chunkPath, []byte("data"), 0644))
	}

	require.NoError(s.CleanupChunks(layer))

	// verify chunks are gone
	for i := 0; i < layer.Tree.NumChunks; i++ {
		chunkPath := filepath.Join(layer.StorePath, chunkfmt(i))
		_, err := os.Stat(chunkPath)
		require.True(os.IsNotExist(err))
	}
}

func TestBlobPath(t *testing.T) {
	require := require.New(t)

	root := t.TempDir()
	s := New(root)
	digest := "sha256:blobpath123"

	// no blob yet
	require.Empty(s.BlobPath(digest))

	// create fake blob
	layer, err := s.GetOrCreateLayer(digest, 100)
	require.NoError(err)

	blobPath := filepath.Join(layer.StorePath, "blob")
	require.NoError(os.WriteFile(blobPath, []byte("blob"), 0644))

	require.Equal(blobPath, s.BlobPath(digest))
}

func chunkfmt(i int) string {
	return "chunk-" + padInt(i, 5)
}

func padInt(n, width int) string {
	s := ""
	for range width {
		s = string(rune('0'+n%10)) + s
		n /= 10
	}
	return s
}
