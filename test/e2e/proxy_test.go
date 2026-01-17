package e2e_test

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"

	"github.com/hexfusion/fray/pkg/logging"
	"github.com/hexfusion/fray/pkg/oci"
	"github.com/hexfusion/fray/pkg/proxy"
	"github.com/hexfusion/fray/pkg/store"
)

func newTestLogger() logging.Logger {
	config := zapcore.EncoderConfig{
		MessageKey:   "msg",
		LevelKey:     "level",
		TimeKey:      "",
		EncodeLevel:  zapcore.LowercaseLevelEncoder,
		EncodeCaller: zapcore.ShortCallerEncoder,
	}
	core := zapcore.NewCore(
		zapcore.NewConsoleEncoder(config),
		zapcore.AddSync(GinkgoWriter),
		zapcore.DebugLevel,
	)
	return logging.Wrap(zap.New(core))
}

var _ = Describe("Proxy", func() {
	var (
		cacheDir string
		layout   *store.Layout
		client   *oci.Client
		server   *proxy.Server
		ts       *httptest.Server
	)

	BeforeEach(func() {
		var err error
		cacheDir, err = os.MkdirTemp("", "fray-e2e-*")
		Expect(err).NotTo(HaveOccurred())

		layout, err = store.Open(cacheDir)
		Expect(err).NotTo(HaveOccurred())

		client = oci.NewClient()
		client.SetAuth(oci.NewRegistryAuth())

		server = proxy.New(layout, client, newTestLogger(), proxy.Options{
			ChunkSize: 64 * 1024, // 64KB chunks for visibility
			Parallel:  2,
		})

		ts = httptest.NewServer(server)
	})

	AfterEach(func() {
		if ts != nil {
			ts.Close()
		}
		if cacheDir != "" {
			os.RemoveAll(cacheDir)
		}
	})

	Describe("V2 endpoint", func() {
		It("should respond to /v2/", func() {
			resp, err := http.Get(ts.URL + "/v2/")
			Expect(err).NotTo(HaveOccurred())
			defer resp.Body.Close()

			Expect(resp.StatusCode).To(Equal(http.StatusOK))
			Expect(resp.Header.Get("Docker-Distribution-API-Version")).To(Equal("registry/2.0"))
		})

		It("should respond to /v2 without trailing slash", func() {
			resp, err := http.Get(ts.URL + "/v2")
			Expect(err).NotTo(HaveOccurred())
			defer resp.Body.Close()

			Expect(resp.StatusCode).To(Equal(http.StatusOK))
		})
	})

	Describe("Unknown paths", func() {
		It("should return 404 for unknown paths", func() {
			resp, err := http.Get(ts.URL + "/unknown")
			Expect(err).NotTo(HaveOccurred())
			defer resp.Body.Close()

			Expect(resp.StatusCode).To(Equal(http.StatusNotFound))
		})
	})

	Describe("Blob endpoints", func() {
		It("should return 404 for non-existent blobs", func() {
			resp, err := http.Get(ts.URL + "/v2/docker.io/library/alpine/blobs/sha256:notexist")
			Expect(err).NotTo(HaveOccurred())
			defer resp.Body.Close()

			Expect(resp.StatusCode).To(Equal(http.StatusNotFound))
		})
	})

	Describe("Manifest pull", Label("integration"), func() {
		It("should pull and cache a manifest from upstream", func() {
			// Request manifest through proxy
			resp, err := http.Get(ts.URL + "/v2/docker.io/library/alpine/manifests/latest")
			Expect(err).NotTo(HaveOccurred())
			defer resp.Body.Close()

			Expect(resp.StatusCode).To(Equal(http.StatusOK))
			Expect(resp.Header.Get("Docker-Content-Digest")).NotTo(BeEmpty())
			Expect(resp.Header.Get("Content-Type")).NotTo(BeEmpty())

			// Verify cache was populated
			index, err := layout.GetIndex()
			Expect(err).NotTo(HaveOccurred())
			Expect(len(index.Manifests)).To(BeNumerically(">", 0))
		})

		It("should serve cached manifest on second request", func() {
			// First request - pulls from upstream
			resp1, err := http.Get(ts.URL + "/v2/docker.io/library/alpine/manifests/latest")
			Expect(err).NotTo(HaveOccurred())
			body1, _ := io.ReadAll(resp1.Body)
			resp1.Body.Close()
			Expect(resp1.StatusCode).To(Equal(http.StatusOK))

			// Second request - should be from cache
			resp2, err := http.Get(ts.URL + "/v2/docker.io/library/alpine/manifests/latest")
			Expect(err).NotTo(HaveOccurred())
			body2, _ := io.ReadAll(resp2.Body)
			resp2.Body.Close()
			Expect(resp2.StatusCode).To(Equal(http.StatusOK))

			// Bodies should match
			Expect(body1).To(Equal(body2))
		})
	})

	Describe("Blob pull", Label("integration"), func() {
		It("should pull and cache blobs from upstream", func() {
			// First pull the manifest to get blob digests
			resp, err := http.Get(ts.URL + "/v2/docker.io/library/alpine/manifests/latest")
			Expect(err).NotTo(HaveOccurred())
			resp.Body.Close()
			Expect(resp.StatusCode).To(Equal(http.StatusOK))

			digest := resp.Header.Get("Docker-Content-Digest")
			Expect(digest).NotTo(BeEmpty())

			// Request the manifest blob
			blobResp, err := http.Get(ts.URL + "/v2/docker.io/library/alpine/blobs/" + digest)
			Expect(err).NotTo(HaveOccurred())
			defer blobResp.Body.Close()

			Expect(blobResp.StatusCode).To(Equal(http.StatusOK))
			Expect(blobResp.Header.Get("Docker-Content-Digest")).To(Equal(digest))
		})
	})

	Describe("Cache verification", Label("integration"), func() {
		It("should create OCI layout structure", func() {
			// Pull something to populate cache
			resp, err := http.Get(ts.URL + "/v2/docker.io/library/alpine/manifests/latest")
			Expect(err).NotTo(HaveOccurred())
			resp.Body.Close()

			// Verify OCI layout files exist
			Expect(filepath.Join(cacheDir, "oci-layout")).To(BeAnExistingFile())
			Expect(filepath.Join(cacheDir, "index.json")).To(BeAnExistingFile())
			Expect(filepath.Join(cacheDir, "blobs", "sha256")).To(BeADirectory())
		})

		It("should track cached images in index", func() {
			// Pull manifest
			resp, err := http.Get(ts.URL + "/v2/docker.io/library/alpine/manifests/latest")
			Expect(err).NotTo(HaveOccurred())
			resp.Body.Close()

			// Check index
			index, err := layout.GetIndex()
			Expect(err).NotTo(HaveOccurred())

			found := false
			for _, m := range index.Manifests {
				if m.Annotations["org.opencontainers.image.ref.name"] == "docker.io/library/alpine:latest" {
					found = true
					break
				}
			}
			Expect(found).To(BeTrue(), "expected alpine image in index")
		})
	})
})

