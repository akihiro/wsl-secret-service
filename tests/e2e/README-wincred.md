# wincred-helper E2E Tests

Direct end-to-end tests for `wincred-helper.exe` using IPC (Inter-Process Communication) JSON messages.

## Overview

These tests validate the `wincred-helper.exe` binary directly without requiring:
- A running Secret Service daemon
- D-Bus
- systemd
- The full Secret Service stack

The tests communicate directly with the helper via stdin/stdout using newline-delimited JSON messages, matching the protocol used by the `internal/backend/wincred` bridge.

## Test Cases

### 1. **test_wincred_helper_exists**
Validates that the wincred-helper binary is present and executable.

### 2. **test_wincred_set_and_get**
- Sets a secret with `set` action
- Retrieves it with `get` action
- Verifies the retrieved value matches the original
- Cleans up afterward

### 3. **test_wincred_delete**
- Sets a secret
- Deletes it with `delete` action
- Verifies it no longer exists (get fails)

### 4. **test_wincred_list**
- Sets multiple secrets with the same prefix
- Lists secrets with prefix filter
- Verifies all items appear in results
- Cleans up all items

### 5. **test_wincred_large_secret**
- Tests storing a 1KB secret
- Verifies size is preserved during round-trip

### 6. **test_wincred_special_chars**
- Stores a secret with special characters: `!@#$%^&*()_+-={}[]|:;"<>?,./`
- Verifies special characters survive base64 encoding/decoding

### 7. **test_wincred_overwrite**
- Sets an initial secret
- Overwrites it with a different value
- Verifies the new value is returned on get

### 8. **test_wincred_multiple_secrets**
- Sets 5 secrets sequentially
- Retrieves and validates each one
- Verifies no corruption or cross-contamination

### 9. **test_wincred_nonexistent**
- Attempts to get a non-existent target
- Verifies the response has `ok: false`
- Verifies an error message is included

### 10. **test_wincred_empty_secret**
- Sets an empty secret (zero-length plaintext)
- Verifies it can be stored and retrieved without error

## Running the Tests

### Build and test
```bash
# Run all wincred-helper tests
make e2e-test-wincred

# Run with verbose output
make e2e-test-wincred-verbose
```

### Direct execution
```bash
# Ensure the helper is built
make build-windows

# Run tests directly
bash tests/e2e/run-tests-wincred.sh

# With verbose output
bash tests/e2e/run-tests-wincred.sh -v
```

## Protocol Details

### Request Format
```json
{
  "action": "set|get|delete|list",
  "target": "credential-target-name",
  "secret": "base64-encoded-secret",  // for "set"
  "filter": "prefix-filter"            // for "list"
}
```

### Response Format
```json
{
  "ok": true|false,
  "secret": "base64-encoded-secret",   // for "get"
  "targets": ["target1", "target2"],    // for "list"
  "error": "error message"
}
```

### Supported Actions

| Action | Parameters | Response |
|--------|-----------|----------|
| `get` | target | secret (base64) |
| `set` | target, secret (base64) | ok status |
| `delete` | target | ok status |
| `list` | filter (prefix) | targets array |

## Encoding

All secrets are transmitted as base64-encoded strings:
- Plaintext → base64 → JSON string → wire
- Wire → JSON string → base64 → plaintext

This ensures binary data can be safely transmitted through the JSON interface.

## Test Output

Passing tests produce:
```
[PASS] test_wincred_set_and_get
```

Failing tests produce:
```
[FAIL] test_wincred_delete
  Reason: Secret still exists after delete
```

## Troubleshooting

### Helper not found
```
wincred-helper not found: /home/user/.local/share/wsl-secret-service/wincred-helper.exe
```
**Solution**: Run `make install` to build and install the helper.

### JSON parsing errors
Verify the helper binary is properly compiled for your architecture:
```bash
file ~/.local/share/wsl-secret-service/wincred-helper.exe
# Should show: PE32+ executable (console), x86-64
```

### Cleanup
The tests clean up their own test credentials. If a test is interrupted:
```bash
# List all test credentials in Windows Credential Manager
~/.local/share/wsl-secret-service/wincred-helper.exe <<< '{"action":"list","filter":"test/wincred"}'
```

## Files

- `test-wincred-helper.sh` - Test implementations
- `run-tests-wincred.sh` - Test runner and setup
- `Makefile` - Build targets (`e2e-test-wincred`, `e2e-test-wincred-verbose`)
