# wsl-secret-service

A Freedesktop.org Secret Service daemon for WSL2 that bridges Linux applications and Windows credential storage.

`wsl-secret-service` implements the standard `org.freedesktop.secrets` D-Bus interface, allowing Linux apps to store and retrieve secrets seamlessly while persisting the actual data in the Windows Credential Manager. The daemon runs as a systemd user service and communicates with a companion Windows executable (`wincred-helper.exe`) via WSL2 interop.

## Features

- **Cross-platform Secret Storage**: Store secrets from Linux apps in Windows Credential Manager
- **Standard D-Bus API**: Compatible with any application that uses the Freedesktop.org Secret Service specification
- **Automatic Collection Management**: Creates a default "login" collection on first run
- **Memory Protection**: Hardens the process against memory inspection and swap exposure
- **Session Encryption**: Encrypts secrets in transit using industry-standard algorithms
- **Systemd Integration**: Runs as a user service with automatic startup

## Prerequisites

- **WSL2 Environment**: Must be running on Windows Subsystem for Linux 2
- **Go 1.26.0**: Required for building from source
- **D-Bus Session Bus**: Available in WSL2 (typically via systemd user instance)
- **Systemd User Services**: For automatic service management

## Installation

### Install from Release (Recommended)

Download pre-built binaries from the [GitHub Releases page](https://github.com/akihiro/wsl-secret-service/releases).

The following files are available for your architecture (`amd64` or `arm64`):
- `wsl-secret-service-linux-<arch>` — Linux daemon
- `wincred-helper-windows-<arch>.exe` — Windows helper
- `wsl-secret-service-linux-<arch>.intoto.jsonl` — SLSA provenance for the daemon
- `wincred-helper-windows-<arch>.intoto.jsonl` — SLSA provenance for the helper

#### Verify (Recommended)

Binaries are built with [SLSA Level 3](https://slsa.dev/) and signed via keyless signing (Sigstore/Fulcio). You can verify them before installing.

Using [slsa-verifier](https://github.com/slsa-framework/slsa-verifier):
```bash
VERSION=v<version>
ARCH=amd64  # or arm64

slsa-verifier verify-artifact \
  wsl-secret-service-linux-${ARCH} \
  --provenance-path wsl-secret-service-linux-${ARCH}.intoto.jsonl \
  --source-uri github.com/akihiro/wsl-secret-service \
  --source-tag ${VERSION}
```

Using [cosign](https://github.com/sigstore/cosign):
```bash
ARCH=amd64  # or arm64

cosign verify-blob \
  --bundle wsl-secret-service-linux-${ARCH}.intoto.jsonl \
  --certificate-oidc-issuer https://token.actions.githubusercontent.com \
  --certificate-identity-regexp "https://github.com/slsa-framework/slsa-github-generator/.github/workflows/builder_go_slsa3.yml@refs/tags/" \
  wsl-secret-service-linux-${ARCH}
```

#### Install Binaries

```bash
ARCH=amd64  # or arm64

install -Dm755 wsl-secret-service-linux-${ARCH} ~/.local/bin/wsl-secret-service

mkdir -p ~/.local/share/wsl-secret-service
cp wincred-helper-windows-${ARCH}.exe ~/.local/share/wsl-secret-service/wincred-helper.exe
```

Then download the service files and proceed to [Enable Systemd Service](#enable-systemd-service) below.

```bash
VERSION=v<version>
curl -LO "https://github.com/akihiro/wsl-secret-service/raw/${VERSION}/wsl-secret-service.service"
curl -LO "https://github.com/akihiro/wsl-secret-service/raw/${VERSION}/org.freedesktop.secrets.service"
```

### Build from Source

1. Clone the repository:
   ```bash
   git clone https://github.com/akihiro/wsl-secret-service.git
   cd wsl-secret-service
   ```

2. Build the binaries:
   ```bash
   make build
   ```
   This creates `bin/wsl-secret-service` (Linux daemon) and `bin/wincred-helper.exe` (Windows helper).

3. Install the binaries:
   ```bash
   make install
   ```
   This copies the daemon to `~/.local/bin/` and the helper to `~/.local/share/wsl-secret-service/`.

Then proceed to [Enable Systemd Service](#enable-systemd-service) below.

### Enable Systemd Service

Once the binaries are in place, register and start the service:

```bash
mkdir -p ~/.config/systemd/user ~/.local/share/dbus-1/services
cp wsl-secret-service.service ~/.config/systemd/user/
cp org.freedesktop.secrets.service ~/.local/share/dbus-1/services/
systemctl --user daemon-reload
systemctl --user enable --now wsl-secret-service
```

Verify the service is running:

```bash
systemctl --user status wsl-secret-service
```

### Verify Installation

Confirm the service is reachable over D-Bus:

```bash
# Install libsecret-tools if not already present
sudo apt-get install -y libsecret-tools

# Store a test secret
secret-tool store --label="test" mykey myvalue

# Retrieve it
secret-tool lookup mykey myvalue
```

## Usage

Once installed and running, applications automatically discover the secret service through the standard D-Bus interface (`org.freedesktop.secrets`). No manual configuration is typically required.

### Checking Service Status

```bash
# Check if the service is running
systemctl --user status wsl-secret-service

# View service logs
journalctl --user -u wsl-secret-service
```

### Example Use Cases

- Password managers storing credentials
- Applications caching API tokens
- Browsers managing saved passwords
- Development tools storing access keys

## Configuration

The daemon accepts the following command-line flags:

| Flag | Default | Description |
|------|---------|-------------|
| `--config-dir <path>` | `$XDG_CONFIG_HOME/wsl-secret-service` | Directory for metadata storage |
| `--helper-path <path>` | auto-discovered | Path to `wincred-helper.exe` |
| `--replace` | false | Replace existing D-Bus name owner |
| `--disable-memprotect` | false | Disable memory protection (debugging only) |

For the systemd-managed service, override flags via `ExecStart` in a drop-in file:

```bash
systemctl --user edit wsl-secret-service
```

### wincred-helper.exe Auto-Discovery

When `--helper-path` is not specified, the daemon searches these locations in order:

1. Same directory as the `wsl-secret-service` binary
2. `$XDG_DATA_HOME/wsl-secret-service/wincred-helper.exe`
3. `~/.local/share/wsl-secret-service/wincred-helper.exe`
4. `PATH` (includes Windows paths via WSL2 interop)

## Development

### Running Without WSL2

A Linux-native mock helper is provided for development and testing on any Linux machine (no WSL2 or Windows required):

```bash
# Build and run the daemon with the mock helper
make run-dev
```

This stores secrets in `bin/dev-store.jsonl` instead of Windows Credential Manager.

## Testing

### Unit and Integration Tests

```bash
make test
```

This runs `go test ./...` across all packages.

### End-to-End Tests

E2E tests verify the full D-Bus API surface. See [docs/e2e-testing.md](docs/e2e-testing.md) for full details.

**Without WSL2** (uses mock helper):

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
make e2e-test-debug    # show all commands
make e2e-clean         # clean up test environment
```

E2E tests cover:
- Collection management (create, list, delete)
- Secret storage and retrieval with encryption
- Attribute-based search functionality

## Troubleshooting

### Service Won't Start

- Verify the systemd user instance is running: `systemctl --user list-units`
- Check that `DBUS_SESSION_BUS_ADDRESS` is set: `echo $DBUS_SESSION_BUS_ADDRESS`
- Check logs: `journalctl --user -u wsl-secret-service`

### Helper Not Found

The daemon searches for `wincred-helper.exe` in the locations listed under [wincred-helper.exe Auto-Discovery](#wincred-helperexe-auto-discovery). If it is not found there, specify the path explicitly:

```bash
wsl-secret-service --helper-path /path/to/wincred-helper.exe
```

Also verify that WSL interop is enabled in Windows (`wsl.exe --status`).

### D-Bus Connection Issues

If `DBUS_SESSION_BUS_ADDRESS` is not set, the systemd user instance may not be running. Check its status and start it if needed:

```bash
systemctl --user status
systemctl --user start dbus
```

Then restart the service:

```bash
systemctl --user restart wsl-secret-service
```

### Build Issues

- Ensure Go 1.26.0 is installed
- Run `go mod tidy` to resolve dependencies

## License

Licensed under the Apache License 2.0. See [LICENSE](LICENSE) for details.

## Links

- [Freedesktop.org Secret Service Specification](https://specifications.freedesktop.org/secret-service/0.2/)
- [WSL Documentation](https://learn.microsoft.com/en-us/windows/wsl/)
