# Development Guide

This document covers building from source, running a local development environment, and testing.

## Prerequisites

- **Go 1.26.0**
- **make**

## Building from Source

```bash
git clone https://github.com/akihiro/wsl-secret-service.git
cd wsl-secret-service
```

```bash
# Build both Linux daemon and Windows helper
make build

# Build only the Linux daemon
make build-linux

# Build only the Windows helper (cross-compiled from Linux)
make build-windows
```

Output binaries are placed in `./bin/`:
- `bin/wsl-secret-service` — Linux daemon
- `bin/wincred-helper.exe` — Windows helper (cross-compiled)

### Build Details

- CGO is disabled (`CGO_ENABLED=0`) for fully static binaries
- Uses `GOEXPERIMENT=runtimesecret` for memory-protection features
- Both binaries are built with `-trimpath -buildmode pie`

### Install After Building

```bash
make install
```

This copies the daemon to `~/.local/bin/` and the helper to `~/.local/share/wsl-secret-service/`. Follow the printed instructions to enable the systemd user service.

## Running Without WSL2

A Linux-native mock helper is provided so you can develop and test on any Linux machine without WSL2 or Windows:

```bash
make run-dev
```

This builds the daemon and `mock-wincred-helper`, then runs the daemon with memory protection disabled and secrets stored in `bin/dev-store.jsonl` instead of Windows Credential Manager.

## Testing

### Unit and Integration Tests

```bash
make test
```

Runs `go test ./...` across all packages.

### End-to-End Tests

**Without WSL2** (uses mock helper — no Windows required):

```bash
make e2e-test-dev          # standard
make e2e-test-dev-verbose  # verbose
```

**With WSL2** (requires `secret-tool` from `libsecret-tools`):

```bash
# Install test dependencies
sudo apt-get install -y libsecret-tools dbus-tools jq

make e2e-test          # standard
make e2e-test-verbose  # verbose
make e2e-test-debug    # show each command as executed
make e2e-clean         # remove test metadata and stop any running daemon
```

See [e2e-testing.md](e2e-testing.md) for a full description of test cases and troubleshooting.

## Troubleshooting Build Issues

- Ensure Go 1.26.0 is installed: `go version`
- Run `go mod tidy` to resolve dependency issues
- CGO must remain disabled; do not set `CGO_ENABLED=1`
