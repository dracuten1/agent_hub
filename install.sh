#!/bin/bash
# install.sh — Install AgentHub workers and systemd service files.
# Run as root: sudo ./install.sh

set -euo pipefail

# Check for root (issue #8)
if [ "$EUID" -ne 0 ]; then
    echo "ERROR: This script must be run as root. Use: sudo ./install.sh"
    exit 1
fi

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
BINARY_NAME="agenthub-worker"
INSTALL_DIR="${INSTALL_DIR:-/usr/local/bin}"
SYSTEMD_DIR="${SYSTEMD_DIR:-/etc/systemd/system}"
ENVFILE="${ENVFILE_DIR:-/etc/default}/agenthub-worker"

echo "=== AgentHub Worker Installer ==="

# Build
echo "[1/5] Building..."
cd "$SCRIPT_DIR"
if command -v go >/dev/null 2>&1; then
    go build -o "$BINARY_NAME" ./cmd
else
    echo "ERROR: Go is not installed. Install from https://go.dev/"
    exit 1
fi

# Install binary
echo "[2/5] Installing binary to $INSTALL_DIR/$BINARY_NAME..."
install -m 0755 "$BINARY_NAME" "$INSTALL_DIR/$BINARY_NAME"

# Install systemd units
echo "[3/5] Installing systemd units..."
for svc in dev reviewer tester; do
    src="$SCRIPT_DIR/systemd/agenthub-worker-${svc}.service"
    dst="$SYSTEMD_DIR/agenthub-worker-${svc}.service"
    if [ -f "$src" ]; then
        install -m 0644 "$src" "$dst"
        echo "  installed: $dst"
    else
        echo "  WARNING: missing $src"
    fi
done

# Install env template
echo "[4/5] Installing environment template..."
if [ ! -f "$ENVFILE" ]; then
    if [ -f "$SCRIPT_DIR/systemd/agenthub-worker.env.example" ]; then
        install -m 0600 "$SCRIPT_DIR/systemd/agenthub-worker.env.example" "$ENVFILE"
        echo "  installed: $ENVFILE (edit this to set DAODUC_API_KEY)"
    fi
else
    echo "  skipped: $ENVFILE already exists"
fi

# Reload systemd
echo "[5/5] Reloading systemd daemon..."
systemctl daemon-reload

echo ""
echo "=== Installation complete ==="
echo ""
echo "Next steps:"
echo "  1. Edit $ENVFILE and set DAODUC_API_KEY"
echo "  2. Enable workers:"
echo "       sudo systemctl enable agenthub-worker-dev"
echo "       sudo systemctl enable agenthub-worker-reviewer"
echo "       sudo systemctl enable agenthub-worker-tester"
echo "  3. Start workers:"
echo "       sudo systemctl start agenthub-worker-dev"
echo "       sudo systemctl start agenthub-worker-reviewer"
echo "       sudo systemctl start agenthub-worker-tester"
echo "  4. Check status:"
echo "       make status"
echo ""
