// Package integration tests Fray against live OCI registries.
package integration

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/hexfusion/fray/pkg/oci"
)

// Test images representing different scenarios.
var testImages = []struct {
	Name        string
	Image       string
	Description string
	NeedsAuth   bool
	ExpectFail  bool
}{
	// === Quay.io ===
	{
		Name:        "quay-busybox",
		Image:       "quay.io/prometheus/busybox:latest",
		Description: "Quay.io, small image",
	},
	{
		Name:        "quay-prometheus",
		Image:       "quay.io/prometheus/prometheus:latest",
		Description: "Quay.io, medium multi-arch",
	},

	// === Docker Hub ===
	{
		Name:        "dockerhub-alpine",
		Image:       "docker.io/library/alpine:latest",
		Description: "Docker Hub, minimal",
	},
	{
		Name:        "dockerhub-nginx",
		Image:       "docker.io/library/nginx:alpine",
		Description: "Docker Hub, multi-arch",
	},

	// === GCR ===
	{
		Name:        "gcr-distroless",
		Image:       "gcr.io/distroless/static-debian12:nonroot",
		Description: "GCR, distroless",
	},

	// === Edge cases ===
	{
		Name:        "nonexistent",
		Image:       "quay.io/nonexistent/image:v999",
		Description: "Non-existent (expected fail)",
		ExpectFail:  true,
	},
}

func TestRegistryCompatibility(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	client := oci.NewClient()
	client.SetAuth(oci.NewRegistryAuth())

	for _, tc := range testImages {
		t.Run(tc.Name, func(t *testing.T) {
			t.Logf("Image: %s (%s)", tc.Image, tc.Description)

			registry, repo, ref := oci.ParseImageRef(tc.Image)

			manifest, err := client.GetManifest(ctx, registry, repo, ref)
			if tc.ExpectFail {
				if err == nil {
					t.Fatal("expected failure but got success")
				}
				t.Logf("expected failure: %v", err)
				return
			}

			if err != nil {
				t.Fatalf("GetManifest: %v", err)
			}

			var totalSize int64
			for _, l := range manifest.Layers {
				totalSize += l.Size
			}

			t.Logf("Manifest: %s, Layers: %d, Size: %.1f MB",
				shortMediaType(manifest.MediaType),
				len(manifest.Layers),
				float64(totalSize)/1024/1024)

			// Test Range request support on first layer
			if len(manifest.Layers) > 0 {
				layer := manifest.Layers[0]
				supported, err := client.SupportsRange(ctx, registry, repo, layer.Digest)
				if err != nil {
					t.Fatalf("SupportsRange: %v", err)
				}
				if !supported {
					t.Error("Range requests not supported")
				}
			}
		})
	}
}

func shortMediaType(mediaType string) string {
	switch mediaType {
	case "application/vnd.docker.distribution.manifest.v2+json":
		return "Docker v2"
	case "application/vnd.oci.image.manifest.v1+json":
		return "OCI"
	case "":
		return "unknown"
	default:
		parts := strings.Split(mediaType, ".")
		if len(parts) > 0 {
			return parts[len(parts)-1]
		}
		return mediaType
	}
}
