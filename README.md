# fray

[![License](https://img.shields.io/badge/License-Apache_2.0-blue.svg)](LICENSE)
[![Go Report Card](https://goreportcard.com/badge/github.com/hexfusion/fray)](https://goreportcard.com/report/github.com/hexfusion/fray)

Resumable OCI image pulls for unreliable networks.

## The Problem

OCI image layers can be massive - [bootc](https://github.com/containers/bootc) OS images often have layers of 800MB or more. On unreliable edge networks, a failed pull means starting over.

## How Fray Solves It

Fray splits each layer into chunks and tracks progress with a merkle tree. Interrupted pulls resume from where they left off.

```
Traditional:  Layer (800MB) ------------------------> FAIL -> restart from 0

Fray:         Layer (800MB)
              +-----+-----+-----+-----+-----+-----+
              |  #  |  #  |  #  |  x  |     |     |  <- interrupt
              +-----+-----+-----+-----+-----+-----+
                                 |
                                 v resume
              +-----+-----+-----+-----+-----+-----+
              |  #  |  #  |  #  |  #  |  #  |  #  |  <- done
              +-----+-----+-----+-----+-----+-----+
```

## Quick Start

```bash
# Install
go install github.com/hexfusion/fray/cmd/fray@latest

# Pull an image
fray pull quay.io/prometheus/busybox:latest

# Run proxy
fray proxy -l :5000 -d /var/cache/fray &
podman pull --tls-verify=false localhost:5000/docker.io/library/alpine:latest
```

## Features

- **Resumable pulls** - chunk state persists across restarts
- **No server changes** - uses standard HTTP Range requests
- **Pull-through proxy** - caching registry for edge clusters
- **Parallel downloads** - configurable concurrency
- **OCI Image Layout** - standard storage format

## Documentation

- [User Guide](docs/user/README.md) - commands, options, proxy setup
- [Developer Guide](docs/dev/README.md) - architecture, building, testing

## Deployment

### bootc

[bootc](https://github.com/containers/bootc) enables transactional, image-based OS updates. Fray can be deployed as a [logically bound image](https://bootc-dev.github.io/bootc/logically-bound-images.html):

```dockerfile
FROM quay.io/centos-bootc/centos-bootc:stream9
COPY --from=ghcr.io/hexfusion/fray:latest / /
```

## License

Apache 2.0
