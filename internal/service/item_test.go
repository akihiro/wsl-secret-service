// SPDX-License-Identifier: Apache-2.0

package service

import "testing"

func TestNormalizeContentType(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{"empty defaults to text/plain", "", "text/plain"},
		{"legacy default is normalized", "text/plain; charset=utf8", "text/plain"},
		{"text/plain preserved", "text/plain", "text/plain"},
		{"client-supplied value preserved", "application/octet-stream", "application/octet-stream"},
		{"charset variant preserved", "text/plain; charset=utf-8", "text/plain; charset=utf-8"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := normalizeContentType(tt.in); got != tt.want {
				t.Errorf("normalizeContentType(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}
