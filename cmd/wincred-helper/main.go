//go:build windows

// wincred-helper is a Windows-side companion executable for wsl-secret-service.
// It is cross-compiled (GOOS=windows) and called from WSL2 via
// interop whenever the Linux daemon needs to access the Windows Credential Manager.
//
// Protocol: reads one JSON request line from stdin, writes one JSON response
// line to stdout, then exits. Exit code 0 means the response was written
// (including error responses where ok=false). Non-zero exit means a fatal error
// before a response could be written.
//
// Request fields:
//
//	action  string  "get" | "set" | "delete" | "list"
//	target  string  Windows Credential Manager TargetName
//	secret  string  base64-encoded CredentialBlob (only for "set")
//	filter  string  TargetName prefix for "list"
//
// Response fields:
//
//	ok      bool
//	secret  string  base64-encoded CredentialBlob (only for "get")
//	targets []string  matched TargetNames (only for "list")
//	error   string  human-readable error (only when ok=false)
package main

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/danieljoos/wincred"
	"github.com/akihiro/wsl-secret-service/internal/ipc"
)

func main() {
	var req ipc.Request
	dec := json.NewDecoder(os.Stdin)
	if err := dec.Decode(&req); err != nil {
		writeError(fmt.Sprintf("decode request: %v", err))
		os.Exit(1)
	}

	switch req.Action {
	case "get":
		handleGet(req.Target)
	case "set":
		handleSet(req.Target, req.Secret)
	case "delete":
		handleDelete(req.Target)
	case "list":
		handleList(req.Filter)
	default:
		writeError(fmt.Sprintf("unknown action: %q", req.Action))
		os.Exit(1)
	}
}

// handleGet retrieves a generic credential from Windows Credential Manager
// and writes its CredentialBlob (base64-encoded) in the response.
func handleGet(target string) {
	cred, err := wincred.GetGenericCredential(target)
	if err != nil {
		writeError(err.Error())
		return
	}
	writeOK(ipc.Response{
		OK:     true,
		Secret: base64.StdEncoding.EncodeToString(cred.CredentialBlob),
	})
}

// handleSet stores secret bytes (base64-encoded in request) as a generic
// credential in Windows Credential Manager with PersistLocalMachine scope.
func handleSet(target, secretB64 string) {
	secretBytes, err := base64.StdEncoding.DecodeString(secretB64)
	if err != nil {
		writeError(fmt.Sprintf("decode base64 secret: %v", err))
		return
	}

	cred := wincred.NewGenericCredential(target)
	cred.CredentialBlob = secretBytes
	cred.UserName = "wsl-secret-service"
	cred.Persist = wincred.PersistLocalMachine
	if err := cred.Write(); err != nil {
		writeError(err.Error())
		return
	}
	writeOK(ipc.Response{OK: true})
}

// handleDelete removes a generic credential from Windows Credential Manager.
func handleDelete(target string) {
	cred, err := wincred.GetGenericCredential(target)
	if err != nil {
		writeError(err.Error())
		return
	}
	if err := cred.Delete(); err != nil {
		writeError(err.Error())
		return
	}
	writeOK(ipc.Response{OK: true})
}

// handleList returns all TargetNames whose prefix matches filter.
// wincred.FilteredList uses a wildcard suffix internally; we pass filter+"*"
// to match all credentials under that prefix, then strip any trailing wildcard
// characters from results for clean output.
func handleList(filter string) {
	// FilteredList accepts a filter string where "*" acts as a wildcard.
	// Append "*" so we get all entries with the given prefix.
	pattern := filter
	if !strings.HasSuffix(pattern, "*") {
		pattern += "*"
	}

	creds, err := wincred.FilteredList(pattern)
	if err != nil {
		writeError(err.Error())
		return
	}

	targets := make([]string, 0, len(creds))
	for _, c := range creds {
		targets = append(targets, c.TargetName)
	}
	writeOK(ipc.Response{OK: true, Targets: targets})
}

func writeOK(r ipc.Response) {
	enc := json.NewEncoder(os.Stdout)
	_ = enc.Encode(r)
}

func writeError(msg string) {
	enc := json.NewEncoder(os.Stdout)
	_ = enc.Encode(ipc.Response{OK: false, Error: msg})
}
