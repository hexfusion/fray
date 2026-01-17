# Fray User Guide

## Quick Start

```bash
# Install
go install github.com/hexfusion/fray/cmd/fray@latest

# Pull an image directly
fray pull docker.io/library/alpine:latest

# Or run as a caching proxy
fray proxy -l :5000 &
podman pull --tls-verify=false localhost:5000/docker.io/library/alpine:latest

# Check cache status
fray status

# Clean up incomplete downloads
fray prune
```

## Installation

### Binary

Download from [releases](https://github.com/hexfusion/fray/releases):

```bash
curl -LO https://github.com/hexfusion/fray/releases/latest/download/fray-linux-amd64
chmod +x fray-linux-amd64
sudo mv fray-linux-amd64 /usr/local/bin/fray
```

### From Source

```bash
go install github.com/hexfusion/fray/cmd/fray@latest
```

### Container

```bash
podman pull ghcr.io/hexfusion/fray:latest
```

## Commands

### pull

Pull an image to an OCI layout directory:

```bash
fray pull quay.io/prometheus/busybox:latest
fray pull -o /var/lib/images quay.io/fedora/fedora:latest
fray pull -c 4194304 -p 8 quay.io/myorg/myimage:v1
```

Options:
- `-o` - output directory
- `-c` - chunk size in bytes (default: 1MB)
- `-p` - parallel downloads (default: 4)

### proxy

Run a pull-through caching registry proxy:

```bash
fray proxy
fray proxy -l :5000 -d /var/cache/fray
fray proxy --log-file /var/log/fray.log --log-level debug
```

Options:
- `-l` - listen address (default: `:5000`)
- `-d` - cache directory
- `-c` - chunk size in bytes (default: 1MB)
- `-p` - parallel downloads (default: 4)
- `--log-file` - log file path
- `--log-level` - log level: debug, info, warn, error (default: info)
- `--log-max-size` - max log file size in MB (default: 100)
- `--log-max-backups` - max rotated log files (default: 3)

### status

Show OCI layout status:

```bash
fray status
fray status /path/to/layout
```

### prune

Remove incomplete downloads and temporary files:

```bash
fray prune
fray prune --dry-run
fray prune /path/to/cache
```

Options:
- `--dry-run` - show what would be deleted without deleting

### version

Show version information:

```bash
fray version
fray version -json
```

## Proxy Setup

### With Podman

```bash
fray proxy -l :5000 -d /var/cache/fray &
podman pull --tls-verify=false localhost:5000/quay.io/fedora/fedora:latest
```

### With registries.conf

Add to `/etc/containers/registries.conf.d/fray.conf`:

```toml
[[registry]]
location = "quay.io"

[[registry.mirror]]
location = "localhost:5000/quay.io"
insecure = true
```

### Systemd Service

```bash
sudo cp dist/systemd/fray-proxy.service /etc/systemd/system/
sudo systemctl daemon-reload
sudo systemctl enable --now fray-proxy
```

## Authentication

Fray reads credentials from standard locations:

1. `~/.docker/config.json`
2. `${XDG_RUNTIME_DIR}/containers/auth.json`
3. `REGISTRY_AUTH_FILE` environment variable

## Resumable Downloads

Fray automatically resumes interrupted downloads. State is stored in `.fray/` within the cache directory. If a download is interrupted, run the same command again to resume.

## Environment Variables

- `FRAY_CACHE_DIR` - default cache directory for all commands
