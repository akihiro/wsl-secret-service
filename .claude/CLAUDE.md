# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

**wsl-secret-service** is a Freedesktop.org Secret Service daemon for WSL2. It bridges Linux (WSL) and Windows credential storage by exposing the `org.freedesktop.secrets` D-Bus interface and storing secrets in the Windows Credential Manager via a companion helper executable.

- **License**: Apache 2.0
- **Go Version**: 1.26.0
- **Key Binaries**:
  - `wsl-secret-service` (Linux daemon)
  - `wincred-helper.exe` (Windows helper)

## Build & Development Commands

### Build
```bash
# Build both Linux daemon and Windows helper
make build

# Build only Linux daemon
make build-linux

# Build only Windows helper
make build-windows
```

**Important build details**:
- CGO is disabled (`CGO_ENABLED=0`) for static linking
- Uses `GOEXPERIMENT=runtimesecret` for memory protection features
- Both binaries built with `-trimpath` and `-buildmode pie` (position-independent executables)
- Output directory: `./bin/`

### Testing
```bash
# Run all tests
go test ./...

# Run tests for a specific package
go test ./internal/store/...

# Run a single test
go test -run TestNewCreatesLoginCollection ./internal/store/...

# Run tests with verbose output and coverage
go test -v -cover ./...
```

### Installation
```bash
# Build and install binaries
make install
```

Then follow the printed instructions to enable the systemd user service.

### Clean
```bash
make clean
```

## Architecture Overview

The service implements the Freedesktop.org Secret Service specification via D-Bus. The architecture consists of three main layers:

### 1. **D-Bus Service Layer** (`/internal/service/`)
- **Service** (`service.go`): Root D-Bus object at `/org/freedesktop/secrets`, exports D-Bus methods and properties
- **Collection** (`collection.go`): Manages groups of secrets, D-Bus object per collection
- **Item** (`item.go`): Individual secret entries within collections
- **Session** (`session.go`): Encryption sessions for client connections, manages per-client state
- **Prompt** (`prompt.go`): Stub implementation for auth prompts (minimally implemented)
- **Crypto** (`crypto.go`): Session key derivation (DH-IETF1024-SHA256-AES128-CBC-PKCS7 algorithm)
- **Types** (`types.go`): D-Bus interface definitions and constants

D-Bus lifecycle:
1. Service connects to session bus and claims `org.freedesktop.secrets` name
2. Existing collections are loaded and exported as D-Bus objects
3. `NameOwnerChanged` signal monitored to clean up orphaned sessions when clients disconnect

### 2. **Metadata Store** (`/internal/store/`)
- **Store** (`store.go`): Thread-safe persistent storage for collection and item metadata
- Contains only metadata: labels, attributes, timestamps, content type
- Data persisted as JSON to `$XDG_CONFIG_HOME/wsl-secret-service/metadata.json`
- Automatically creates "login" collection with "default" alias on first run
- Provides item lookup by UUID and collection name

### 3. **Backend Layer** (`/internal/backend/`)
- **Backend interface** (`backend.go`): Abstract storage interface for raw secret bytes
- **Wincred implementation** (`wincred/bridge.go`, `wincred/bridge_test.go`): Uses Windows Credential Manager via `wincred-helper.exe`
  - Launches helper process and communicates via IPC (`/internal/ipc/messages.go`)
  - Helper discovered via `--helper-path` flag or auto-discovery (searches common install paths)

### Data Flow
1. **Store secret**: Client → D-Bus Service → Backend (wincred) → Windows Credential Manager
2. **Retrieve secret**: Client → D-Bus Service → Backend (wincred) → Windows Credential Manager
3. **Metadata only**: Store ↔ Persistent JSON file

### Memory Protection

The main daemon hardens process memory via `memprotect` package to prevent same-user inspection:
- Sets `prctl(PR_SET_DUMPABLE, 0)` to block `/proc/<pid>/mem` reads and ptrace
- Calls `mlockall()` to pin pages in RAM, preventing secrets from reaching swap
- Invoked early in `main()` before any secrets are loaded

## Key Packages

| Package | Responsibility |
|---------|-----------------|
| `internal/service` | D-Bus interface implementation and object lifecycle |
| `internal/store` | Persistent metadata management (JSON-based) |
| `internal/backend` | Abstract secret storage interface |
| `internal/backend/wincred` | Windows Credential Manager backend via helper EXE |
| `internal/memprotect` | Process memory hardening (Linux-specific) |
| `internal/ipc` | Inter-process communication with wincred-helper |
| `cmd/wsl-secret-service` | Main daemon entry point |
| `cmd/wincred-helper` | Windows helper that calls Credential Manager APIs |

## Important Development Notes

### Windows Build Cross-Compilation
The Windows helper (`wincred-helper.exe`) is cross-compiled from Linux to Windows:
```bash
GOOS=windows go build ./cmd/wincred-helper
```
This is fully self-hosted in the makefile and does not require Windows to build.

### D-Bus Error Handling
- Errors returned to D-Bus clients use standard org.freedesktop.DBus error names
- Collection/item not found errors map to `org.freedesktop.DBus.Error.NotFound`
- Invalid data errors map to `org.freedesktop.DBus.Error.InvalidArgs`

### Memory-Protected Variables
The codebase uses `runtime/secret` type annotations to mark sensitive data. These integrate with the memory hardening to prevent secrets from appearing in core dumps or memory inspection.

### D-Bus Path Generation
D-Bus object paths use UUID-based addressing with underscores (not hyphens) in path components to comply with D-Bus naming requirements (see recent fix: 6102b9d).

### Session Encryption
Sessions use the dh-ietf1024-sha256-aes128-cbc-pkcs7 algorithm (implemented in crypto.go). This is a Diffie-Hellman key exchange followed by AES-128-CBC encryption with PKCS#7 padding.

### Config Directory
- Respects `XDG_CONFIG_HOME` environment variable
- Defaults to `~/.config/wsl-secret-service/`
- Can be overridden via `--config-dir` flag
- Stores `metadata.json` and potentially other state files
