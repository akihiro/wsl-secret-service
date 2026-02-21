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
//	--config-dir   path   Config/metadata directory (default: $XDG_CONFIG_HOME/wsl-secret-service)
//	--helper-path  path   Path to wincred-helper.exe (default: auto-discover)
//	--replace            Replace an existing org.freedesktop.secrets name owner
package main

import (
	"flag"
	"log"
	"os"
	"path/filepath"

	"github.com/akihiro/wsl-secret-service/internal/backend/wincred"
	"github.com/akihiro/wsl-secret-service/internal/service"
	"github.com/akihiro/wsl-secret-service/internal/store"
	"github.com/godbus/dbus/v5"
)

func main() {
	configDir := flag.String("config-dir", defaultConfigDir(), "metadata storage directory")
	helperPath := flag.String("helper-path", "", "path to wincred-helper.exe (auto-discovered if empty)")
	replace := flag.Bool("replace", false, "replace an existing org.freedesktop.secrets owner")
	flag.Parse()

	log.SetPrefix("wsl-secret-service: ")
	log.SetFlags(0)

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

	// Start the Secret Service.
	if _, err := service.New(conn, st, be); err != nil {
		log.Fatalf("start secret service: %v", err)
	}
	log.Printf("org.freedesktop.secrets is ready")

	// Block until the D-Bus connection closes.
	select {}
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
