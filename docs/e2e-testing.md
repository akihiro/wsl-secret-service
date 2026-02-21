# End-to-End Testing Guide

This document describes how to run end-to-end (e2e) tests for `wsl-secret-service`.

## Overview

E2E tests verify the D-Bus layer functionality by simulating real-world usage patterns using the `secret-tool` command. These tests cover:

- **Collection Management**: Creating, listing, and deleting secret collections
- **Secret Operations**: Storing, retrieving, and deleting secrets via the Secret Service D-Bus interface
- **Attribute-Based Search**: Searching secrets by multiple attributes

## Prerequisites

### System Requirements

- **WSL2 Environment**: Tests must run on Windows Subsystem for Linux 2
- **Systemd User Instance**: Working user-level systemd service system
- **D-Bus Session Bus**: Active D-Bus session (usually available by default in WSL2)
- **Bash 4.0+**: For script execution

### Required Packages

Install the following packages on your WSL2 system:

```bash
sudo apt-get update
sudo apt-get install -y libsecret-tools dbus-tools jq
```

**Package Details**:
- `libsecret-tools`: Provides `secret-tool` command-line interface to Secret Service
- `dbus-tools`: Provides `dbus-send` and related D-Bus utilities for direct D-Bus interface calls
- `jq`: JSON query processor for metadata validation

### Optional Packages

```bash
sudo apt-get install -y dbus-user-session
```

For debugging:
```bash
sudo apt-get install -y dbus-x11
```

## Setup

### 1. Build the Service

```bash
cd /path/to/wsl-secret-service
make build
```

This creates:
- `bin/wsl-secret-service` (Linux daemon)
- `bin/wincred-helper.exe` (Windows helper)

### 2. Install Binaries

```bash
make install
```

This places:
- Daemon at `~/.local/bin/wsl-secret-service`
- Helper at `~/.local/share/wsl-secret-service/wincred-helper.exe`

### 3. Ensure D-Bus Session is Active

```bash
# Check D-Bus session availability
echo $DBUS_SESSION_BUS_ADDRESS

# If not set, initialize it
if [ -z "$DBUS_SESSION_BUS_ADDRESS" ]; then
    eval $(dbus-launch --sh-syntax)
    export DBUS_SESSION_BUS_ADDRESS
fi
```

## Running E2E Tests

### Quick Start

Run all e2e tests:

```bash
make e2e-test
```

### Test Variants

**Verbose output** (show more details):
```bash
make e2e-test-verbose
```

**Debug mode** (show each command as executed):
```bash
make e2e-test-debug
```

**Manual test execution**:
```bash
bash tests/e2e/run-tests.sh
bash tests/e2e/run-tests.sh -v    # verbose
```

### Cleaning Up After Tests

```bash
make e2e-clean
```

This removes test metadata and stops any running daemon.

## secret-tool Usage Guide

`secret-tool` is a command-line interface to the Secret Service D-Bus API. Here are the main commands used in tests:

### Store a Secret

```bash
# Store a secret with label and attributes
printf '%s' "my-password-value" | secret-tool store \
    --label "My Service Password" \
    service github.com \
    username alice \
    environment production
```

**Parameters**:
- `--label "..."`: Human-readable label for the secret
- `KEY VALUE`: One or more attribute key-value pairs

### Search for Secrets

```bash
# Search by single attribute
secret-tool search service github.com

# Search by multiple attributes (AND condition)
secret-tool search service github.com username alice
```

### Retrieve a Secret

```bash
# Get the first matching secret's value
secret-tool search service github.com username alice | head -n1

# Get from specific collection
secret-tool get collection login service github.com username alice
```

### Retrieve Secret Value

```bash
# Get the secret value (decrypts automatically)
secret-tool lookup service github.com username alice
```

### Delete a Secret

```bash
# Delete by attributes
secret-tool delete service github.com username alice

# Delete from specific collection
secret-tool delete --collection login service github.com
```

### List All Secrets

```bash
# Show all stored secrets
secret-tool search --all
```

## Understanding Collections

Collections are groups of secrets. The service automatically creates a "login" collection on first run.

### Default Collections

- **login**: Default collection for storing credentials (auto-created)
- **default**: Alias pointing to the login collection (auto-created)

### Accessing Collections via secret-tool

Secrets are stored in collections but typically remain implicit when using high-level `secret-tool` commands. The service manages collection paths internally as `/org/freedesktop/secrets/collection/<NAME>`.

## Test Cases

E2E tests are organized into three categories:

### Collection Management Tests

Located in `tests/e2e/test-collections.sh`:

1. **test_default_login_collection_exists**: Verifies that "login" collection is auto-created
2. **test_create_collection_via_secret_tool**: Confirms implicit collection creation
3. **test_list_collections**: Retrieves collection list via D-Bus
4. **test_delete_collection_removes_items**: Validates cascading deletion
5. **test_collection_persistence**: Checks metadata.json persistence

### Secret Operation Tests

Located in `tests/e2e/test-secrets.sh`:

