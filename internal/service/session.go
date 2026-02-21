// SPDX-License-Identifier: Apache-2.0

package service

import (
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
// Only the "plain" algorithm is supported — secrets are transferred unencrypted
// over the local D-Bus session bus.
type Session struct {
	path dbus.ObjectPath
	conn *dbus.Conn
	svc  *Service
}

// Close implements org.freedesktop.Secret.Session.Close().
// It removes this session from the service registry and unexports its D-Bus object.
func (s *Session) Close() *dbus.Error {
	s.svc.sessions.remove(s.path)
	s.conn.Export(nil, s.path, SessionIface)
	return nil
}
