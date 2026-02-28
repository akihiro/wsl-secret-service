# SPDX-License-Identifier: Apache-2.0

.PHONY: build build-linux build-windows build-mock-helper run-dev test e2e-test e2e-test-verbose e2e-test-debug e2e-clean clean install

# Output directory for compiled binaries.
BINDIR := bin

build: build-linux build-windows

build-linux:
	@mkdir -p $(BINDIR)
	CGO_ENABLED=0 GOEXPERIMENT=runtimesecret GOOS=linux go build -trimpath -buildmode pie -o $(BINDIR)/wsl-secret-service ./cmd/wsl-secret-service

# Cross-compile the Windows helper EXE from Linux.
build-windows:
	@mkdir -p $(BINDIR)
	CGO_ENABLED=0 GOOS=windows go build -trimpath -buildmode pie -o $(BINDIR)/wincred-helper.exe ./cmd/wincred-helper

# Build the Linux-native mock wincred helper for development/testing.
build-mock-helper:
	@mkdir -p $(BINDIR)
	CGO_ENABLED=0 GOOS=linux go build -trimpath -o $(BINDIR)/mock-wincred-helper ./cmd/mock-wincred-helper

# Run the daemon locally using the mock helper (no WSL2 required).
run-dev: build-linux build-mock-helper
	MOCK_WINCRED_STORE=$(BINDIR)/dev-store.json \
	$(BINDIR)/wsl-secret-service \
		--helper-path $(BINDIR)/mock-wincred-helper \
		--disable-memprotect

test:
	go test ./...

# End-to-end tests using secret-tool
e2e-test: build
	@bash tests/e2e/run-tests.sh

# Verbose E2E tests
e2e-test-verbose: build
	@bash tests/e2e/run-tests.sh -v

# Debug E2E tests (show each command)
e2e-test-debug: build
	@bash -x tests/e2e/run-tests.sh -v

# Clean E2E test environment
e2e-clean:
	@rm -rf ~/.config/wsl-secret-service/metadata.json
	@systemctl --user stop wsl-secret-service 2>/dev/null || true
	@pkill -f wsl-secret-service 2>/dev/null || true
	@echo "E2E test environment cleaned"

clean:
	rm -rf $(BINDIR)

# Install the Linux daemon to ~/.local/bin and the Windows helper alongside it.
install: build
	@mkdir -p ~/.local/bin ~/.local/share/wsl-secret-service
	cp $(BINDIR)/wsl-secret-service ~/.local/bin/wsl-secret-service
	cp $(BINDIR)/wincred-helper.exe ~/.local/share/wsl-secret-service/wincred-helper.exe
	@echo "Installed wsl-secret-service to ~/.local/bin/"
	@echo "Installed wincred-helper.exe to ~/.local/share/wsl-secret-service/"
	@echo ""
	@echo "To enable the systemd user service:"
	@echo "  mkdir -p ~/.config/systemd/user ~/.local/share/dbus-1/services"
	@echo "  cp wsl-secret-service.service ~/.config/systemd/user/"
	@echo "  cp org.freedesktop.secrets.service ~/.local/share/dbus-1/services/"
	@echo "  systemctl --user daemon-reload"
