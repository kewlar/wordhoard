# 0002. Single Go Binary + SQLite (WAL) Shared Between TUI Ingester and Web Server

Status: accepted

## Context and Decision

The system consists of a terminal-based (TUI) book ingestion tool and a browser-based vocabulary explorer and quiz application (Go backend + TypeScript SPA frontend). Both operate on the same local machine for a single user.

We will package the backend as a single Go module (and binary with `ingest` and `serve` subcommands) backed by a single local SQLite database configured with `PRAGMA journal_mode=WAL;`. This allows the user to run the TUI ingestion tool in a terminal while the web server is actively running without database locking conflicts, and avoids needing an external database server.
