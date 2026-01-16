# Fray Developer Guide

## Architecture

```
cmd/fray/           CLI entrypoint
pkg/
  layout/           OCI image layout storage
  merkle/           Merkle tree for chunk tracking
  oci/              OCI registry client
  registry/         Pull-through caching proxy server
  logging/          Zap + lumberjack logging
  version/          Build version info
```

### Key Components

**Merkle Tree** (`pkg/merkle/`)
- Tracks downloaded chunks for resumable transfers
- Serializes state to disk for crash recovery
- Enables efficient diff between partial downloads

**OCI Client** (`pkg/oci/`)
- Fetches manifests and blobs from registries
- Handles Docker Hub and GHCR token auth
- Supports HTTP Range requests for chunked downloads

**Layout** (`pkg/layout/`)
- OCI Image Layout spec storage
- Content-addressable blob store
- Partial blob support for resumable writes

**Registry Proxy** (`pkg/registry/`)
- OCI Distribution API (read-only)
- Pull-through caching
- Deduplicates concurrent pulls

## Building

```bash
make build          # build binary
make test-unit      # run unit tests
make test-integration # run integration tests
make lint           # run golangci-lint
make release        # build all platforms
make image          # build container image
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
