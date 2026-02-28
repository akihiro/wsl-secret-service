// SPDX-License-Identifier: Apache-2.0

//go:build !windows

// mock-wincred-helper is a Linux-native stand-in for wincred-helper.exe used
// during development and testing in non-WSL2 environments. It stores secrets as
// a JSON map in a file specified by the MOCK_WINCRED_STORE environment variable
// (default: /tmp/mock-wincred-store.json).
//
// Protocol: identical to wincred-helper.exe — reads one JSON request line from
// stdin, writes one JSON response line to stdout, then exits.
//
// Usage:
//
//	MOCK_WINCRED_STORE=/path/to/store.json ./bin/wsl-secret-service \
//	    --helper-path ./bin/mock-wincred-helper \
//	    --disable-memprotect
package main

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"syscall"

	"github.com/akihiro/wsl-secret-service/internal/ipc"
)

func storePath() string {
	if p := os.Getenv("MOCK_WINCRED_STORE"); p != "" {
		return p
	}
	return "/tmp/mock-wincred-store.json"
}

func loadStore(f *os.File) (map[string]string, error) {
	store := make(map[string]string)
	info, err := f.Stat()
	if err != nil {
		return nil, err
	}
	if info.Size() == 0 {
		return store, nil
	}
	if err := json.NewDecoder(f).Decode(&store); err != nil {
		return nil, fmt.Errorf("decode store: %w", err)
	}
	return store, nil
}

func saveStore(f *os.File, store map[string]string) error {
	if err := f.Truncate(0); err != nil {
		return err
	}
	if _, err := f.Seek(0, 0); err != nil {
		return err
	}
	return json.NewEncoder(f).Encode(store)
}

func handleGet(store map[string]string, target string) ipc.Response {
	v, ok := store[target]
	if !ok {
		return ipc.Response{OK: false, Error: "credential not found"}
	}
	return ipc.Response{OK: true, Secret: v}
}

func handleSet(store map[string]string, target, secret string) ipc.Response {
	store[target] = secret
	return ipc.Response{OK: true}
}

func handleDelete(store map[string]string, target string) ipc.Response {
	if _, ok := store[target]; !ok {
		return ipc.Response{OK: false, Error: "credential not found"}
	}
	delete(store, target)
	return ipc.Response{OK: true}
}

func handleList(store map[string]string, filter string) ipc.Response {
	targets := []string{}
	for k := range store {
		if strings.HasPrefix(k, filter) {
			targets = append(targets, k)
		}
	}
	return ipc.Response{OK: true, Targets: targets}
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
	var mutated bool

	switch req.Action {
	case "get":
		resp = handleGet(store, req.Target)
	case "set":
		resp = handleSet(store, req.Target, req.Secret)
		mutated = true
	case "delete":
		resp = handleDelete(store, req.Target)
		if resp.OK {
			mutated = true
		}
	case "list":
		resp = handleList(store, req.Filter)
	default:
		resp = ipc.Response{OK: false, Error: fmt.Sprintf("unknown action: %q", req.Action)}
	}

	if mutated && resp.OK {
		if err := saveStore(f, store); err != nil {
			writeResponse(ipc.Response{OK: false, Error: fmt.Sprintf("save store: %v", err)})
			os.Exit(1)
		}
	}

	writeResponse(resp)
}
