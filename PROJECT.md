# AgentHub — ocTeam Task Management API

## Overview
Task management API + worker framework for the AI development team (ocTeam). Go backend (Gin + PostgreSQL) with React dashboard. Workers poll for tasks and execute via OpenCode.

## Tech Stack
Go 1.21, Gin, PostgreSQL 15, sqlx, JWT auth, Docker Compose, React (Vite)

## Repo
Local project at `projects/agenthub/` (git initialized, no remote)

## Status: IN PROGRESS — Bug fixes needed

## Components
- **API Server** (port 8081) — REST API with JWT + API key auth
- **Workers** — Dev, Reviewer, Tester binaries (systemd or standalone)
- **Web Dashboard** — React frontend (Vite)
- **ocTeam Driver** — Cron health check script

## Known Issues (2026-03-30 PM Review)
See tasks.md for detailed breakdown
