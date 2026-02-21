// SPDX-License-Identifier: Apache-2.0

package service

import (
	"github.com/godbus/dbus/v5"
)

// Prompt is a stub implementation of org.freedesktop.Secret.Prompt.
// Since all collections are always unlocked and no master password is required,
// this service never needs real user interaction. The stub is exported at
// PromptStubObjPath but should never be called in normal operation.
//
// When a method returns "/" as the prompt path, clients must not call Prompt().
// This stub exists only for strict spec compliance.
type Prompt struct {
	path dbus.ObjectPath
	conn *dbus.Conn
}

// Prompt implements org.freedesktop.Secret.Prompt.Prompt(window-id).
// It immediately emits a Completed signal with dismissed=false and an empty result.
func (p *Prompt) Prompt(windowID string) *dbus.Error {
	_ = p.conn.Emit(
		p.path,
		PromptIface+".Completed",
		false,                // dismissed = false (operation proceeds)
		dbus.MakeVariant(""), // result = empty variant
	)
	return nil
}

// Dismiss implements org.freedesktop.Secret.Prompt.Dismiss().
// It emits a Completed signal with dismissed=true.
func (p *Prompt) Dismiss() *dbus.Error {
	_ = p.conn.Emit(
		p.path,
		PromptIface+".Completed",
		true,                 // dismissed = true
		dbus.MakeVariant(""), // result = empty variant
	)
	return nil
}
