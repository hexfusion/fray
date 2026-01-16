package store

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/hexfusion/fray/pkg/merkle"
	"github.com/hexfusion/fray/pkg/oci"
)

const (
	DefaultChunkSize = 1024 * 1024
	TreeFile         = "tree.json"
)

// Store manages layer downloads with merkle tree state.
type Store struct {
	root        string
	chunkSize   int
	parallelism int
	fetcher     *oci.Fetcher
}

// Option configures a Store.
type Option func(*Store)

// WithChunkSize sets the chunk size for downloads.
func WithChunkSize(size int) Option {
	return func(s *Store) {
		if size > 0 {
			s.chunkSize = size
		}
	}
}

// WithParallelism sets the number of parallel downloads.
func WithParallelism(n int) Option {
	return func(s *Store) {
		if n > 0 {
			s.parallelism = n
		}
	}
}

// New creates a new store.
func New(root string, opts ...Option) *Store {
	s := &Store{
		root:        root,
		chunkSize:   DefaultChunkSize,
		parallelism: 1,
		fetcher:     oci.NewFetcher(),
	}
	for _, opt := range opts {
		opt(s)
	}
	return s
}

// LayerState represents the download state of a layer.
type LayerState struct {
	Digest    string
	Size      int64
	Tree      *merkle.Tree
	StorePath string
}

// GetOrCreateLayer gets existing layer state or creates new.
func (s *Store) GetOrCreateLayer(digest string, size int64) (*LayerState, error) {
	storePath := s.layerPath(digest)

	treePath := filepath.Join(storePath, TreeFile)
	if tree, err := merkle.LoadFromFile(treePath); err == nil {
		return &LayerState{
			Digest:    digest,
			Size:      size,
			Tree:      tree,
			StorePath: storePath,
		}, nil
	}

	if err := os.MkdirAll(storePath, 0755); err != nil {
		return nil, err
	}

	tree := merkle.New(size, s.chunkSize)

	return &LayerState{
		Digest:    digest,
		Size:      size,
		Tree:      tree,
		StorePath: storePath,
	}, nil
}

// SaveState saves the layer state to disk.
func (s *Store) SaveState(layer *LayerState) error {
	treePath := filepath.Join(layer.StorePath, TreeFile)
	return layer.Tree.SaveToFile(treePath)
}

// FetchChunk fetches a single chunk and stores it.
func (s *Store) FetchChunk(ctx context.Context, layer *LayerState, url string, chunkIndex int) error {
	start := layer.Tree.ChunkOffset(chunkIndex)
	length := layer.Tree.ChunkLength(chunkIndex)
	end := start + int64(length)

	data, err := s.fetcher.FetchRange(ctx, url, start, end)
	if err != nil {
		return fmt.Errorf("fetch chunk %d: %w", chunkIndex, err)
	}

	if len(data) != length {
		return fmt.Errorf("chunk %d: expected %d bytes, got %d", chunkIndex, length, len(data))
	}

	chunkPath := filepath.Join(layer.StorePath, fmt.Sprintf("chunk-%05d", chunkIndex))
	if err := os.WriteFile(chunkPath, data, 0644); err != nil {
		return fmt.Errorf("write chunk %d: %w", chunkIndex, err)
	}

	if err := layer.Tree.SetChunk(chunkIndex, data); err != nil {
		return fmt.Errorf("update tree for chunk %d: %w", chunkIndex, err)
	}

	return nil
}

