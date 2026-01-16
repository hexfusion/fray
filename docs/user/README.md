# Fray User Guide

Fray is an edge-native OCI image puller with resumable downloads and pull-through caching.

## Installation

### Binary

Download from [releases](https://github.com/hexfusion/fray/releases):

```bash
curl -LO https://github.com/hexfusion/fray/releases/latest/download/fray-linux-amd64
chmod +x fray-linux-amd64
sudo mv fray-linux-amd64 /usr/local/bin/fray
```

### Container

```bash
podman pull ghcr.io/hexfusion/fray:latest
```

## Commands

### pull

Pull an image to an OCI layout directory:

```bash
fray pull alpine:latest
fray pull -o /var/lib/images alpine:latest
fray pull -c 4194304 -p 8 registry.example.com/myimage:v1
```

Options:
- `-o` - output directory (default: `./oci-layout`)
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
- `-d` - data/cache directory (default: `./fray-cache`)
- `-c` - chunk size in bytes (default: 1MB)
- `-p` - parallel downloads (default: 4)
- `--log-file` - log file path (default: stdout)
- `--log-level` - log level: debug, info, warn, error (default: info)
- `--log-max-size` - max log file size in MB (default: 100)
- `--log-max-backups` - max rotated log files (default: 3)

### status

Show OCI layout status:

```bash
fray status
fray status /path/to/layout
```

### version

Show version information:

```bash
fray version
fray version -json
```

## Proxy Setup

### With Podman

Configure podman to use fray as a registry mirror:

```bash
# Start fray proxy
fray proxy -l :5000 -d /var/cache/fray &

# Pull through fray
podman pull --tls-verify=false localhost:5000/docker.io/library/alpine:latest
```

### With containers-registries.conf

Add to `/etc/containers/registries.conf.d/fray.conf`:

```toml
[[registry]]
location = "docker.io"

[[registry.mirror]]
location = "localhost:5000/docker.io"
insecure = true
```

### Systemd Service

Install the systemd unit:

```bash
sudo cp dist/systemd/fray-proxy.service /etc/systemd/system/
sudo systemctl daemon-reload
sudo systemctl enable --now fray-proxy
```

## Resumable Downloads

Fray automatically resumes interrupted downloads. State is stored in `.fray/` within the layout directory. If a download is interrupted, simply run the same pull command again.
