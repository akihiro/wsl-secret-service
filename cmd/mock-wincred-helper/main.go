// SPDX-License-Identifier: Apache-2.0

//go:build !windows

// mock-wincred-helper is a Linux-native stand-in for wincred-helper.exe used
// during development and testing in non-WSL2 environments. It stores secrets as
// a JSON Lines log in a file specified by the MOCK_WINCRED_STORE environment
// variable (default: /tmp/mock-wincred-store.jsonl).
//
// Each operation is appended as a new JSON Line with the following structure:
// - timestamp: ISO8601 UTC timestamp of the operation
// - action: "get", "set", "delete", or "list"
// - target/filter: The credential identifier or filter pattern
// - success: Whether the operation succeeded
// - error: Error message if unsuccessful
// - result: For "get" operations, the retrieved secret value
// - state: The complete credential store state after the operation
//
// This format enables easy debugging and audit trails of all operations.
//
// Protocol: identical to wincred-helper.exe — reads one JSON request line from
// stdin, writes one JSON response line to stdout, then exits.
//
// Usage:
//
//	MOCK_WINCRED_STORE=/path/to/store.jsonl ./bin/wsl-secret-service \
//	    --helper-path ./bin/mock-wincred-helper \
//	    --disable-memprotect
package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"syscall"
	"time"

	"github.com/akihiro/wsl-secret-service/internal/ipc"
)

func storePath() string {
	if p := os.Getenv("MOCK_WINCRED_STORE"); p != "" {
		return p
	}
	return "/tmp/mock-wincred-store.jsonl"
}

// LogEntry represents a single operation in the JSON Lines log
type LogEntry struct {
	Timestamp string            `json:"timestamp"`
	Action    string            `json:"action"`
	Target    string            `json:"target,omitempty"`
	Filter    string            `json:"filter,omitempty"`
	Secret    string            `json:"secret,omitempty"`
	Success   bool              `json:"success"`
	Error     string            `json:"error,omitempty"`
	Result    string            `json:"result,omitempty"`
	State     map[string]string `json:"state"`
}

func loadStore(f *os.File) (map[string]string, error) {
	store := make(map[string]string)
	if _, err := f.Seek(0, 0); err != nil {
		return nil, err
	}

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		var entry LogEntry
		if err := json.Unmarshal(scanner.Bytes(), &entry); err != nil {
			// Skip invalid lines
			continue
		}
		// Reconstruct state from the last entry with a valid state
		if entry.State != nil {
			store = entry.State
		}
	}

	return store, scanner.Err()
}

func appendLog(f *os.File, entry LogEntry) error {
	encoder := json.NewEncoder(f)
	return encoder.Encode(entry)
}

func handleGet(store map[string]string, target string) (ipc.Response, *LogEntry) {
	v, ok := store[target]
	if !ok {
		return ipc.Response{OK: false, Error: "credential not found"}, &LogEntry{
			Timestamp: time.Now().UTC().Format(time.RFC3339Nano),
			Action:    "get",
			Target:    target,
			Success:   false,
			Error:     "credential not found",
			State:     store,
		}
	}
	return ipc.Response{OK: true, Secret: v}, &LogEntry{
		Timestamp: time.Now().UTC().Format(time.RFC3339Nano),
		Action:    "get",
		Target:    target,
		Success:   true,
		Result:    v,
		State:     store,
	}
}

func handleSet(store map[string]string, target, secret string) (ipc.Response, *LogEntry) {
	store[target] = secret
	return ipc.Response{OK: true}, &LogEntry{
		Timestamp: time.Now().UTC().Format(time.RFC3339Nano),
		Action:    "set",
		Target:    target,
		Secret:    secret,
		Success:   true,
		State:     store,
	}
}

func handleDelete(store map[string]string, target string) (ipc.Response, *LogEntry) {
	if _, ok := store[target]; !ok {
		return ipc.Response{OK: false, Error: "credential not found"}, &LogEntry{
			Timestamp: time.Now().UTC().Format(time.RFC3339Nano),
			Action:    "delete",
			Target:    target,
			Success:   false,
			Error:     "credential not found",
			State:     store,
		}
	}
	delete(store, target)
	return ipc.Response{OK: true}, &LogEntry{
		Timestamp: time.Now().UTC().Format(time.RFC3339Nano),
		Action:    "delete",
		Target:    target,
		Success:   true,
		State:     store,
	}
}

func handleList(store map[string]string, filter string) (ipc.Response, *LogEntry) {
	targets := []string{}
	for k := range store {
		if strings.HasPrefix(k, filter) {
			targets = append(targets, k)
		}
	}
	return ipc.Response{OK: true, Targets: targets}, &LogEntry{
		Timestamp: time.Now().UTC().Format(time.RFC3339Nano),
		Action:    "list",
		Filter:    filter,
		Success:   true,
		State:     store,
	}
}

func writeResponse(r ipc.Response) {
	_ = json.NewEncoder(os.Stdout).Encode(r)
}

func main() {
	var req ipc.Request
	if err := json.NewDecoder(os.Stdin).Decode(&req); err != nil {
		writeResponse(ipc.Response{OK: false, Error: fmt.Sprintf("decode request: %v", err)})
		os.Exit(1)
	}

	f, err := os.OpenFile(storePath(), os.O_RDWR|os.O_CREATE, 0o600)
	if err != nil {
		writeResponse(ipc.Response{OK: false, Error: fmt.Sprintf("open store: %v", err)})
		os.Exit(1)
	}
	defer f.Close()

	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX); err != nil {
		writeResponse(ipc.Response{OK: false, Error: fmt.Sprintf("lock store: %v", err)})
		os.Exit(1)
	}
	defer syscall.Flock(int(f.Fd()), syscall.LOCK_UN) //nolint:errcheck

	store, err := loadStore(f)
	if err != nil {
		writeResponse(ipc.Response{OK: false, Error: fmt.Sprintf("load store: %v", err)})
		os.Exit(1)
	}

	var resp ipc.Response
	var logEntry *LogEntry

	switch req.Action {
	case "get":
		resp, logEntry = handleGet(store, req.Target)
	case "set":
		resp, logEntry = handleSet(store, req.Target, req.Secret)
	case "delete":
		resp, logEntry = handleDelete(store, req.Target)
	case "list":
		resp, logEntry = handleList(store, req.Filter)
	default:
		logEntry = &LogEntry{
			Timestamp: time.Now().UTC().Format(time.RFC3339Nano),
			Action:    req.Action,
			Success:   false,
			Error:     fmt.Sprintf("unknown action: %q", req.Action),
			State:     store,
		}
		resp = ipc.Response{OK: false, Error: logEntry.Error}
	}

	// Append log entry
	if _, err := f.Seek(0, 2); err != nil { // Seek to end
		writeResponse(ipc.Response{OK: false, Error: fmt.Sprintf("seek store: %v", err)})
		os.Exit(1)
	}
	if err := appendLog(f, *logEntry); err != nil {
		writeResponse(ipc.Response{OK: false, Error: fmt.Sprintf("write log: %v", err)})
		os.Exit(1)
	}

	writeResponse(resp)
}
