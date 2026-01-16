package oci

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestParseImageRef(t *testing.T) {
	tests := []struct {
		name         string
		image        string
		wantRegistry string
		wantRepo     string
		wantRef      string
	}{
		{
			name:         "simple image",
			image:        "nginx",
			wantRegistry: DockerHubRegistry,
			wantRepo:     "library/nginx",
			wantRef:      "latest",
		},
		{
			name:         "image with tag",
			image:        "nginx:1.21",
			wantRegistry: DockerHubRegistry,
			wantRepo:     "library/nginx",
			wantRef:      "1.21",
		},
		{
			name:         "namespaced image",
			image:        "myuser/myapp",
			wantRegistry: DockerHubRegistry,
			wantRepo:     "myuser/myapp",
			wantRef:      "latest",
		},
		{
			name:         "namespaced image with tag",
			image:        "myuser/myapp:v1.0",
			wantRegistry: DockerHubRegistry,
			wantRepo:     "myuser/myapp",
			wantRef:      "v1.0",
		},
		{
			name:         "docker.io registry",
			image:        "docker.io/library/nginx",
			wantRegistry: DockerHubRegistry,
			wantRepo:     "library/nginx",
			wantRef:      "latest",
		},
		{
			name:         "quay.io registry",
			image:        "quay.io/prometheus/prometheus:v2.40.0",
			wantRegistry: "quay.io",
			wantRepo:     "prometheus/prometheus",
			wantRef:      "v2.40.0",
		},
		{
			name:         "gcr.io registry",
			image:        "gcr.io/google-containers/pause:3.2",
			wantRegistry: "gcr.io",
			wantRepo:     "google-containers/pause",
			wantRef:      "3.2",
		},
		{
			name:         "digest reference",
			image:        "nginx@sha256:abc123",
			wantRegistry: DockerHubRegistry,
			wantRepo:     "library/nginx",
			wantRef:      "sha256:abc123",
		},
		{
			name:         "full digest reference",
			image:        "quay.io/prometheus/prometheus@sha256:abc123def456",
			wantRegistry: "quay.io",
			wantRepo:     "prometheus/prometheus",
			wantRef:      "sha256:abc123def456",
		},
		{
			name:         "registry with port",
			image:        "localhost:5000/myapp:latest",
			wantRegistry: "localhost:5000",
			wantRepo:     "myapp",
			wantRef:      "latest",
		},
		{
			name:         "nested repo path",
			image:        "ghcr.io/hexfusion/fray/proxy:v1.0",
			wantRegistry: "ghcr.io",
			wantRepo:     "hexfusion/fray/proxy",
			wantRef:      "v1.0",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require := require.New(t)

			registry, repo, ref := ParseImageRef(tt.image)

			require.Equal(tt.wantRegistry, registry)
			require.Equal(tt.wantRepo, repo)
			require.Equal(tt.wantRef, ref)
		})
	}
}

func TestIsManifestList(t *testing.T) {
	tests := []struct {
		name      string
		mediaType string
		want      bool
	}{
		{"docker manifest list", "application/vnd.docker.distribution.manifest.list.v2+json", true},
		{"oci image index", "application/vnd.oci.image.index.v1+json", true},
		{"docker manifest", "application/vnd.docker.distribution.manifest.v2+json", false},
		{"oci manifest", "application/vnd.oci.image.manifest.v1+json", false},
		{"empty", "", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require := require.New(t)
			require.Equal(tt.want, isManifestList(tt.mediaType))
		})
	}
}

func TestSelectPlatform(t *testing.T) {
	tests := []struct {
		name       string
		list       ManifestList
		wantDigest string
		wantErr    bool
	}{
		{
			name: "finds linux/amd64",
			list: ManifestList{
				Manifests: []Platform{
					{Digest: "sha256:arm", Platform: struct {
						Architecture string `json:"architecture"`
						OS           string `json:"os"`
						Variant      string `json:"variant,omitempty"`
					}{"arm64", "linux", ""}},
					{Digest: "sha256:amd64", Platform: struct {
						Architecture string `json:"architecture"`
						OS           string `json:"os"`
						Variant      string `json:"variant,omitempty"`
					}{"amd64", "linux", ""}},
				},
			},
			wantDigest: "sha256:amd64",
			wantErr:    false,
		},
		{
			name: "empty list",
			list: ManifestList{
				Manifests: []Platform{},
			},
			wantDigest: "",
			wantErr:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require := require.New(t)

			digest, err := selectPlatform(tt.list)

			if tt.wantErr {
				require.Error(err)
			} else {
				require.NoError(err)
				require.Equal(tt.wantDigest, digest)
			}
		})
	}
}