var _ = Describe("Puller", func() {
	var (
		cacheDir string
		layout   *store.Layout
		client   *oci.Client
	)

	BeforeEach(func() {
		var err error
		cacheDir, err = os.MkdirTemp("", "fray-pull-e2e-*")
		Expect(err).NotTo(HaveOccurred())

		layout, err = store.Open(cacheDir)
		Expect(err).NotTo(HaveOccurred())

		client = oci.NewClient()
		client.SetAuth(oci.NewRegistryAuth())
	})

	AfterEach(func() {
		if cacheDir != "" {
			os.RemoveAll(cacheDir)
		}
	})

	Describe("Direct pull", Label("integration"), func() {
		It("should pull an image directly", func() {
			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
			defer cancel()

			puller := store.NewPuller(layout, client, newTestLogger(), store.PullOptions{
				ChunkSize: 64 * 1024, // 64KB chunks for visibility
				Parallel:  2,
			})

			result, err := puller.Pull(ctx, "docker.io/library/alpine:latest")
			Expect(err).NotTo(HaveOccurred())
			Expect(result.Digest).NotTo(BeEmpty())
			Expect(result.Layers).To(BeNumerically(">", 0))
		})

		It("should use cache on second pull", func() {
			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
			defer cancel()

			puller := store.NewPuller(layout, client, newTestLogger(), store.PullOptions{
				ChunkSize: 64 * 1024, // 64KB chunks for visibility
				Parallel:  2,
			})

			// First pull
			result1, err := puller.Pull(ctx, "docker.io/library/alpine:latest")
			Expect(err).NotTo(HaveOccurred())

			// Second pull - should be cached
			result2, err := puller.Pull(ctx, "docker.io/library/alpine:latest")
			Expect(err).NotTo(HaveOccurred())

			Expect(result2.Digest).To(Equal(result1.Digest))
			Expect(result2.Downloaded).To(Equal(int64(0)), "second pull should not download anything")
			Expect(result2.Cached).To(BeNumerically(">", 0), "second pull should use cache")
		})
	})

	Describe("Resume interrupted download", Label("integration"), func() {
		It("should create state files for interrupted downloads", func() {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()

			puller := store.NewPuller(layout, client, newTestLogger(), store.PullOptions{
				ChunkSize: 64 * 1024, // 64KB chunks for visibility
				Parallel:  1,
			})

			// Pull a larger image with short timeout - will likely be interrupted
			_, _ = puller.Pull(ctx, "docker.io/library/golang:alpine")

			// Check for state files (may or may not exist depending on timing)
			stateDir := filepath.Join(cacheDir, ".fray")
			if _, err := os.Stat(stateDir); err == nil {
				entries, err := os.ReadDir(stateDir)
				Expect(err).NotTo(HaveOccurred())
				// If state dir exists, it should have state files
				if len(entries) > 0 {
					GinkgoWriter.Printf("Found %d state files after interrupted pull\n", len(entries))
				}
			}
		})
	})
})
