// SPDX-License-Identifier: Apache-2.0

package service

import (
	"fmt"
	"sync"

	"github.com/godbus/dbus/v5"
)

// sessionRegistry tracks open D-Bus sessions keyed by their object path.
type sessionRegistry struct {
	mu       sync.Mutex
	sessions map[dbus.ObjectPath]*Session
}

func newSessionRegistry() *sessionRegistry {
	return &sessionRegistry{
		sessions: make(map[dbus.ObjectPath]*Session),
	}
}

func (r *sessionRegistry) add(s *Session) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.sessions[s.path] = s
}

func (r *sessionRegistry) remove(path dbus.ObjectPath) {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.sessions, path)
}

func (r *sessionRegistry) get(path dbus.ObjectPath) (*Session, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	s, ok := r.sessions[path]
	return s, ok
}

// Session represents an open Secret Service session with a client application.
// aesKey is nil for plain sessions (no encryption); 16 bytes for DH sessions.
type Session struct {
	path   dbus.ObjectPath
	conn   *dbus.Conn
	svc    *Service
	aesKey []byte // nil → plain; 16 bytes → dh-ietf1024-sha256-aes128-cbc-pkcs7
}

// encryptSecret encrypts plaintext for delivery over D-Bus.
// For plain sessions it is a no-op. For DH sessions it uses AES-128-CBC.
// Returns (parameters/IV, ciphertext).
func (s *Session) encryptSecret(plaintext []byte) (params, value []byte, err error) {
	if s.aesKey == nil {
		return []byte{}, plaintext, nil
	}
	iv, ciphertext, err := aesEncrypt(s.aesKey, plaintext)
	if err != nil {
		return nil, nil, fmt.Errorf("encrypt secret: %w", err)
	}
	return iv, ciphertext, nil
}

// decryptSecret decrypts a secret received over D-Bus.
// For plain sessions it is a no-op. For DH sessions it uses AES-128-CBC.
func (s *Session) decryptSecret(params, ciphertext []byte) ([]byte, error) {
	if s.aesKey == nil {
		return ciphertext, nil
	}
	if len(params) != 16 {
		return nil, fmt.Errorf("expected 16-byte IV, got %d bytes", len(params))
	}
	plaintext, err := aesDecrypt(s.aesKey, params, ciphertext)
	if err != nil {
		return nil, fmt.Errorf("decrypt secret: %w", err)
	}
	return plaintext, nil
}

// Close implements org.freedesktop.Secret.Session.Close().
// It removes this session from the service registry and unexports its D-Bus object.
func (s *Session) Close() *dbus.Error {
	s.svc.sessions.remove(s.path)
	_ = s.conn.Export(nil, s.path, SessionIface)
	return nil
}
