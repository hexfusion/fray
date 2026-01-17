package proxy

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/hexfusion/fray/pkg/logging"
	"github.com/hexfusion/fray/pkg/oci"
	"github.com/hexfusion/fray/pkg/store"
)

func TestDefaultOptions(t *testing.T) {
	require := require.New(t)

	opts := DefaultOptions()

	require.Equal(DefaultChunkSize, opts.ChunkSize)
	require.Equal(DefaultParallel, opts.Parallel)
	require.Equal(DefaultPullTimeout, opts.PullTimeout)
}

func TestNewServer(t *testing.T) {
	tests := []struct {
		name            string
		opts            Options
		wantChunkSize   int
		wantParallel    int
		wantPullTimeout int
	}{
		{
			name:            "zero values get defaults",
			opts:            Options{},
			wantChunkSize:   DefaultChunkSize,
			wantParallel:    DefaultParallel,
			wantPullTimeout: DefaultPullTimeout,
		},
		{
			name: "custom values preserved",
			opts: Options{
				ChunkSize:   512 * 1024,
				Parallel:    8,
				PullTimeout: 60 * 60,
			},
			wantChunkSize:   512 * 1024,
			wantParallel:    8,
			wantPullTimeout: 60 * 60,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require := require.New(t)

			dir := t.TempDir()
			l, err := store.Open(dir)
			require.NoError(err)

			client := oci.NewClient()
			s := New(l, client, logging.Nop(), tt.opts)

			require.Equal(tt.wantChunkSize, s.opts.ChunkSize)
			require.Equal(tt.wantParallel, s.opts.Parallel)
			require.Equal(tt.wantPullTimeout, s.opts.PullTimeout)
		})
	}
}

func TestDetectMediaType(t *testing.T) {
	tests := []struct {
		name string
		data string
		want string
	}{
		{
			name: "oci manifest",
			data: `{"mediaType":"application/vnd.oci.image.manifest.v1+json"}`,
			want: "application/vnd.oci.image.manifest.v1+json",
		},
		{
			name: "docker manifest",
			data: `{"mediaType":"application/vnd.docker.distribution.manifest.v2+json"}`,
			want: "application/vnd.docker.distribution.manifest.v2+json",
		},
		{
			name: "no media type field",
			data: `{"schemaVersion":2}`,
			want: "application/vnd.docker.distribution.manifest.v2+json",
		},
		{
			name: "invalid json",
			data: `not json`,
			want: "application/vnd.docker.distribution.manifest.v2+json",
		},
		{
			name: "empty",
			data: ``,
			want: "application/vnd.docker.distribution.manifest.v2+json",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require := require.New(t)
			require.Equal(tt.want, detectMediaType([]byte(tt.data)))
		})
	}
}

func TestHandleVersion(t *testing.T) {
	require := require.New(t)

	dir := t.TempDir()
	l, err := store.Open(dir)
	require.NoError(err)

	client := oci.NewClient()
	s := New(l, client, logging.Nop(), DefaultOptions())

	req := httptest.NewRequest(http.MethodGet, "/v2/", nil)
	w := httptest.NewRecorder()

	s.ServeHTTP(w, req)

	require.Equal(http.StatusOK, w.Code)
	require.Equal("registry/2.0", w.Header().Get("Docker-Distribution-API-Version"))
	require.Equal("application/json", w.Header().Get("Content-Type"))
}

func TestServeHTTPRouting(t *testing.T) {
	tests := []struct {
		name       string
		path       string
		wantStatus int
	}{
		{"v2 endpoint", "/v2/", http.StatusOK},
		{"v2 no slash", "/v2", http.StatusOK},
		{"unknown path", "/unknown", http.StatusNotFound},
		{"root", "/", http.StatusNotFound},
		{"short v2 path", "/v2/foo", http.StatusNotFound},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require := require.New(t)

			dir := t.TempDir()
			l, err := store.Open(dir)
			require.NoError(err)

			client := oci.NewClient()
			s := New(l, client, logging.Nop(), DefaultOptions())

			req := httptest.NewRequest(http.MethodGet, tt.path, nil)
			w := httptest.NewRecorder()

			s.ServeHTTP(w, req)

			require.Equal(tt.wantStatus, w.Code)
		})
	}
}

func TestHandleBlobNotFound(t *testing.T) {
	require := require.New(t)

	dir := t.TempDir()
	l, err := store.Open(dir)
	require.NoError(err)

	client := oci.NewClient()
	s := New(l, client, logging.Nop(), DefaultOptions())

	req := httptest.NewRequest(http.MethodGet, "/v2/quay.io/test/repo/blobs/sha256:notexist", nil)
	w := httptest.NewRecorder()

	s.ServeHTTP(w, req)

	require.Equal(http.StatusNotFound, w.Code)
	require.True(strings.Contains(w.Body.String(), "not found"))
}

func TestHandleBlobExists(t *testing.T) {
	require := require.New(t)

	dir := t.TempDir()
	l, err := store.Open(dir)
	require.NoError(err)

	digest := "sha256:abc123"
	content := "blob content here"
	_, err = l.WriteBlob(digest, strings.NewReader(content))
	require.NoError(err)

	client := oci.NewClient()
	s := New(l, client, logging.Nop(), DefaultOptions())

	// GET request
	req := httptest.NewRequest(http.MethodGet, "/v2/quay.io/test/repo/blobs/sha256:abc123", nil)
	w := httptest.NewRecorder()

	s.ServeHTTP(w, req)

	require.Equal(http.StatusOK, w.Code)
	require.Equal(content, w.Body.String())
	require.Equal("sha256:abc123", w.Header().Get("Docker-Content-Digest"))
}

func TestHandleBlobHead(t *testing.T) {
	require := require.New(t)

	dir := t.TempDir()
	l, err := store.Open(dir)
	require.NoError(err)

	digest := "sha256:headtest"
	content := "head test content"
	_, err = l.WriteBlob(digest, strings.NewReader(content))
	require.NoError(err)

	client := oci.NewClient()
	s := New(l, client, logging.Nop(), DefaultOptions())

	req := httptest.NewRequest(http.MethodHead, "/v2/quay.io/test/repo/blobs/sha256:headtest", nil)
	w := httptest.NewRecorder()

	s.ServeHTTP(w, req)

	require.Equal(http.StatusOK, w.Code)
	require.Equal("sha256:headtest", w.Header().Get("Docker-Content-Digest"))
	require.Equal("17", w.Header().Get("Content-Length"))
	require.Empty(w.Body.String())
}
