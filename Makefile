.PHONY: build build-linux build-windows test clean install

# Output directory for compiled binaries.
BINDIR := bin

build: build-linux build-windows

build-linux:
	@mkdir -p $(BINDIR)
	GOOS=linux go build -trimpath -buildmode pie -o $(BINDIR)/wsl-secret-service ./cmd/wsl-secret-service

# Cross-compile the Windows helper EXE from Linux.
build-windows:
	@mkdir -p $(BINDIR)
	GOOS=windows go build -trimpath -buildmode pie -o $(BINDIR)/wincred-helper.exe ./cmd/wincred-helper

test:
	go test ./...

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
	@echo "  mkdir -p ~/.config/systemd/user"
	@echo "  cp wsl-secret-service.service ~/.config/systemd/user/"
	@echo "  systemctl --user daemon-reload"
	@echo "  systemctl --user enable --now wsl-secret-service"
