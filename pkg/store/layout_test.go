package store

import (
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestLayoutCreate(t *testing.T) {
	require := require.New(t)
	dir := t.TempDir()

	_, err := Open(dir)
	require.NoError(err)

	data, err := os.ReadFile(filepath.Join(dir, "oci-layout"))
	require.NoError(err)
	require.Contains(string(data), `"imageLayoutVersion":"1.0.0"`)

	data, err = os.ReadFile(filepath.Join(dir, "index.json"))
	require.NoError(err)
	require.Contains(string(data), `"schemaVersion"`)

	_, err = os.Stat(filepath.Join(dir, "blobs", "sha256"))
	require.NoError(err)
}

func TestLayoutReopen(t *testing.T) {
	require := require.New(t)
	dir := t.TempDir()

	l1, err := Open(dir)
	require.NoError(err)

	digest := "sha256:abc123"
	content := "test blob content"
	_, err = l1.WriteBlob(digest, strings.NewReader(content))
	require.NoError(err)

	l2, err := Open(dir)
	require.NoError(err)
	require.True(l2.HasBlob(digest))
}

func TestBlobOperations(t *testing.T) {
	tests := []struct {
		name    string
		digest  string
		content string
	}{
		{"simple content", "sha256:simple", "hello world"},
		{"empty content", "sha256:empty", ""},
		{"large content", "sha256:large", strings.Repeat("x", 10000)},
		{"binary-like", "sha256:binary", "\x00\x01\x02\x03"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require := require.New(t)
			dir := t.TempDir()

			l, err := Open(dir)
			require.NoError(err)

			n, err := l.WriteBlob(tt.digest, strings.NewReader(tt.content))
			require.NoError(err)
			require.Equal(int64(len(tt.content)), n)

			require.True(l.HasBlob(tt.digest))

			reader, err := l.OpenBlob(tt.digest)
			require.NoError(err)
			defer reader.Close()

			data, err := io.ReadAll(reader)
			require.NoError(err)
			require.Equal(tt.content, string(data))
		})
	}
}

func TestBlobDeduplication(t *testing.T) {
	require := require.New(t)
	dir := t.TempDir()

	l, err := Open(dir)
	require.NoError(err)

	digest := "sha256:duplicate"
	content := "duplicate content"

	n1, err := l.WriteBlob(digest, strings.NewReader(content))
	require.NoError(err)
	require.Equal(int64(len(content)), n1)

	n2, err := l.WriteBlob(digest, strings.NewReader(content))
	require.NoError(err)
	require.Equal(int64(0), n2)

	stats, err := l.GetStats()
	require.NoError(err)
	require.Equal(1, stats.BlobCount)
}

func TestPartialBlob(t *testing.T) {
	require := require.New(t)
	dir := t.TempDir()

	l, err := Open(dir)
	require.NoError(err)

	digest := "sha256:partial"

	require.NoError(l.WriteBlobAt(digest, 0, []byte("chunk0")))
	require.NoError(l.WriteBlobAt(digest, 6, []byte("chunk1")))
	require.NoError(l.FinalizeBlob(digest))

	require.True(l.HasBlob(digest))

	reader, err := l.OpenBlob(digest)
	require.NoError(err)
	defer reader.Close()

	data, err := io.ReadAll(reader)
	require.NoError(err)
	require.Equal("chunk0chunk1", string(data))
}

func TestManifestIndex(t *testing.T) {
	require := require.New(t)
	dir := t.TempDir()

	l, err := Open(dir)
	require.NoError(err)

	desc := Descriptor{
		MediaType: "application/vnd.oci.image.manifest.v1+json",
		Digest:    "sha256:manifest1",
		Size:      1234,
		Annotations: map[string]string{
			"org.opencontainers.image.ref.name": "latest",
		},
	}

	require.NoError(l.AddManifest(desc))

	index, err := l.GetIndex()
	require.NoError(err)
	require.Len(index.Manifests, 1)
	require.Equal(desc.Digest, index.Manifests[0].Digest)

	desc2 := desc
	desc2.Size = 5678
	require.NoError(l.AddManifest(desc2))

	index, err = l.GetIndex()
	require.NoError(err)
	require.Len(index.Manifests, 1)
	require.Equal(int64(5678), index.Manifests[0].Size)
}

func TestManifestMultiple(t *testing.T) {
	require := require.New(t)
	dir := t.TempDir()

	l, err := Open(dir)
	require.NoError(err)

	manifests := []Descriptor{
		{
			MediaType:   "application/vnd.oci.image.manifest.v1+json",
			Digest:      "sha256:manifest1",
			Size:        100,
			Annotations: map[string]string{"org.opencontainers.image.ref.name": "v1.0"},
		},
		{
			MediaType:   "application/vnd.oci.image.manifest.v1+json",
			Digest:      "sha256:manifest2",
			Size:        200,
			Annotations: map[string]string{"org.opencontainers.image.ref.name": "v2.0"},
		},
	}

	for _, m := range manifests {
		require.NoError(l.AddManifest(m))
	}

	index, err := l.GetIndex()
	require.NoError(err)
	require.Len(index.Manifests, 2)
}

func TestReadBlob(t *testing.T) {
	require := require.New(t)
	dir := t.TempDir()

	l, err := Open(dir)
	require.NoError(err)

	digest := "sha256:readblob"
	content := "read blob test content"

	_, err = l.WriteBlob(digest, strings.NewReader(content))
	require.NoError(err)

	data, err := l.ReadBlob(digest)
	require.NoError(err)
	require.Equal(content, string(data))
}

func TestStats(t *testing.T) {
	require := require.New(t)
	dir := t.TempDir()

	l, err := Open(dir)
	require.NoError(err)

	stats, err := l.GetStats()
	require.NoError(err)
	require.Equal(0, stats.BlobCount)
	require.Equal(int64(0), stats.TotalSize)

	_, err = l.WriteBlob("sha256:blob1", strings.NewReader("content1"))
	require.NoError(err)
	_, err = l.WriteBlob("sha256:blob2", strings.NewReader("longer content 2"))
	require.NoError(err)

	stats, err = l.GetStats()
	require.NoError(err)
	require.Equal(2, stats.BlobCount)
	require.Equal(int64(len("content1")+len("longer content 2")), stats.TotalSize)
}