// FetchMissing fetches all missing chunks with parallel downloads.
func (s *Store) FetchMissing(ctx context.Context, layer *LayerState, url string, progress func(int, int)) error {
	missing := layer.Tree.MissingChunks()
	total := len(missing)

	if total == 0 {
		return nil
	}

	if s.parallelism == 1 {
		return s.fetchMissingSeq(ctx, layer, url, missing, progress)
	}

	type job struct {
		index      int
		chunkIndex int
	}

	type result struct {
		index      int
		chunkIndex int
		data       []byte
		err        error
	}

	jobs := make(chan job, total)
	results := make(chan result, total)

	var wg sync.WaitGroup
	for w := 0; w < s.parallelism; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := range jobs {
				select {
				case <-ctx.Done():
					results <- result{j.index, j.chunkIndex, nil, ctx.Err()}
					return
				default:
				}

				start := layer.Tree.ChunkOffset(j.chunkIndex)
				length := layer.Tree.ChunkLength(j.chunkIndex)
				end := start + int64(length)

				data, err := s.fetcher.FetchRange(ctx, url, start, end)
				results <- result{j.index, j.chunkIndex, data, err}
			}
		}()
	}

	go func() {
		for i, chunkIndex := range missing {
			jobs <- job{i, chunkIndex}
		}
		close(jobs)
	}()

	var firstErr error
	completed := 0

	for i := 0; i < total; i++ {
		r := <-results

		if r.err != nil && firstErr == nil {
			firstErr = r.err
			continue
		}

		if r.err == nil {
			chunkPath := filepath.Join(layer.StorePath, fmt.Sprintf("chunk-%05d", r.chunkIndex))
			if err := os.WriteFile(chunkPath, r.data, 0644); err != nil {
				if firstErr == nil {
					firstErr = fmt.Errorf("write chunk %d: %w", r.chunkIndex, err)
				}
				continue
			}

			if err := layer.Tree.SetChunk(r.chunkIndex, r.data); err != nil {
				if firstErr == nil {
					firstErr = fmt.Errorf("update tree for chunk %d: %w", r.chunkIndex, err)
				}
				continue
			}

			completed++
			if progress != nil {
				progress(completed, total)
			}

			if completed%10 == 0 {
				s.SaveState(layer)
			}
		}
	}

	wg.Wait()
	s.SaveState(layer)

	return firstErr
}

func (s *Store) fetchMissingSeq(ctx context.Context, layer *LayerState, url string, missing []int, progress func(int, int)) error {
	total := len(missing)

	for i, chunkIndex := range missing {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		if err := s.FetchChunk(ctx, layer, url, chunkIndex); err != nil {
			s.SaveState(layer)
			return err
		}

		if progress != nil {
			progress(i+1, total)
		}

		if (i+1)%10 == 0 {
			s.SaveState(layer)
		}
	}

	return s.SaveState(layer)
}

// AssembleBlob assembles all chunks into the final blob.
func (s *Store) AssembleBlob(layer *LayerState) (string, error) {
	if !layer.Tree.Complete() {
		return "", fmt.Errorf("layer not complete: %d/%d chunks",
			layer.Tree.PresentCount, layer.Tree.NumChunks)
	}

	blobPath := filepath.Join(layer.StorePath, "blob")
	f, err := os.Create(blobPath)
	if err != nil {
		return "", err
	}
	defer f.Close()

	hasher := sha256.New()

	for i := 0; i < layer.Tree.NumChunks; i++ {
		chunkPath := filepath.Join(layer.StorePath, fmt.Sprintf("chunk-%05d", i))
		data, err := os.ReadFile(chunkPath)
		if err != nil {
			return "", fmt.Errorf("read chunk %d: %w", i, err)
		}

		if _, err := f.Write(data); err != nil {
			return "", fmt.Errorf("write chunk %d: %w", i, err)
		}

		hasher.Write(data)
	}

	computedDigest := "sha256:" + hex.EncodeToString(hasher.Sum(nil))
	if computedDigest != layer.Digest {
		os.Remove(blobPath)
		return "", fmt.Errorf("digest mismatch: expected %s, got %s", layer.Digest, computedDigest)
	}

	return blobPath, nil
}

// CleanupChunks removes individual chunk files after assembly.
func (s *Store) CleanupChunks(layer *LayerState) error {
	for i := 0; i < layer.Tree.NumChunks; i++ {
		chunkPath := filepath.Join(layer.StorePath, fmt.Sprintf("chunk-%05d", i))
		os.Remove(chunkPath)
	}
	return nil
}

func (s *Store) layerPath(digest string) string {
	clean := strings.TrimPrefix(digest, "sha256:")
	return filepath.Join(s.root, "layers", clean)
}

// BlobPath returns the path to an assembled blob, or empty if not assembled.
func (s *Store) BlobPath(digest string) string {
	blobPath := filepath.Join(s.layerPath(digest), "blob")
	if _, err := os.Stat(blobPath); err == nil {
		return blobPath
	}
	return ""
}
