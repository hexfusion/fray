# Fray Developer Guide

## Architecture

```
cmd/fray/           CLI entrypoint
pkg/
  store/            OCI image layout storage and puller
  merkle/           Merkle tree for chunk tracking
  oci/              OCI registry client
  proxy/            Pull-through caching proxy server
  logging/          Logger interface with zap + lumberjack
internal/
  prune/            Cleanup incomplete downloads
  version/          Build version info
```

### Key Components

**Merkle Tree** (`pkg/merkle/`)
- Tracks downloaded chunks for resumable transfers
- Serializes state to disk for crash recovery
- Uses xxHash64 for fast chunk verification

**OCI Client** (`pkg/oci/`)
- Fetches manifests and blobs from registries
- Handles Docker Hub and GHCR token auth
- Supports HTTP Range requests for chunked downloads

**Store** (`pkg/store/`)
- OCI Image Layout spec storage
- Content-addressable blob store
- Puller with resumable chunked downloads
- Chunk verification on resume

**Proxy** (`pkg/proxy/`)
- OCI Distribution API (read-only)
- Pull-through caching
- Deduplicates concurrent pulls

## Building

```bash
make build              # build binary
make test               # run unit + e2e tests
make test-unit          # run unit tests
make test-e2e           # run e2e tests (no network)
make test-e2e-integration # run e2e tests (pulls from registries)
make lint               # run golangci-lint
make release            # build all platforms
make image              # build container image
```

## Testing

Tests use `require` from testify with the pattern:

```go
func TestFoo(t *testing.T) {
    require := require.New(t)

    // table-driven tests preferred
    tests := []struct {
        name string
        input int
        want  int
    }{
        {"case one", 1, 2},
        {"case two", 2, 4},
    }

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            require := require.New(t)
            require.Equal(tt.want, double(tt.input))
        })
    }
}
```

## Code Style

See [AGENTS.md](../../AGENTS.md) for project principles:

- Minimal dependencies
- Error wrapping, not string matching
- Require-only tests, table-driven
- Concise comments
- Performance-focused

## Release Process

1. Ensure tests pass: `make lint && make test-unit`
2. Tag release: `git tag v0.0.1`
3. Push tag: `git push origin v0.0.1`
4. CI builds binaries and container, creates GitHub release
