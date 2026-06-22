# rotom-ng-mock-connector

A CLI tool that connects **fake devices, workers, and controllers** to a running
RotomNG instance using the real WebSocket wire protocol.

Instead of mocking data inside the UI, it drives RotomNG with genuine
connections so the API and UI light up with live devices, workers, controllers,
request stats, and proxied RPC traffic — useful for local development and
load/smoke testing.

## What it simulates

For each simulated entity it speaks the same protocol RotomNG expects:

| Entity         | Endpoint            | Behaviour                                                                                  |
| -------------- | ------------------- | ----------------------------------------------------------------------------------------- |
| **Device**     | `GET /control`      | Sends the JSON init message, then answers management commands (memory, screen size, …).   |
| **Worker**     | `GET /`             | Sends a protobuf `WelcomeMessage`, then echoes a `SUCCESS` response for every proxied request. |
| **Controller** | `GET /controller`   | Registers (v2), logs in on its assigned worker, then sends periodic RPC requests.         |

This forms a complete pipeline: **controller → RotomNG → worker → RotomNG → controller**,
so request tracking, stats, and worker-in-use counts all populate.

Each connection self-heals: if it drops it reconnects after `-reconnect-delay`.

## Usage

```bash
go run ./apps/rotom-ng-mock-connector [flags]
```

### Common flags

| Flag                   | Default                 | Description                                            |
| ---------------------- | ----------------------- | ------------------------------------------------------ |
| `-devices`             | `1`                     | Number of devices to simulate                          |
| `-workers`             | `1`                     | Workers per device                                     |
| `-controllers`         | `1`                     | Number of controllers                                  |
| `-device-endpoint`     | `ws://localhost:7070`   | Device listener (devices + workers)                    |
| `-controller-endpoint` | `ws://localhost:7071`   | Controller listener                                    |
| `-device-secret`       | _(empty)_               | `X-Rotom-Secret` for device/worker connections         |
| `-controller-secret`   | _(empty)_               | `X-Rotom-Secret` for controller connections            |
| `-origin`              | `mock`                  | Origin reported by devices/workers                     |
| `-id-prefix`           | `mock`                  | Prefix for generated IDs                                |
| `-rpc-interval`        | `30s`                   | How often each controller sends an RPC (must be < 5m)  |
| `-weight`              | `5`                     | Controller weight (1–10)                               |
| `-verbose`             | `false`                 | Debug logging                                          |

> **Note:** each controller claims one worker, so to keep every controller busy
> ensure `controllers ≤ devices × workers`. Extra workers stay connected but idle.

### Example

```bash
go run ./apps/rotom-ng-mock-connector \
  -devices=3 -workers=2 -controllers=4 \
  -device-endpoint=ws://localhost:7070 \
  -controller-endpoint=ws://localhost:7071 \
  -rpc-interval=5s
```

Press `Ctrl-C` to disconnect everything cleanly.

## One-command dev/test stack

The `gen-compose` subcommand writes a `docker-compose.yml` and a matching
RotomNG dev config that stand up the whole environment:

```bash
go run ./apps/rotom-ng-mock-connector gen-compose
docker compose up --build
```

The generated stack runs three services:

- **rotom-ng** — built from the repo `Dockerfile` with the `DEV_MODE=true` build
  arg (unminified embedded UI).
- **ui-dev** — a Vite dev server that mounts the repo source and serves the UI
  directly from `src` with hot reload (no build step). It shares the backend's
  network namespace so Vite's built-in `/api` proxy reaches it.
- **mock-connector** — a simulated fleet driving live connections in.

Then open:

- <http://localhost:4201> — live UI (hot reload from source)
- <http://localhost:7072> — backend API + embedded dev UI

`gen-compose` flags: `-out` (output dir), `-devices`, `-workers`,
`-controllers`, `-rpc-interval`, and `-force` (overwrite existing files).
