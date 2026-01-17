package proxy

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"go.uber.org/zap"

	"github.com/hexfusion/fray/pkg/logging"
	"github.com/hexfusion/fray/pkg/oci"
	"github.com/hexfusion/fray/pkg/store"
)

// Server is a pull-through caching OCI registry proxy.
type Server struct {
	layout  *store.Layout
	client  *oci.Client
	log     logging.Logger
	opts    Options
	pulling map[string]*pullState
	mu      sync.Mutex
}

type pullState struct {
	done chan struct{}
	err  error
}

const (
	DefaultChunkSize   = 1024 * 1024 // 1MB
	DefaultParallel    = 4
	DefaultPullTimeout = 30 * 60 // 30 minutes in seconds
)

// Options configures the proxy server.
type Options struct {
	ChunkSize   int
	Parallel    int
	PullTimeout int
}

// DefaultOptions returns sensible defaults.
func DefaultOptions() Options {
	return Options{
		ChunkSize:   DefaultChunkSize,
		Parallel:    DefaultParallel,
		PullTimeout: DefaultPullTimeout,
	}
}

// New creates a new proxy server.
func New(l *store.Layout, client *oci.Client, log logging.Logger, opts Options) *Server {
	if opts.ChunkSize == 0 {
		opts.ChunkSize = DefaultChunkSize
	}
	if opts.Parallel == 0 {
		opts.Parallel = DefaultParallel
	}
	if opts.PullTimeout == 0 {
		opts.PullTimeout = DefaultPullTimeout
	}
	return &Server{
		layout:  l,
		client:  client,
		log:     log,
		opts:    opts,
		pulling: make(map[string]*pullState),
	}
}

// ServeHTTP implements http.Handler.
func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	start := time.Now()
	path := r.URL.Path

	defer func() {
		s.log.Info("request",
			zap.String("method", r.Method),
			zap.String("path", path),
			zap.Duration("latency", time.Since(start)),
		)
	}()

	if path == "/v2/" || path == "/v2" {
		s.handleVersion(w, r)
		return
	}

	if strings.HasPrefix(path, "/v2/") {
		parts := strings.Split(strings.TrimPrefix(path, "/v2/"), "/")
		if len(parts) >= 4 {
			for i := 1; i < len(parts)-1; i++ {
				if parts[i] == "manifests" {
					registry := parts[0]
					repo := strings.Join(parts[1:i], "/")
					ref := strings.Join(parts[i+1:], "/")
					s.handleManifest(w, r, registry, repo, ref)
					return
				}
				if parts[i] == "blobs" {
					registry := parts[0]
					repo := strings.Join(parts[1:i], "/")
					digest := strings.Join(parts[i+1:], "/")
					s.handleBlob(w, r, registry, repo, digest)
					return
				}
			}
		}
	}

	http.NotFound(w, r)
}

func (s *Server) handleVersion(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Docker-Distribution-API-Version", "registry/2.0")
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("{}"))
}

func (s *Server) handleManifest(w http.ResponseWriter, r *http.Request, registry, repo, ref string) {
	image := fmt.Sprintf("%s/%s:%s", registry, repo, ref)

	if strings.HasPrefix(ref, "sha256:") {
		image = fmt.Sprintf("%s/%s@%s", registry, repo, ref)
	}

	digest, err := s.findManifestDigest(image)
	if err != nil {
		s.log.Info("cache miss, pulling from upstream", zap.String("image", image))
		if err := s.pullImage(r.Context(), image); err != nil {
			s.log.Error("upstream pull failed", zap.String("image", image), zap.Error(err))
			http.Error(w, fmt.Sprintf("upstream pull failed: %v", err), http.StatusBadGateway)
			return
		}
		digest, err = s.findManifestDigest(image)
		if err != nil {
			s.log.Error("manifest not found after pull", zap.String("image", image), zap.Error(err))
			http.Error(w, "manifest not found after pull", http.StatusInternalServerError)
			return
		}
		s.log.Info("pull complete", zap.String("image", image))
	} else {
		s.log.Debug("cache hit", zap.String("image", image))
	}

	data, err := s.layout.ReadBlob(digest)
	if err != nil {
		s.log.Error("read manifest blob failed", zap.String("digest", digest), zap.Error(err))
		http.Error(w, "failed to read manifest", http.StatusInternalServerError)
		return
	}

	mediaType := detectMediaType(data)

	if r.Method == http.MethodHead {
		w.Header().Set("Content-Type", mediaType)
		w.Header().Set("Docker-Content-Digest", digest)
		w.Header().Set("Content-Length", fmt.Sprintf("%d", len(data)))
		w.WriteHeader(http.StatusOK)
		return
	}

	w.Header().Set("Content-Type", mediaType)
	w.Header().Set("Docker-Content-Digest", digest)
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(data)
}

func (s *Server) handleBlob(w http.ResponseWriter, r *http.Request, _, _, digest string) {
	if !s.layout.HasBlob(digest) {
		http.Error(w, "blob not found", http.StatusNotFound)
		return
	}

	blobPath := filepath.Join(s.layout.Root(), "blobs", "sha256", strings.TrimPrefix(digest, "sha256:"))
	info, err := os.Stat(blobPath)
	if err != nil {
		http.Error(w, "blob stat failed", http.StatusInternalServerError)
		return
	}

	if r.Method == http.MethodHead {
		w.Header().Set("Content-Type", "application/octet-stream")
		w.Header().Set("Docker-Content-Digest", digest)
		w.Header().Set("Content-Length", fmt.Sprintf("%d", info.Size()))
		w.WriteHeader(http.StatusOK)
		return
	}

	f, err := os.Open(blobPath)
	if err != nil {
		http.Error(w, "blob open failed", http.StatusInternalServerError)
		return
	}
	defer f.Close()

	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Docker-Content-Digest", digest)
	w.Header().Set("Content-Length", fmt.Sprintf("%d", info.Size()))
	w.WriteHeader(http.StatusOK)

	io.Copy(w, f)
}

func (s *Server) findManifestDigest(image string) (string, error) {
	index, err := s.layout.GetIndex()
	if err != nil {
		return "", err
	}

	for _, m := range index.Manifests {
		refName := m.Annotations["org.opencontainers.image.ref.name"]
		if refName == image {
			return m.Digest, nil
		}
	}

	return "", fmt.Errorf("manifest not found: %s", image)
}

func (s *Server) pullImage(ctx context.Context, image string) error {
	s.mu.Lock()
	if state, ok := s.pulling[image]; ok {
		s.mu.Unlock()
		<-state.done
		return state.err
	}

	state := &pullState{done: make(chan struct{})}
	s.pulling[image] = state
	s.mu.Unlock()

	puller := store.NewPuller(s.layout, s.client, s.log, store.PullOptions{
		ChunkSize: s.opts.ChunkSize,
		Parallel:  s.opts.Parallel,
	})

	_, err := puller.Pull(ctx, image)
	state.err = err
	close(state.done)

	go func() {
		s.mu.Lock()
		delete(s.pulling, image)
		s.mu.Unlock()
	}()

	return err
}

func detectMediaType(data []byte) string {
	var m struct {
		MediaType string `json:"mediaType"`
	}
	if err := json.Unmarshal(data, &m); err == nil && m.MediaType != "" {
		return m.MediaType
	}
	return "application/vnd.docker.distribution.manifest.v2+json"
}
