# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

Perf Rabbit is a mobile app performance collector. It runs as a local HTTP server (net/http stdlib, Go 1.22+ pattern-based `http.ServeMux` routing — no third-party web framework) that collects CPU, memory, FPS, jank, and GPU metrics from connected Android and iOS devices, and serves a React frontend as a single embedded binary.

`cmd/main.go` doubles as the downloadable "Agent" for the `automation` platform's 性能测试 module (see `../docs/architecture-perf-rabbit-merge.md`): the central platform's browser page talks to it directly on `127.0.0.1:9527` for live device/collection control, then uploads the finished record to the platform's own `/api/perf/tasks` for MySQL storage. `cmd/app` (the standalone all-in-one product with its own embedded UI) is unrelated to that integration.

- **Android**: requires `adb` in PATH; uses ADB shell commands (`top`, `dumpsys gfxinfo`, `dumpsys SurfaceFlinger`, etc.)
- **iOS**: requires Python 3.8+ with `pymobiledevice3` (auto-installs if missing); uses `dvt sysmon` and `dvt graphics` subcommands

## Working Directory

All Go commands must be run from `client/`:

```
cd client
```

## Commands

### Run (dev — backend only, no embedded frontend)

```bash
go run ./cmd/main.go
```

Starts on port 9527 (override with `PERF_RABBIT_DEV_PORT`). Does not open a browser.

### Build (full app with embedded frontend)

The frontend must be built first so `web/dist/` is populated before Go embeds it:

```bash
# 1. Build frontend (run from client/web/ if it's a separate npm project)
#    The dist/ output lands at client/web/dist/

# 2. Build the Go binary
go build -o release/perf-rabbit ./cmd/app
```

### Cross-compile releases

```bash
# macOS ARM64
GOOS=darwin GOARCH=arm64 go build -o release/perf-rabbit-mac-arm64 ./cmd/app

# Windows AMD64
GOOS=windows GOARCH=amd64 go build -o release/perf-rabbit-windows-amd64.exe ./cmd/app
```

### Test

```bash
go test ./...                   # all tests
go test ./internal/perf/...     # single package
```

### Lint / vet

```bash
go vet ./...
```

## Architecture

### Two entry points

| File | Purpose |
|---|---|
| `cmd/main.go` | Dev entry: starts API server only, no frontend embedding, no browser open |
| `cmd/app/main.go` | Release entry: starts API + embedded frontend, auto-opens browser |

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
- **`internal/webapp`** — serves embedded `web/dist/` as static files; SPA fallback for non-`/api/` routes.
- **`web`** — embeds the compiled frontend via `//go:embed dist`.
- **`common`** — `AdbShell`/`AdbShellCommand` wrappers; `PythonCommand()` detects and caches the Python executable (respects `PERF_RABBIT_PYTHON`).

### iOS collector design

iOS CPU/memory uses a persistent background subprocess (`pymobiledevice3 developer dvt sysmon process monitor`) started once per task and kept alive for the task duration. The subprocess key is `deviceID::processName`. Callers read the latest sample from a shared `iosSysmonMonitor` via a mutex rather than spawning a new process each second.

iOS FPS/GPU uses a similar pattern in `ios_graphics.go`.

### Data flow for a collection task

1. `POST /api/collect/perf/start` → `start.StartCollectPerf` → `DefaultManager.Start()` → first sample collected synchronously, then goroutine ticks every second.
2. Frontend polls `GET /api/collect/perf/:taskId?from=<next_from>` — incremental slice returned.
3. `POST /api/collect/perf/:taskId/stop` → `DefaultManager.Stop()` → task goroutine exits, `SavePerfHistory()` writes JSON + CSV to `data/perf/`.
4. If the device disconnects mid-collection, the manager marks the task `interrupted` and still saves history.

## Environment Variables

| Variable | Default | Purpose |
|---|---|---|
| `PERF_RABBIT_PORT` | `8080` | Release app listen port |
| `PERF_RABBIT_DEV_PORT` | `9527` | Dev server listen port |
| `PERF_RABBIT_OPEN_BROWSER` | `true` | Set `false`/`0`/`no` to skip auto browser open |
| `PERF_RABBIT_PYTHON` | (auto-detect) | Path to Python executable for iOS support |

## History Storage

Completed tasks are saved under `data/perf/` (relative to where the binary runs):
- `<taskId>.json` — full record including all samples
- `<taskId>.csv` — flat table; generated at stop time, or on first CSV download if only JSON exists
