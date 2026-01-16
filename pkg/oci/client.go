package oci

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"runtime"
	"strings"
)

var (
	ErrUnauthorized = errors.New("unauthorized")
	ErrNotFound     = errors.New("not found")
	ErrNoManifest   = errors.New("no matching manifest")
)

const (
	DockerHubRegistry = "registry-1.docker.io"
	DockerHubAlias    = "docker.io"
)

// Client fetches OCI artifacts from registries.
type Client struct {
	httpClient *http.Client
	auth       AuthProvider
	insecure   map[string]bool
}

// AuthProvider provides authentication for registry requests.
type AuthProvider interface {
	GetAuth(ctx context.Context, registry, repo string) (string, error)
}

// NewClient creates a new OCI client.
func NewClient() *Client {
	return &Client{
		httpClient: http.DefaultClient,
		insecure:   make(map[string]bool),
	}
}

// SetAuth sets the authentication provider.
func (c *Client) SetAuth(auth AuthProvider) {
	c.auth = auth
}

// SetInsecure marks a registry as insecure (HTTP instead of HTTPS).
func (c *Client) SetInsecure(registry string, insecure bool) {
	c.insecure[registry] = insecure
}

func (c *Client) registryURL(registry string) string {
	scheme := "https"
	if c.insecure[registry] {
		scheme = "http"
	}
	return fmt.Sprintf("%s://%s", scheme, registry)
}

// Manifest is an OCI/Docker image manifest.
type Manifest struct {
	SchemaVersion int    `json:"schemaVersion"`
	MediaType     string `json:"mediaType"`
	Config        Blob   `json:"config"`
	Layers        []Blob `json:"layers"`
}

// Blob is a content-addressable blob reference.
type Blob struct {
	MediaType string `json:"mediaType"`
	Digest    string `json:"digest"`
	Size      int64  `json:"size"`
}

// ManifestList is a multi-arch manifest list or OCI index.
type ManifestList struct {
	SchemaVersion int        `json:"schemaVersion"`
	MediaType     string     `json:"mediaType"`
	Manifests     []Platform `json:"manifests"`
}

// Platform is a platform-specific manifest reference.
type Platform struct {
	MediaType string `json:"mediaType"`
	Digest    string `json:"digest"`
	Size      int64  `json:"size"`
	Platform  struct {
		Architecture string `json:"architecture"`
		OS           string `json:"os"`
		Variant      string `json:"variant,omitempty"`
	} `json:"platform"`
}

// GetManifest fetches the manifest for an image, resolving manifest lists.
func (c *Client) GetManifest(ctx context.Context, registry, repo, ref string) (*Manifest, error) {
	body, mediaType, err := c.fetchManifest(ctx, registry, repo, ref)
	if err != nil {
		return nil, err
	}

	if isManifestList(mediaType) {
		var list ManifestList
		if err := json.Unmarshal(body, &list); err != nil {
			return nil, fmt.Errorf("parse manifest list: %w", err)
		}

		digest, err := selectPlatform(list)
		if err != nil {
			return nil, err
		}

		body, _, err = c.fetchManifest(ctx, registry, repo, digest)
		if err != nil {
			return nil, fmt.Errorf("fetch platform manifest: %w", err)
		}
	}

	var manifest Manifest
	if err := json.Unmarshal(body, &manifest); err != nil {
		return nil, fmt.Errorf("parse manifest: %w", err)
	}

	return &manifest, nil
}

func (c *Client) fetchManifest(ctx context.Context, registry, repo, ref string) ([]byte, string, error) {
	url := fmt.Sprintf("%s/v2/%s/manifests/%s", c.registryURL(registry), repo, ref)
	return c.doManifestRequest(ctx, url, registry, repo, false)
}

func (c *Client) doManifestRequest(ctx context.Context, url, registry, repo string, withAuth bool) ([]byte, string, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, "", err
	}

	req.Header.Set("Accept", strings.Join([]string{
		"application/vnd.oci.image.manifest.v1+json",
		"application/vnd.oci.image.index.v1+json",
		"application/vnd.docker.distribution.manifest.v2+json",
		"application/vnd.docker.distribution.manifest.list.v2+json",
	}, ", "))

	if withAuth && c.auth != nil {
		authHeader, err := c.auth.GetAuth(ctx, registry, repo)
		if err != nil && !strings.Contains(err.Error(), "DENIED") {
			return nil, "", fmt.Errorf("get auth: %w", err)
		}
		if authHeader != "" {
			req.Header.Set("Authorization", authHeader)
		}
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, "", err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, "", err
	}

	if resp.StatusCode == http.StatusUnauthorized && !withAuth && c.auth != nil {
		return c.doManifestRequest(ctx, url, registry, repo, true)
	}

	if resp.StatusCode == http.StatusUnauthorized {
		return nil, "", fmt.Errorf("%w: %s", ErrUnauthorized, registry)
	}

	if resp.StatusCode == http.StatusNotFound {
		return nil, "", fmt.Errorf("%w: %s", ErrNotFound, url)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, "", fmt.Errorf("status %d: %s", resp.StatusCode, string(body))
	}

	return body, resp.Header.Get("Content-Type"), nil
}

