# fray

[![License](https://img.shields.io/badge/License-Apache_2.0-blue.svg)](LICENSE)
[![Go Report Card](https://goreportcard.com/badge/github.com/hexfusion/fray)](https://goreportcard.com/report/github.com/hexfusion/fray)

Resumable OCI image pulls for unreliable networks.

**Status:** Experimental - APIs may change.

## The Problem

OCI image layers can be hundreds of megabytes. On unreliable networks, a failed pull means starting over.

## How Fray Solves It

Fray splits each layer into chunks and tracks progress with a merkle tree. Interrupted pulls resume from where they left off.

```
Traditional:  Layer (500MB) ----------------------> FAIL -> restart from 0

Fray:         Layer (500MB)
              +-----+-----+-----+-----+-----+-----+
              |  #  |  #  |  #  |  x  |     |     |  <- interrupt
              +-----+-----+-----+-----+-----+-----+
                                 |
                                 v resume
              +-----+-----+-----+-----+-----+-----+
              |  #  |  #  |  #  |  #  |  #  |  #  |  <- done
              +-----+-----+-----+-----+-----+-----+
```

## Features

- **Resumable pulls** - chunk state persists across restarts
- **No server changes** - uses standard HTTP Range requests
- **Pull-through proxy** - caching registry for edge clusters
- **Parallel downloads** - configurable concurrency
- **OCI Image Layout** - standard storage format with content-addressable deduplication

## Installation

```bash
go install github.com/hexfusion/fray/cmd/fray@latest
```

## Commands

### pull

Pull an image to an OCI layout directory:

```bash
fray pull quay.io/prometheus/busybox:latest
fray pull -o ./images -p 8 quay.io/myorg/myapp:v1.2
```

Options:
- `-o` - output directory (default: `./oci-layout`)
- `-p` - parallel downloads (default: 4)
- `-c` - chunk size in bytes (default: 1MB)

### proxy

Run a pull-through caching proxy:

```bash
fray proxy -l :5000 -d /var/cache/fray
```

Configure your container runtime to use `localhost:5000` as a registry mirror. First pull downloads and caches; subsequent pulls are served from cache with resumable chunk support.

Options:
- `-l` - listen address (default: `:5000`)
- `-d` - cache directory (default: `./fray-cache`)
- `-p` - parallel downloads (default: 4)
- `--log-file` - log to file instead of stderr

### status

Show layout contents and in-progress pulls:

```bash
fray status ./oci-layout
```

## Authentication

Fray reads credentials from standard locations:

1. `~/.docker/config.json`
2. `${XDG_RUNTIME_DIR}/containers/auth.json`
3. `REGISTRY_AUTH_FILE` environment variable

## Deployment

### Systemd

Run as a systemd unit:

```bash
systemctl enable --now fray-proxy
```

See `dist/systemd/fray-proxy.service` for the unit file.

### Logically Bound Images (bootc)

Fray can be deployed as a [logically bound image](https://bootc-dev.github.io/bootc/logically-bound-images.html) for bootc-based systems:

```dockerfile
FROM quay.io/centos-bootc/centos-bootc:stream9
COPY --from=ghcr.io/hexfusion/fray:latest / /
```

This embeds fray into the OS image - no container runtime needed at the edge.

## License

Apache 2.0
