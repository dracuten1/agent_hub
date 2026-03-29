# AgentHub Workers

Worker scripts for the AgentHub multi-agent pipeline.

## Workers

| Worker | Task Type | Description |
|--------|-----------|-------------|
| `dev` | Dev | Polls dev queue, executes tasks via OpenCode, reports results |
| `reviewer` | Review | Polls review queue, runs code review, reports findings |
| `tester` | Test | Polls test queue, runs test suites, reports results |

## Quick Start

### Prerequisites
- Go 1.21+
- AgentHub API server running at `http://localhost:8081`
- OpenCode API key (set `DAODUC_API_KEY`)

### Build
```bash
make build        # build all workers
make build-linux # cross-compile for Linux
```

### Run manually
```bash
./bin/agenthub-worker dev
./bin/agenthub-worker reviewer
./bin/agenthub-worker tester
```

### Install & run as systemd services
```bash
# As root:
sudo ./install.sh

# Or manually:
sudo make install-systemd

# Edit DAODUC_API_KEY:
sudo nano /etc/default/agenthub-worker

# Start workers:
sudo systemctl start agenthub-worker-dev
sudo systemctl start agenthub-worker-reviewer
sudo systemctl start agenthub-worker-tester

# Check status:
make status

# Follow logs:
make logs
```

## Environment Variables

| Variable | Default | Required | Description |
|----------|---------|----------|-------------|
| `AGENTHUB_URL` | `http://localhost:8081` | No | AgentHub API URL |
| `AGENTHUB_TOKEN` | (none) | No | API auth token |
| `DAODUC_API_KEY` | (none) | **Yes** | OpenCode API key |

Set `DAODUC_API_KEY` in `/etc/default/agenthub-worker`.

## Architecture

```
AgentHub API (port 8081)
    ├── polls /tasks/poll?type=dev       → Dev Worker (OpenCode)
    ├── polls /tasks/poll?type=review    → Reviewer Worker (OpenCode)
    └── polls /tasks/poll?type=test      → Tester Worker (cargo/npm test)
```

## Systemd Units

- `agenthub-worker-dev.service` — Dev worker
- `agenthub-worker-reviewer.service` — Reviewer worker
- `agenthub-worker-tester.service` — Tester worker

All include:
- Auto-restart on failure (5s backoff)
- Health watchdog (dev/reviewer: 10min, tester: 30min)
- Journal logging (`journalctl -u agenthub-worker-*`)
- Security hardening (NoNewPrivileges, PrivateTmp, ProtectSystem)

## Project Structure

```
agenthub/
├── main.go              # API server
├── go.mod
├── schema.sql           # Database schema
├── workers/
│   ├── base.go          # Framework: API client, Worker loop, OpenCode runner
│   ├── dev.go           # Dev worker processor
│   ├── review.go        # Reviewer worker processor
│   └── test.go         # Tester worker processor
├── cmd/
│   └── main.go          # CLI entry point (go run main.go <worker>)
├── systemd/
│   ├── agenthub-worker-dev.service
│   ├── agenthub-worker-reviewer.service
│   ├── agenthub-worker-tester.service
│   └── agenthub-worker.env.example
├── install.sh           # Installer script
└── Makefile
```