// SupportsRange checks if a registry supports HTTP Range requests.
func (c *Client) SupportsRange(ctx context.Context, registry, repo, digest string) (bool, error) {
	url := fmt.Sprintf("%s/v2/%s/blobs/%s", c.registryURL(registry), repo, digest)
	return c.doRangeCheck(ctx, url, registry, repo, false)
}

func (c *Client) doRangeCheck(ctx context.Context, url, registry, repo string, withAuth bool) (bool, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return false, err
	}

	req.Header.Set("Range", "bytes=0-0")

	if withAuth && c.auth != nil {
		authHeader, err := c.auth.GetAuth(ctx, registry, repo)
		if err != nil && !strings.Contains(err.Error(), "DENIED") {
			return false, fmt.Errorf("get auth: %w", err)
		}
		if authHeader != "" {
			req.Header.Set("Authorization", authHeader)
		}
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return false, err
	}
	defer resp.Body.Close()
	io.Copy(io.Discard, resp.Body)

	if resp.StatusCode == http.StatusUnauthorized && !withAuth && c.auth != nil {
		return c.doRangeCheck(ctx, url, registry, repo, true)
	}

	return resp.StatusCode == http.StatusPartialContent, nil
}

// GetBlob downloads a complete blob.
func (c *Client) GetBlob(ctx context.Context, registry, repo, digest string) (io.ReadCloser, error) {
	url := fmt.Sprintf("%s/v2/%s/blobs/%s", c.registryURL(registry), repo, digest)
	return c.doBlobRequest(ctx, url, registry, repo, "", false)
}

// GetBlobRange downloads a byte range from a blob.
func (c *Client) GetBlobRange(ctx context.Context, registry, repo, digest string, start, end int64) (io.ReadCloser, error) {
	url := fmt.Sprintf("%s/v2/%s/blobs/%s", c.registryURL(registry), repo, digest)
	rangeHeader := fmt.Sprintf("bytes=%d-%d", start, end)
	return c.doBlobRequest(ctx, url, registry, repo, rangeHeader, false)
}

func (c *Client) doBlobRequest(ctx context.Context, url, registry, repo, rangeHeader string, withAuth bool) (io.ReadCloser, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, err
	}

	if rangeHeader != "" {
		req.Header.Set("Range", rangeHeader)
	}

	if withAuth && c.auth != nil {
		authHeader, err := c.auth.GetAuth(ctx, registry, repo)
		if err != nil && !strings.Contains(err.Error(), "DENIED") {
			return nil, fmt.Errorf("get auth: %w", err)
		}
		if authHeader != "" {
			req.Header.Set("Authorization", authHeader)
		}
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode == http.StatusUnauthorized && !withAuth && c.auth != nil {
		resp.Body.Close()
		return c.doBlobRequest(ctx, url, registry, repo, rangeHeader, true)
	}

	if resp.StatusCode == http.StatusUnauthorized {
		resp.Body.Close()
		return nil, fmt.Errorf("%w: %s", ErrUnauthorized, registry)
	}

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusPartialContent {
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		return nil, fmt.Errorf("status %d: %s", resp.StatusCode, string(body))
	}

	return resp.Body, nil
}

func isManifestList(mediaType string) bool {
	return strings.Contains(mediaType, "manifest.list") || strings.Contains(mediaType, "image.index")
}

func selectPlatform(list ManifestList) (string, error) {
	targetOS := runtime.GOOS
	targetArch := runtime.GOARCH

	for _, m := range list.Manifests {
		if m.Platform.OS == targetOS && m.Platform.Architecture == targetArch {
			return m.Digest, nil
		}
	}

	for _, m := range list.Manifests {
		if m.Platform.OS == "linux" && m.Platform.Architecture == "amd64" {
			return m.Digest, nil
		}
	}

	available := make([]string, 0, len(list.Manifests))
	for _, m := range list.Manifests {
		available = append(available, fmt.Sprintf("%s/%s", m.Platform.OS, m.Platform.Architecture))
	}

	return "", fmt.Errorf("%w for %s/%s, available: %v", ErrNoManifest, targetOS, targetArch, available)
}

// ParseImageRef parses an image reference into registry, repo, and tag/digest.
func ParseImageRef(image string) (registry, repo, ref string) {
	ref = "latest"

	if idx := strings.LastIndex(image, "@"); idx != -1 {
		ref = image[idx+1:]
		image = image[:idx]
	} else if idx := strings.LastIndex(image, ":"); idx != -1 {
		if !strings.Contains(image[idx:], "/") {
			ref = image[idx+1:]
			image = image[:idx]
		}
	}

	parts := strings.SplitN(image, "/", 2)
	if len(parts) == 1 {
		registry = DockerHubRegistry
		repo = "library/" + parts[0]
	} else if strings.Contains(parts[0], ".") || strings.Contains(parts[0], ":") {
		registry = parts[0]
		repo = parts[1]
		if registry == DockerHubAlias {
			registry = DockerHubRegistry
		}
	} else {
		registry = DockerHubRegistry
		repo = image
	}

	return
}
