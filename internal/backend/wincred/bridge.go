// SPDX-License-Identifier: Apache-2.0

// Package wincred provides a backend that stores secrets in the Windows
// Credential Manager by invoking a companion wincred-helper.exe via WSL2
// interop. Communication uses newline-delimited JSON over stdin/stdout.
package wincred

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/akihiro/wsl-secret-service/internal/backend"
	"github.com/akihiro/wsl-secret-service/internal/ipc"
)

// Bridge implements backend.Backend by calling wincred-helper.exe.
type Bridge struct {
	helperPath string
}

// New creates a Bridge that uses the wincred-helper.exe at helperPath.
// If helperPath is empty, the helper is discovered automatically (see findHelper).
func New(helperPath string) (*Bridge, error) {
	if helperPath == "" {
		discovered, err := findHelper()
		if err != nil {
			return nil, fmt.Errorf("wincred-helper not found: %w", err)
		}
		helperPath = discovered
	}
	return &Bridge{helperPath: helperPath}, nil
}

// findHelper searches for wincred-helper.exe in standard locations.
func findHelper() (string, error) {
	var candidates []string

	// 1. Same directory as the running daemon binary.
	if exe, err := os.Executable(); err == nil {
		candidates = append(candidates, filepath.Join(filepath.Dir(exe), "wincred-helper.exe"))
	}

	// 2. $XDG_DATA_HOME/wsl-secret-service/wincred-helper.exe
	if xdgData := os.Getenv("XDG_DATA_HOME"); xdgData != "" {
		candidates = append(candidates, filepath.Join(xdgData, "wsl-secret-service", "wincred-helper.exe"))
	}

	// 3. ~/.local/share/wsl-secret-service/wincred-helper.exe
	if home, err := os.UserHomeDir(); err == nil {
		candidates = append(candidates, filepath.Join(home, ".local", "share", "wsl-secret-service", "wincred-helper.exe"))
	}

	// 4. PATH (includes Windows paths via WSL2 interop).
	if path, err := exec.LookPath("wincred-helper.exe"); err == nil {
		candidates = append(candidates, path)
	}

	for _, c := range candidates {
		if _, err := os.Stat(c); err == nil {
			return c, nil
		}
	}
	return "", errors.New("wincred-helper.exe not found; " +
		"place it alongside wsl-secret-service or in ~/.local/share/wsl-secret-service/")
}

// call invokes wincred-helper.exe with the given request and returns the response.
func (b *Bridge) call(req ipc.Request) (*ipc.Response, error) {
	reqData, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}
	reqData = append(reqData, '\n')

	cmd := exec.Command(b.helperPath)
	cmd.Stdin = bytes.NewReader(reqData)
	out, err := cmd.Output()
	if err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			return nil, fmt.Errorf("wincred-helper exited %d: %s", exitErr.ExitCode(), string(exitErr.Stderr))
		}
		return nil, fmt.Errorf("run wincred-helper: %w", err)
	}

	var resp ipc.Response
	if err := json.Unmarshal(bytes.TrimSpace(out), &resp); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}
	return &resp, nil
}

// Get returns the raw secret bytes for the given target.
func (b *Bridge) Get(target string) ([]byte, error) {
	resp, err := b.call(ipc.Request{Action: "get", Target: target})
	if err != nil {
		return nil, err
	}
	if !resp.OK {
		if isNotFound(resp.Error) {
			return nil, &backend.ErrNotFound{Target: target}
		}
		return nil, fmt.Errorf("wincred get %q: %s", target, resp.Error)
	}
	decoded, err := base64.StdEncoding.DecodeString(resp.Secret)
	if err != nil {
		return nil, fmt.Errorf("decode secret: %w", err)
	}
	return decoded, nil
}

// Set stores raw secret bytes under the given target.
func (b *Bridge) Set(target string, secret []byte) error {
	if len(secret) > 2560 {
		return fmt.Errorf("secret too large for Windows Credential Manager (max 2560 bytes, got %d)", len(secret))
	}
	encoded := base64.StdEncoding.EncodeToString(secret)
	resp, err := b.call(ipc.Request{Action: "set", Target: target, Secret: encoded})
	if err != nil {
		return err
	}
	if !resp.OK {
		return fmt.Errorf("wincred set %q: %s", target, resp.Error)
	}
	return nil
}

// Delete removes the secret for the given target.
func (b *Bridge) Delete(target string) error {
	resp, err := b.call(ipc.Request{Action: "delete", Target: target})
	if err != nil {
		return err
	}
	if !resp.OK {
		if isNotFound(resp.Error) {
			return &backend.ErrNotFound{Target: target}
		}
		return fmt.Errorf("wincred delete %q: %s", target, resp.Error)
	}
	return nil
}

// List returns all target strings that have the given prefix.
func (b *Bridge) List(prefix string) ([]string, error) {
	resp, err := b.call(ipc.Request{Action: "list", Filter: prefix})
	if err != nil {
		return nil, err
	}
	if !resp.OK {
		return nil, fmt.Errorf("wincred list %q: %s", prefix, resp.Error)
	}
	return resp.Targets, nil
}

// isNotFound reports whether an error message indicates a missing credential.
func isNotFound(errMsg string) bool {
	lower := strings.ToLower(errMsg)
	return strings.Contains(lower, "not found") ||
		strings.Contains(lower, "element not found") ||
		strings.Contains(lower, "no such")
}
