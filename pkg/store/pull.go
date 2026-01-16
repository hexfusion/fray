package store

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"go.uber.org/zap"

	"github.com/hexfusion/fray/pkg/merkle"
	"github.com/hexfusion/fray/pkg/oci"
)

// PullOptions configures a pull operation.
type PullOptions struct {
	ChunkSize  int
	Parallel   int
	StateDir   string
	Logger     *zap.Logger
	OnProgress func(layer int, progress float64)
}

// Puller downloads images to an OCI layout with resumable chunked transfers.
type Puller struct {
	layout *Layout
	client *oci.Client
	opts   PullOptions
}

// NewPuller creates a puller with the given options.
func NewPuller(layout *Layout, client *oci.Client, opts PullOptions) *Puller {
	if opts.ChunkSize == 0 {
		opts.ChunkSize = 1024 * 1024
	}
	if opts.Parallel == 0 {
		opts.Parallel = 4
	}
	if opts.StateDir == "" {
		opts.StateDir = filepath.Join(layout.Root(), ".fray")
	}
	return &Puller{
		layout: layout,
		client: client,
		opts:   opts,
	}
}

func (p *Puller) log() *zap.Logger {
	if p.opts.Logger != nil {
		return p.opts.Logger
	}
	return zap.NewNop()
}

// PullResult contains pull operation results.
type PullResult struct {
	Digest     string
	Layers     int
	TotalSize  int64
	Downloaded int64
	Cached     int64
}

// Pull downloads an image to the layout.
func (p *Puller) Pull(ctx context.Context, image string) (*PullResult, error) {
	result := &PullResult{}

	registry, repo, ref := oci.ParseImageRef(image)

	manifest, err := p.client.GetManifest(ctx, registry, repo, ref)
	if err != nil {
		return nil, fmt.Errorf("get manifest: %w", err)
	}

	manifestData, err := json.Marshal(manifest)
	if err != nil {
		return nil, fmt.Errorf("marshal manifest: %w", err)
	}

	manifestDigest := fmt.Sprintf("sha256:%x", sha256Sum(manifestData))
	result.Digest = manifestDigest

	if _, err := p.layout.WriteBlob(manifestDigest, strings.NewReader(string(manifestData))); err != nil {
		return nil, fmt.Errorf("write manifest: %w", err)
	}

	configDigest := manifest.Config.Digest
	if !p.layout.HasBlob(configDigest) {
		if err := p.downloadBlob(ctx, registry, repo, configDigest); err != nil {
			return nil, fmt.Errorf("download config: %w", err)
		}
		result.Downloaded += manifest.Config.Size
	} else {
		result.Cached += manifest.Config.Size
	}

	result.Layers = len(manifest.Layers)
	for i, layer := range manifest.Layers {
		result.TotalSize += layer.Size

		if p.layout.HasBlob(layer.Digest) {
			result.Cached += layer.Size
			if p.opts.OnProgress != nil {
				p.opts.OnProgress(i, 1.0)
			}
			continue
		}

		downloaded, err := p.downloadLayerResumable(ctx, registry, repo, layer, i)
		if err != nil {
			return nil, fmt.Errorf("layer %d: %w", i, err)
		}
		result.Downloaded += downloaded
	}

	desc := Descriptor{
		MediaType: manifest.MediaType,
		Digest:    manifestDigest,
		Size:      int64(len(manifestData)),
		Annotations: map[string]string{
			"org.opencontainers.image.ref.name": image,
		},
	}
	if err := p.layout.AddManifest(desc); err != nil {
		return nil, fmt.Errorf("add to index: %w", err)
	}

	return result, nil
}

func (p *Puller) downloadBlob(ctx context.Context, registry, repo, digest string) error {
	r, err := p.client.GetBlob(ctx, registry, repo, digest)
	if err != nil {
		return err
	}
	defer r.Close()

	_, err = p.layout.WriteBlob(digest, r)
	return err
}

func (p *Puller) downloadLayerResumable(ctx context.Context, registry, repo string, layer oci.Blob, layerIdx int) (int64, error) {
	tree, statePath, err := p.loadOrCreateTree(layer.Digest, layer.Size)
	if err != nil {
		return 0, err
	}

	if tree.Complete() {
		if err := p.layout.FinalizeBlob(layer.Digest); err != nil {
			return 0, err
		}
		os.Remove(statePath)
		return 0, nil
	}

	downloaded := int64(0)
	for _, r := range tree.MissingRanges() {
		for chunkIdx := r[0]; chunkIdx < r[1]; chunkIdx++ {
			offset := tree.ChunkOffset(chunkIdx)
			length := int64(tree.ChunkLength(chunkIdx))

			data, err := p.downloadChunk(ctx, registry, repo, layer.Digest, offset, length)
			if err != nil {
				saveErr := p.saveTree(tree, statePath)
				return downloaded, errors.Join(fmt.Errorf("chunk %d: %w", chunkIdx, err), saveErr)
			}

			if err := p.layout.WriteBlobAt(layer.Digest, offset, data); err != nil {
				saveErr := p.saveTree(tree, statePath)
				return downloaded, errors.Join(fmt.Errorf("write chunk %d: %w", chunkIdx, err), saveErr)
			}

			if err := tree.SetChunk(chunkIdx, data); err != nil {
				saveErr := p.saveTree(tree, statePath)
				return downloaded, errors.Join(fmt.Errorf("set chunk %d: %w", chunkIdx, err), saveErr)
			}
			downloaded += int64(len(data))

			if p.opts.OnProgress != nil {
				p.opts.OnProgress(layerIdx, tree.Progress())
			}

			if chunkIdx%10 == 0 {
				if err := p.saveTree(tree, statePath); err != nil {
					return downloaded, fmt.Errorf("save state: %w", err)
				}
			}
		}
	}

	if !tree.Complete() {
		return downloaded, fmt.Errorf("incomplete")
	}

	if err := p.layout.FinalizeBlob(layer.Digest); err != nil {
		return downloaded, err
	}

	if err := os.Remove(statePath); err != nil && !os.IsNotExist(err) {
		p.log().Debug("cleanup state file", zap.String("path", statePath), zap.Error(err))
	}
	return downloaded, nil
}

func (p *Puller) downloadChunk(ctx context.Context, registry, repo, digest string, offset, length int64) ([]byte, error) {
	r, err := p.client.GetBlobRange(ctx, registry, repo, digest, offset, offset+length-1)
	if err != nil {
		return nil, err
	}
	defer r.Close()

	return io.ReadAll(r)
}

func (p *Puller) loadOrCreateTree(digest string, size int64) (*merkle.Tree, string, error) {
	if err := os.MkdirAll(p.opts.StateDir, 0755); err != nil {
		return nil, "", err
	}

	digestHash := strings.TrimPrefix(digest, "sha256:")
	if len(digestHash) > 12 {
		digestHash = digestHash[:12]
	}
	statePath := filepath.Join(p.opts.StateDir, digestHash+".state")

	if _, err := os.Stat(statePath); err == nil {
		tree, err := merkle.LoadFromFile(statePath)
		if err == nil {
			return tree, statePath, nil
		}
	}

	tree := merkle.New(size, p.opts.ChunkSize)
	return tree, statePath, nil
}

func (p *Puller) saveTree(tree *merkle.Tree, path string) error {
	return tree.SaveToFile(path)
}

func sha256Sum(data []byte) []byte {
	h := sha256.New()
	h.Write(data)
	return h.Sum(nil)
}
