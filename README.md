# wsl-secret-service

A Freedesktop.org Secret Service daemon for WSL2 that bridges Linux applications and Windows credential storage.

## Description

`wsl-secret-service` provides a secure way for applications running in WSL2 (Windows Subsystem for Linux 2) to store and retrieve sensitive information like passwords, API keys, and certificates. It implements the standard `org.freedesktop.secrets` D-Bus interface, allowing Linux apps to interact with secrets seamlessly, while storing the actual secret data in the Windows Credential Manager for better integration with the host system.

The daemon runs as a background service in WSL2 and communicates with a companion Windows executable (`wincred-helper.exe`) to access Windows credentials securely.

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

### Install

1. Install the binaries:
   ```bash
   make install
   ```
   This copies the daemon to `~/.local/bin/` and the helper to `~/.local/share/wsl-secret-service/`.

2. Enable the systemd user service:
   ```bash
   mkdir -p ~/.config/systemd/user ~/.local/share/dbus-1/services
   cp wsl-secret-service.service ~/.config/systemd/user/
   cp org.freedesktop.secrets.service ~/.local/share/dbus-1/services/
   systemctl --user daemon-reload
   systemctl --user enable --now wsl-secret-service
   ```

3. Verify installation:
   ```bash
   systemctl --user status wsl-secret-service
   ```

The service should now be running and available via D-Bus.

## Usage

Once installed and running, applications can automatically discover and use the secret service through the standard D-Bus interface. No manual configuration is typically required.

### For Application Developers

Applications can interact with the service using D-Bus calls to `org.freedesktop.secrets`. Common operations include:

- **Store a secret**: Create items in collections with attributes for easy lookup
- **Retrieve secrets**: Search by attributes and unlock items
- **Manage collections**: Create, delete, and list secret collections

### Example Use Cases

- Password managers storing credentials
- Applications caching API tokens
- Browsers managing saved passwords
- Development tools storing access keys

### Checking Service Status

```bash
# Check if the service is running
systemctl --user status wsl-secret-service

# View service logs
journalctl --user -u wsl-secret-service
```

## Configuration

The daemon can be configured via command-line flags when started manually:

- `--config-dir <path>`: Directory for metadata storage (default: `~/.config/wsl-secret-service`)
- `--helper-path <path>`: Path to `wincred-helper.exe` (default: auto-discovered)
- `--replace`: Replace existing D-Bus name owner
- `--disable-memprotect`: Disable memory protection (debugging only)

For systemd-managed service, modify the service file or use environment variables.

## Troubleshooting

### Service Won't Start

- Ensure D-Bus is running: Check `DBUS_SESSION_BUS_ADDRESS` environment variable
- Verify systemd user instance: `systemctl --user list-units`
- Check logs: `journalctl --user -u wsl-secret-service`

### Helper Not Found

- Ensure `wincred-helper.exe` is built and accessible
- Check auto-discovery paths or specify `--helper-path`
- Verify WSL interop is enabled in Windows

### D-Bus Connection Issues

- Run `export $(dbus-launch)` if `DBUS_SESSION_BUS_ADDRESS` is not set
- Restart the service: `systemctl --user restart wsl-secret-service`

### Build Issues

- Ensure Go 1.26.0 is installed
- Run `go mod tidy` to resolve dependencies
- Check for CGO requirements (disabled for static builds)

## License

Licensed under the Apache License 2.0. See [LICENSE](LICENSE) for details.

## Links

- [Freedesktop.org Secret Service Specification](https://specifications.freedesktop.org/secret-service/0.2/)
- [WSL Documentation](https://docs.microsoft.com/en-us/windows/wsl/)
