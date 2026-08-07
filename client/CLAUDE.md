# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

This package (originally the standalone tool "Perf Rabbit") is the local mobile-device performance collector Agent for the `automation` platform's 性能测试 module (see `../docs/architecture-perf-rabbit-merge.md`). It runs as a local HTTP server (net/http stdlib, Go 1.22+ pattern-based `http.ServeMux` routing — no third-party web framework) that collects CPU, memory, FPS, jank, and GPU metrics from connected Android and iOS devices.

`cmd/main.go` is the only entry point: it starts the API only, no embedded frontend, no auto-opened browser. The central platform's browser page (`web/src/pages/PerfTestPage.tsx` / `PerfHistoryPage.tsx` in the root `web/` app) talks to it directly on `127.0.0.1:9527` for live device/collection control, then uploads the finished record to the platform's own `/api/perf/tasks` for MySQL storage. There used to be a second entry point (`cmd/app`, a standalone all-in-one product with its own embedded React UI) — it was unrelated to the platform integration and was deleted once the merge into `automation` was complete, so there's only ever one frontend for this feature (the root `web/` app), not two.

- **Android**: requires `adb` in PATH; uses ADB shell commands (`top`, `dumpsys gfxinfo`, `dumpsys SurfaceFlinger`, etc.)
- **iOS**: requires Python 3.8+ with `pymobiledevice3` (auto-installs if missing); uses `dvt sysmon` and `dvt graphics` subcommands

## Module

This is a regular package tree inside the root `interface-load-test` Go module — import paths are `interface-load-test/client/...`. It used to be its own Go module (`module client`, independent `go.mod`, at `perf-rabbit/client/`) during Phase 1-3 of the merge into `automation` — that was a deliberately cheap, reversible decision (see `../docs/architecture-perf-rabbit-merge.md` design principle 5) to defer dealing with dependency overlap until it actually mattered. Phase 4 merged it into the root module and moved it to `client/` at the repo root (dropping the `perf-rabbit/` wrapper directory entirely). All commands below run from the **repo root**.

## Commands

### Run (dev)

```bash
go run ./client/cmd/main.go
```

Starts on port 9527 (override with `PERF_RABBIT_DEV_PORT`). Does not open a browser.

### Cross-compile the Agent for distribution

```bash
make perf-agent-build   # from repo root; cross-compiles macOS arm64/amd64 + Windows amd64 into assets/perf-agent/
```

Or manually:

```bash
GOOS=darwin GOARCH=arm64 go build -o assets/perf-agent/perf-agent-darwin-arm64 ./client/cmd/main.go
GOOS=windows GOARCH=amd64 go build -o assets/perf-agent/perf-agent-windows-amd64.exe ./client/cmd/main.go
```

### Test

```bash
go test ./...                       # whole repo, including this package — one module now
go test ./client/internal/perf/...  # single package
```

### Lint / vet

```bash
go vet ./...
```

## Architecture

### Key packages

- **`internal/server`** — route registration (`RegisterAPI`), port config, and middleware: `CORSMiddleware` (reflects any Origin — this process only binds 127.0.0.1 and has no auth/cookies to protect, so there's no cost to allowing cross-origin browser access) and `AccessLogMiddleware` (replaces Gin's removed default request logger; its `statusRecorder` forwards `Hijack`/`Unwrap` — dropping either breaks the `/ws/collect/perf/:taskId` WebSocket upgrade, since `coder/websocket` finds the real hijacker through `Unwrap`). All HTTP routes live here.
- **`internal/perf`** — core collection engine:
  - `manager.go`: `Manager` struct handles task lifecycle. `DefaultManager` is the singleton. One goroutine per running task, enforces one task per (deviceID, packageName) pair.
  - `collect.go`: `CollectPerformanceMetrics` — dispatches to Android or iOS collectors, called every second by the manager.
  - `history.go`: reads/writes `data/perf/<taskId>.json` and `.csv`; handlers for history list/detail/delete/CSV download.
  - `history_csv.go`: CSV serialization.
  - `query.go`: handler for real-time incremental polling (`GET /api/collect/perf/:taskId?from=N`).
  - `hub.go` + `ws.go`: `GET /ws/collect/perf/:taskId` pushes incremental samples instead of the frontend polling on a timer. `Hub` is a per-task "wake up, no payload" signal — subscribers re-fetch via the same `GetTask(fromSample)` snapshot logic the polling handler uses, so there's no separate push-data code path to keep correct.
  - `start/`, `stop/` — HTTP handlers that call `DefaultManager.Start()`/`.Stop()`.
  - `get/`: individual metric collectors — `cpu.go`, `memory.go`, `fps.go`, `jank.go` (Android); `ios_sysmon.go` (CPU+memory via long-lived `pymobiledevice3` subprocess), `ios_graphics.go` (FPS+GPU), `ios_core.go` (CPU core count by model).
- **`internal/device_list`** — queries ADB and pymobiledevice3 in parallel to produce the device list.
- **`internal/get_device_apps`** — `pm list packages` (Android) / `pymobiledevice3 apps list` (iOS).
- **`common`** — `AdbShell`/`AdbShellCommand` wrappers; `PythonCommand()` detects and caches the Python executable (respects `PERF_RABBIT_PYTHON`); `AgentVersion` (see below).

### iOS collector design

iOS CPU/memory uses a persistent background subprocess (`pymobiledevice3 developer dvt sysmon process monitor`) started once per task and kept alive for the task duration. The subprocess key is `deviceID::processName`. Callers read the latest sample from a shared `iosSysmonMonitor` via a mutex rather than spawning a new process each second.

iOS FPS/GPU uses a similar pattern in `ios_graphics.go`.

### Data flow for a collection task

1. `POST /api/collect/perf/start` → `start.StartCollectPerf` → `DefaultManager.Start()` → first sample collected synchronously, then goroutine ticks every second.
2. Frontend subscribes to `GET /ws/collect/perf/:taskId` for incremental pushes (`GET /api/collect/perf/:taskId?from=<next_from>` still works as a polling fallback).
3. `POST /api/collect/perf/:taskId/stop` → `DefaultManager.Stop()` → task goroutine exits, `SavePerfHistory()` writes JSON + CSV to `data/perf/`.
4. If the device disconnects mid-collection, the manager marks the task `interrupted` and still saves history.
5. The browser fetches the final record and forwards it to the platform's `POST /api/perf/tasks` (using the logged-in user's session, not a separate Agent credential) — see the root `web/src/hooks/usePerfUploadRetryQueue.ts` for the local retry-queue that covers network blips during that forward.

### Agent version reporting

`common.AgentVersion` is returned by `GET /api/agent/info`. The platform's frontend (`web/src/api/perfAgent.ts`, `MIN_COMPATIBLE_AGENT_VERSION`) compares against it and prompts the user to upgrade if this Agent build is too old. Bump both constants together whenever the Agent↔browser protocol changes.

## Environment Variables

| Variable | Default | Purpose |
|---|---|---|
| `PERF_RABBIT_DEV_PORT` | `9527` | Listen port (override with `PERF_RABBIT_DEV_PORT`) |
| `PERF_RABBIT_PYTHON` | (auto-detect) | Path to Python executable for iOS support |

## History Storage

Completed tasks are saved under `data/perf/` (relative to where the binary runs) as a local fallback/debug copy — the record that matters is the one forwarded to the platform's MySQL via `/api/perf/tasks`:
- `<taskId>.json` — full record including all samples
- `<taskId>.csv` — flat table; generated at stop time, or on first CSV download if only JSON exists
