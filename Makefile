.PHONY: build test clean install install-systemd restart-dev restart-reviewer restart-tester status

BINARY=agenthub-worker
INSTALL_DIR=/usr/local/bin
SYSTEMD_DIR=/etc/systemd/system
ENVFILE_DIR=/etc/default

# Build all workers
build:
	go build -o bin/$(BINARY) ./cmd

# Build for Linux (cross-compile from macOS)
build-linux:
	GOOS=linux GOARCH=amd64 go build -o bin/$(BINARY) ./cmd

# Run tests
test:
	go test ./...

# Clean build artifacts
clean:
	rm -rf bin/

# Install binary
install: build
	install -m 0755 bin/$(BINARY) $(INSTALL_DIR)/$(BINARY)

# Install systemd units
install-systemd: install
	install -m 0644 systemd/agenthub-worker-dev.service $(SYSTEMD_DIR)/
	install -m 0644 systemd/agenthub-worker-reviewer.service $(SYSTEMD_DIR)/
	install -m 0644 systemd/agenthub-worker-tester.service $(SYSTEMD_DIR)/
	install -m 0644 systemd/agenthub-worker.env.example $(ENVFILE_DIR)/agenthub-worker
	systemctl daemon-reload

# Restart workers
restart-dev:
	systemctl restart agenthub-worker-dev

restart-reviewer:
	systemctl restart agenthub-worker-reviewer

restart-tester:
	systemctl restart agenthub-worker-tester

restart: restart-dev restart-reviewer restart-tester

# Worker status
status:
	@echo "=== Dev Worker ==="
	@systemctl status agenthub-worker-dev --no-pager || true
	@echo ""
	@echo "=== Reviewer Worker ==="
	@systemctl status agenthub-worker-reviewer --no-pager || true
	@echo ""
	@echo "=== Tester Worker ==="
	@systemctl status agenthub-worker-tester --no-pager || true

# Follow logs
logs-dev:
	journalctl -u agenthub-worker-dev -f

logs-reviewer:
	journalctl -u agenthub-worker-reviewer -f

logs-tester:
	journalctl -u agenthub-worker-tester -f

logs:
	journalctl -u agenthub-worker-dev -u agenthub-worker-reviewer -u agenthub-worker-tester -f
