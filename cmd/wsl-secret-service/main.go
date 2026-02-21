// SPDX-License-Identifier: Apache-2.0

//go:build linux

// wsl-secret-service is a Freedesktop.org Secret Service daemon for WSL2.
// It exposes the org.freedesktop.secrets D-Bus service and stores secrets
// in the Windows Credential Manager via a companion wincred-helper.exe.
//
// Usage:
//
//	wsl-secret-service [flags]
//
// Flags:
//
//	--config-dir         path   Config/metadata directory (default: $XDG_CONFIG_HOME/wsl-secret-service)
//	--helper-path        path   Path to wincred-helper.exe (default: auto-discover)
//	--replace                   Replace an existing org.freedesktop.secrets name owner
//	--disable-memprotect        [DEBUG] Disable memory protection (prctl, mlockall)
package main

import (
	"context"
	"flag"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/akihiro/wsl-secret-service/internal/backend/wincred"
	"github.com/akihiro/wsl-secret-service/internal/memprotect"
	"github.com/akihiro/wsl-secret-service/internal/service"
	"github.com/akihiro/wsl-secret-service/internal/store"
	"github.com/godbus/dbus/v5"
)

func main() {
	configDir := flag.String("config-dir", defaultConfigDir(), "metadata storage directory")
	helperPath := flag.String("helper-path", "", "path to wincred-helper.exe (auto-discovered if empty)")
	replace := flag.Bool("replace", false, "replace an existing org.freedesktop.secrets owner")
	disableMemprotect := flag.Bool("disable-memprotect", false, "[DEBUG] disable memory protection (prctl, mlockall)")
	timeout := flag.Duration("timeout", 30*time.Second, "shutdown daemon after this period of inactivity")
	flag.Parse()

	log.SetPrefix("wsl-secret-service: ")
	log.SetFlags(0)

	// Harden the process against memory inspection by same-user processes.
	// prctl(PR_SET_DUMPABLE,0) blocks /proc/<pid>/mem reads and ptrace.
	// mlockall pins pages in RAM so secrets never reach swap.
	if *disableMemprotect {
		log.Printf("[DEBUG] memory protection disabled")
	} else {
		if err := memprotect.HardenProcess(); err != nil {
			log.Fatalf("harden process: %v", err)
		}
		log.Printf("memory protections applied")
	}

	// Connect to the session D-Bus.
	conn, err := dbus.ConnectSessionBus()
	if err != nil {
		log.Fatalf("connect to session bus: %v\n"+
			"hint: ensure DBUS_SESSION_BUS_ADDRESS is set (run: export $(dbus-launch))", err)
	}
	defer func() {
		if err := conn.Close(); err != nil {
			log.Printf("close D-Bus connection: %v", err)
		}
	}()

	// Request the well-known bus name.
	nameFlags := dbus.NameFlagDoNotQueue
	if *replace {
		nameFlags |= dbus.NameFlagReplaceExisting
	}
	reply, err := conn.RequestName(service.BusName, nameFlags)
	if err != nil {
		log.Fatalf("request D-Bus name %s: %v", service.BusName, err)
	}
	if reply != dbus.RequestNameReplyPrimaryOwner {
		log.Fatalf("D-Bus name %s is already owned (use --replace to take it over)", service.BusName)
	}
	log.Printf("claimed D-Bus name: %s", service.BusName)

	// Initialise the metadata store.
	st, err := store.New(*configDir)
	if err != nil {
		log.Fatalf("open metadata store at %s: %v", *configDir, err)
	}
	log.Printf("metadata store: %s", *configDir)

	// Initialise the Windows Credential Manager backend.
	be, err := wincred.New(*helperPath)
	if err != nil {
		log.Fatalf("init wincred backend: %v\n"+
			"hint: build wincred-helper.exe with 'make build-windows' and place it alongside this binary", err)
	}
	log.Printf("wincred backend ready")

	// Create a context for graceful shutdown.
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Start the Secret Service with timeout.
	if _, err := service.New(ctx, conn, st, be, *timeout); err != nil {
		log.Fatalf("start secret service: %v", err)
	}
	log.Printf("org.freedesktop.secrets is ready")

	// Set up signal handling for graceful shutdown.
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGTERM, syscall.SIGINT)

	// Block until shutdown signal or context cancellation.
	select {
	case <-ctx.Done():
		log.Printf("shutdown initiated (idle timeout)")
	case sig := <-sigChan:
		log.Printf("received signal: %v, shutting down", sig)
		cancel()
	}
}

// defaultConfigDir returns the XDG-compliant config directory for the service.
func defaultConfigDir() string {
	if xdg := os.Getenv("XDG_CONFIG_HOME"); xdg != "" {
		return filepath.Join(xdg, "wsl-secret-service")
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return ".wsl-secret-service"
	}
	return filepath.Join(home, ".config", "wsl-secret-service")
}
