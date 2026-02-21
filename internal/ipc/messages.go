// SPDX-License-Identifier: Apache-2.0

package ipc

// Request is the JSON message sent to wincred-helper.exe on stdin.
type Request struct {
	Action string `json:"action"`           // "get", "set", "delete", "list"
	Target string `json:"target"`           // credential target name
	Secret string `json:"secret,omitempty"` // base64-encoded secret for "set"
	Filter string `json:"filter,omitempty"` // prefix filter for "list"
}

// Response is the JSON message received from wincred-helper.exe on stdout.
type Response struct {
	OK      bool     `json:"ok"`
	Secret  string   `json:"secret,omitempty"`  // base64-encoded secret for "get"
	Targets []string `json:"targets,omitempty"` // for "list"
	Error   string   `json:"error,omitempty"`
}