1. **test_store_and_retrieve_secret**: Complete store → search → retrieve cycle
2. **test_retrieve_with_encryption_session**: Validates session encryption
3. **test_secret_deleted_after_unlock_and_delete**: Confirms deletion removes secret
4. **test_large_secret_storage**: Tests multi-KB secrets
5. **test_special_characters_in_secret**: Validates UTF-8 and special characters
6. **test_secret_content_type_preserved**: Verifies metadata integrity

### Attribute Search Tests

Located in `tests/e2e/test-attributes-search.sh`:

1. **test_search_by_single_attribute**: Single attribute matching
2. **test_search_by_multiple_attributes**: Multi-attribute AND queries
3. **test_empty_search_returns_all**: Wildcard search
4. **test_search_no_match_returns_empty**: Empty result set
5. **test_search_within_collection**: Scoped collection search
6. **test_case_sensitive_attribute_search**: Attribute case sensitivity

## Troubleshooting

### D-Bus Connection Errors

**Problem**: "ERROR: D-Bus connection failed"

**Solution**:
```bash
# Ensure D-Bus session is initialized
eval $(dbus-launch --sh-syntax)
export DBUS_SESSION_BUS_ADDRESS

# Verify with
dbus-send --session --print-reply \
    --dest=org.freedesktop.DBus \
    /org/freedesktop/DBus \
    org.freedesktop.DBus.ListNames
```

### secret-tool Not Found

**Problem**: "ERROR: secret-tool not found"

**Solution**:
```bash
# Install libsecret-tools
sudo apt-get install libsecret-tools

# Verify installation
secret-tool --version
```

### Service Won't Start

**Problem**: "ERROR: Failed to start service"

**Solution**:
```bash
# Check systemd user session
systemctl --user list-units

# Enable user session if needed
systemctl --user daemon-reload

# Check for previous instances
pkill -f wsl-secret-service

# Verify binaries exist
ls -la ~/.local/bin/wsl-secret-service
ls -la ~/.local/share/wsl-secret-service/wincred-helper.exe
```

### Metadata Issues

**Problem**: "ERROR: metadata.json not found or invalid"

**Solution**:
```bash
# Clear metadata (will be recreated on service start)
rm -rf ~/.config/wsl-secret-service/metadata.json

# Verify directory exists
mkdir -p ~/.config/wsl-secret-service

# Check permissions
ls -la ~/.config/wsl-secret-service/
```

### Service Timeout

**Problem**: Tests timeout waiting for service startup

**Solution**:
```bash
# Check if service is actually running
systemctl --user status wsl-secret-service

# View service logs
journalctl --user -u wsl-secret-service --no-pager

# Try manual startup with debug output
~/.local/bin/wsl-secret-service --disable-memprotect -v
```

## Environment Variables

The following environment variables affect test execution:

| Variable | Purpose | Default |
|----------|---------|---------|
| `DBUS_SESSION_BUS_ADDRESS` | D-Bus session address | Auto-detected |
| `XDG_CONFIG_HOME` | Config directory base | `~/.config` |
| `XDG_DATA_HOME` | Data directory base | `~/.local/share` |

## Advanced: Manual D-Bus Introspection

Inthe test suite to understand D-Bus interface structure:

```bash
# Introspect the Secret Service object
dbus-send --session \
    --print-reply \
    --dest=org.freedesktop.secrets \
    /org/freedesktop/secrets \
    org.freedesktop.DBus.Introspectable.Introspect

# Get service properties
dbus-send --session \
    --print-reply \
    --dest=org.freedesktop.secrets \
    /org/freedesktop/secrets \
    org.freedesktop.DBus.Properties.GetAll \
    string:org.freedesktop.Secret.Service

# List all collections
dbus-send --session \
    --print-reply \
    --dest=org.freedesktop.secrets \
    /org/freedesktop/secrets \
    org.freedesktop.Secret.Service.GetCollections
```

## Continuous Integration

For CI/CD environments:

```bash
# Run tests with CI exit codes
make ci-e2e
```

GitHub Actions example:
```yaml
- name: Install test dependencies
  run: sudo apt-get install -y libsecret-tools dbus jq

- name: Run E2E tests
  run: make ci-e2e
```

## Performance Notes

- Typical test execution time: 10-30 seconds (single run)
- D-Bus startup wait: Up to 30 seconds (auto-timeout)
- Metadata I/O: Minimal (<100ms per operation)

## Debugging Failed Tests

When a test fails, the test runner outputs detailed debug information:

```bash
# Auto-collected on failure:
- Service status (systemctl)
- secret-tool version
- D-Bus introspection output
- Current metadata.json content
- Recent journalctl logs
```

To capture this manually:
```bash
# Run test in verbose + debug mode
make e2e-test-debug 2>&1 | tee test-output.log

# Inspect specific secret
secret-tool search service example.com

# Check metadata structure
jq . ~/.config/wsl-secret-service/metadata.json
```

## Further Reading

- [Freedesktop.org Secret Service Specification](https://specifications.freedesktop.org/secret-service/0.2/)
- [secret service man page](https://linux.die.net/man/1/secret-tool)
- [WSL2 Documentation](https://docs.microsoft.com/en-us/windows/wsl/)
