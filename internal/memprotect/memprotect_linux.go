// SPDX-License-Identifier: Apache-2.0

//go:build linux

// Package memprotect applies OS-level hardening to protect secret material
// held in process memory from inspection by other processes running as the
// same user.
package memprotect

import (
	"fmt"
	"log"

	"golang.org/x/sys/unix"
)

// HardenProcess applies two protections and must be called as early as
// possible in main(), before any secret material is loaded.
//
//  1. prctl(PR_SET_DUMPABLE, 0) — disables core dumps and makes
//     /proc/<pid>/mem and /proc/<pid>/maps unreadable by non-root processes,
//     including processes running as the same UID.  It also prevents ptrace
//     attachment by unprivileged peers.
//
//  2. mlockall(MCL_CURRENT|MCL_FUTURE) — pins all present and future memory
//     pages in RAM so they are never written to swap, which would otherwise
//     leave secret material on disk in plaintext.
func HardenProcess() error {
	if err := unix.Prctl(unix.PR_SET_DUMPABLE, 0, 0, 0, 0); err != nil {
		return fmt.Errorf("prctl PR_SET_DUMPABLE=0: %w", err)
	}

	if err := unix.Mlockall(unix.MCL_CURRENT | unix.MCL_FUTURE); err != nil {
		// mlockall may fail in restricted container environments or when
		// RLIMIT_MEMLOCK is too small.  Log a warning rather than aborting
		// so the service still runs with the dumpable protection active.
		log.Printf("warning: mlockall failed (secrets may reach swap): %v", err)
	}

	return nil
}

