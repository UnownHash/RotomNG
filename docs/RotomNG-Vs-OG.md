# Rotom NG Migration Guide

This document describes the differences between Rotom OG and Rotom NG for those upgrading. It covers HTTP API changes and Prometheus metrics changes.

## HTTP API Changes

### Authentication

OG had no authentication on the HTTP API. NG supports optional authentication via the `X-Rotom-Secret` header, configured with `http_listener.secret` in the config file. When configured, all `/api` endpoints require this header or return `401 Unauthorized`.

### HTTP Method Changes

Several endpoints changed from `POST` to `PUT` to better reflect their semantics:

| Endpoint | OG Method | NG Method |
|----------|-----------|-----------|
| Device actions | `POST` | `PUT` |
| Job execution | `POST` | `PUT` |

### Metrics Endpoint Moved

| OG | NG |
|----|-----|
| `GET /metrics` (always enabled) | `GET /api/metrics` (must enable with `prometheus.enable = true` in config) |

When disabled, `GET /api/metrics` returns `404`.

### Status Endpoint

**`GET /api/status`** - The response structure changed:

| OG field | NG field | Notes |
|----------|----------|-------|
| `devices` | `devices` | Field names changed (see below) |
| `workers` | `controllers` | Renamed; structure changed |

Notable device field renames and additions in NG:

| OG | NG |
|----|-----|
| `isAlive` | `is_connected` |
| `dateLastMessageReceived` | `message_last_received_at_ms` |
| `dateLastMessageSent` | `message_last_sent_at_ms` |
| `noMessagesReceived` | `messages_received` |
| `noMessagesSent` | `messages_sent` |
| `dateConnected` | `last_connected_at_ms` |
| `lastMemory` | `last_memory` |
| `instanceNo` | _(removed)_ |
| `heartbeatCheckStatus` | _(removed)_ |
| `nextId` | _(removed)_ |
| `init` | _(removed)_ |
| _(n/a)_ | `enabled` |
| _(n/a)_ | `can_be_used` |
| _(n/a)_ | `is_in_use` |
| _(n/a)_ | `worker_count` |
| _(n/a)_ | `worker_in_use_count` |
| _(n/a)_ | `worker_in_use_percent` |
| _(n/a)_ | `bytes_received` |
| _(n/a)_ | `bytes_sent` |

NG uses `snake_case` for all JSON field names (OG used `camelCase`).

### Removed Endpoints

| OG Endpoint | Notes |
|-------------|-------|
| `GET /api/getPublicIp` | Public IP is now available in device objects via `GET /api/status` and `GET /api/device` |
| `POST /api/ptcLogin` | Removed entirely |

### Device Endpoints

**`DELETE /api/device`** (delete all inactive) is replaced by:
- `PUT /api/device/_/action/delete` - uses `_` as a wildcard device ID

**`POST /api/device/:deviceId/action/:action`** became **`PUT /api/device/:deviceId/action/:action`**:

| OG action | NG action | Notes |
|-----------|-----------|-------|
| `restart` | `restart` | Unchanged |
| `reboot` | `reboot` | Unchanged |
| `getLogcat` | `logcat` | Renamed |
| `delete` | `delete` | Unchanged |
| _(n/a)_ | `disable` | New - disable device from selection |
| _(n/a)_ | `enable` | New - re-enable device for selection |
| _(n/a)_ | `disconnect` | New - disconnect device |

OG returned `{ status, error }` for most actions. NG returns `{ status, message }`.

### New Device Endpoints in NG

| Endpoint | Description |
|----------|-------------|
| `GET /api/device` | List all devices (supports `?include_workers=true`) |
| `GET /api/device/:deviceId` | Get a single device |

### Job Endpoints

The job API was restructured:

| OG | NG | Notes |
|----|-----|-------|
| `GET /api/job/list` | `GET /api/job` | Path changed |
| `POST /api/job/execute/:jobId/:deviceIds?` | `PUT /api/job/:jobId/run` | Method and path changed; device IDs now sent as JSON body `{"device_ids": [...]}` instead of URL parameter |
| `GET /api/job/status` | `GET /api/job-instance` | Renamed to job-instance |
| `GET /api/job/status/:jobNo` | `GET /api/job-instance/:id` | Renamed |

New job endpoints in NG:

| Endpoint | Description |
|----------|-------------|
| `GET /api/job/:jobId` | Get a specific job definition |
| `PUT /api/job/-/reload` | Reload jobs from disk |
| `PUT /api/job-instance/:id/clear` | Clear a job instance result (use `-` to clear all) |

Job instance response fields changed:

| OG | NG |
|----|-----|
| `executionComplete` | `status` (enum: PENDING, RUNNING, COMPLETED, FAILED) |
| `success` | _(derived from `status`)_ |
| `result` | `result` |
| _(n/a)_ | `started_at_ms` |
| _(n/a)_ | `finished_at_ms` |

### New Endpoints in NG

| Endpoint | Description |
|----------|-------------|
| `GET /api/config` | Get current system configuration including version, tuning, and rate limit settings |
| `PUT /api/config/reload` | Reload configuration from file |
| `GET /api/controller` | List all connected controllers |
| `GET /api/controller/:uuid` | Get a specific controller |
| `PUT /api/controller/:uuid/action/:action` | Execute controller actions (`disconnect`, `reconnect`) |
| `GET /api/debug/pprof/*` | Go pprof profiling endpoints (requires `tuning.profiling = true`) |

---

## Prometheus Metrics Changes

### Renamed Metrics

| OG Metric | NG Metric | Notes |
|-----------|-----------|-------|
| `rotom_devices_alive` | `rotom_devices_connected` | Now includes `origin` label |
| `rotom_devices_total` | `rotom_devices_total` | Now includes `origin` label (OG had no labels) |
| `rotom_workers_total` | `rotom_workers_connected` | Renamed for clarity |
| `rotom_workers_active` | `rotom_workers_in_use` | Renamed for clarity |

### Unchanged Metrics

| Metric | Notes |
|--------|-------|
| `rotom_device_memory_free` | Same name and `origin` label |
| `rotom_device_memory_mitm` | Same name and `origin` label |
| `rotom_device_memory_start` | Same name and `origin` label |

### Removed Metrics

| Metric | Notes |
|--------|-------|
| Node.js default metrics (`nodejs_*`, `process_*`) | NG is written in Go; these are replaced by Go runtime metrics |

### Added Metrics

**Device command metrics:**

| Metric | Type | Labels | Description |
|--------|------|--------|-------------|
| `rotom_device_commands_executed_total` | Counter | `origin`, `command` | Total device commands executed |
| `rotom_device_commands_success_total` | Counter | `origin`, `command` | Successful device commands |
| `rotom_device_commands_error_total` | Counter | `origin`, `command` | Failed device commands |

**Registration metrics:**

| Metric | Type | Labels | Description |
|--------|------|--------|-------------|
| `rotom_device_registrations_total` | Counter | `origin` | Total device registrations |
| `rotom_device_registration_fails_total` | Counter | _(none)_ | Failed device registrations |
| `rotom_worker_registrations_total` | Counter | `origin` | Total worker registrations |
| `rotom_worker_registration_fails_total` | Counter | _(none)_ | Failed worker registrations |

**Connection accept metrics:**

| Metric | Type | Labels | Description |
|--------|------|--------|-------------|
| `rotom_device_control_accepts_total` | Counter | _(none)_ | Device control connection accepts |
| `rotom_device_control_accept_fails_total` | Counter | _(none)_ | Failed device control accepts |
| `rotom_worker_accepts_total` | Counter | _(none)_ | Worker connection accepts |
| `rotom_worker_accept_fails_total` | Counter | _(none)_ | Failed worker accepts |
| `rotom_controller_accepts_total` | Counter | _(none)_ | Controller connection accepts |
| `rotom_controller_accept_fails_total` | Counter | _(none)_ | Failed controller accepts |

**Worker request/response metrics:**

| Metric | Type | Labels | Description |
|--------|------|--------|-------------|
| `rotom_worker_requests_total` | Counter | `method` | Total worker requests |
| `rotom_worker_responses_total` | Counter | `method`, `status`, `error` | Total worker responses |
| `rotom_worker_response_duration_seconds` | Histogram | `method`, `status` | Worker response latency |
| `rotom_worker_dropped_responses_total` | Counter | _(none)_ | Dropped worker responses |

**Controller metrics:**

| Metric | Type | Labels | Description |
|--------|------|--------|-------------|
| `rotom_controller_connections` | Gauge | `user_agent` | Active controller connections by user agent |

**RPC metrics:**

| Metric | Type | Labels | Description |
|--------|------|--------|-------------|
| `rotom_rpc_requests_total` | Counter | _(none)_ | Total RPC requests |
| `rotom_rpc_request_duration_seconds` | Histogram | _(none)_ | RPC request latency |

**Application lifecycle metrics:**

| Metric | Type | Labels | Description |
|--------|------|--------|-------------|
| `rotom_app_startups_total` | Counter | `version` | Application startups |
| `rotom_config_reloads_total` | Counter | `version` | Successful config reloads |

**Go runtime metrics:**

| Metric | Description |
|--------|-------------|
| `rotom_go_*` | Go runtime statistics (goroutines, GC, memory) |
| `rotom_process_*` | Process metrics (CPU, memory, file descriptors) |

These replace the Node.js default metrics from OG.

### Metrics Configuration

OG exposed metrics unconditionally at `/metrics`. NG requires explicit enablement:

```toml
[prometheus]
enable = true

[tuning]
# Enable to provide a slight speedup at the cost of losing request duration stats.
disable_worker_stats = false
```

---

## UI Changes

NG retains the same core layout (dark theme, top navigation, data tables with search/sort/pagination) but adds several new pages, features, and visual improvements.

### New Pages

OG had two pages: **Status** and **Jobs**. NG adds three more:

| Page | Route | Description |
|------|-------|-------------|
| Status | `/` | Redesigned dashboard with aggregated metrics |
| **Devices** | `/devices` | Dedicated device management page (new) |
| **Controllers** | `/controllers` | Controller monitoring and management (new) |
| **Workers** | `/workers` | Per-worker monitoring with performance stats (new) |
| Jobs | `/jobs` | Enhanced job management |

In OG, devices and workers were shown as tables on the single Status page. In NG, each has its own dedicated page with richer detail.

### Status Page

OG displayed four summary cards (Total Devices, Total Workers, Workers in Use, Available Workers) followed by inline device and worker tables.

NG shows a cleaner dashboard with three metric groups:

- **Controllers** - Total count
- **Devices** - In Use, Connected, Enabled, Total
- **Workers** - In Use, Available, Enabled, Total, plus time-windowed request rate and latency stats (Req/s and Avg Req Ms at 1m, 5m, 15m intervals) when available

The device and worker tables are no longer on this page; they have moved to their own pages.

### Devices Page

The Devices page in NG adds several features over the OG status page device table:

**New columns:**
- **Connected** - visual icon (replaces the `isAlive` emoji)
- **In Use** - whether the device is actively in use
- **Workers** - shows `{in_use}/{total} ({percent}%)` per device
- **Weight** - load distribution weight with percentage
- **Enabled** - interactive checkbox to toggle device enabled/disabled state directly from the table

**Expandable rows:** Clicking a row expand button reveals:
- Memory statistics (Free, MITM, Start)
- Total statistics (Last Connected, Messages/Bytes sent and received)
- Session statistics (if an active session exists)

In OG, memory was shown as flat columns in the main table. In NG, it is tucked into the expandable detail view along with additional stats not available in OG.

**Row color coding:**
- Red background: device is disconnected
- Orange background: device is connected but disabled

**New device actions:**
- Disable / Enable (toggle availability for selection)
- Disconnect (force disconnect a live device)

**Confirmation dialogs:** Destructive actions (reboot, restart, disconnect) now require confirmation before executing. OG executed actions immediately on click.

### Controllers Page (New)

OG had no controller management UI. NG provides a full controllers page with:

- Controller metrics grid (total count)
- Sortable, searchable table with columns: Controller ID, User Agent, Weight, Worker ID, Connected At, Last Seen
- Expandable rows showing UUID, protocol version, account info (username, source), and session statistics
- Actions: Reconnect, Disconnect (with confirmation dialogs)

### Workers Page (New)

OG showed a basic workers table on the Status page with 6 columns. NG provides a dedicated Workers page with:

**Additional columns over OG:**
- Enabled status
- Weight
- Controller ID (assigned controller)
- Version
- Connected At
- Error count (with tooltip showing last error timestamp)

**Expandable rows** showing:
- Worker details (User Agent, Version Code, Version Name, Device ID)
- Session statistics
- Time-windowed statistics (request rates and average duration at 30s, 1m, 5m, 15m intervals)

**Row color coding** (same as devices): red for disconnected, orange for connected-but-disabled.

**Debounced search:** The workers search uses 300ms debounce for performance on large worker lists. OG search was immediate.

### Jobs Page

**New features over OG:**
- **Reload button** in the jobs list header to reload job definitions from disk without restarting
- **Job instance status uses color-coded text:** green for success, red for failed, orange for started (OG used emoji checkmarks)
- **Started At / Finished At timestamps** replace the simple "Is Over" boolean from OG
- **Clear button** to remove all job instance records
- **Connected-only filtering:** The execute job modal only shows connected devices (OG showed all devices)

### General UI Improvements

| Feature | OG | NG |
|---------|-----|-----|
| UI framework | NextUI | Radix UI + Tailwind CSS (Shadcn/ui) |
| Navigation (mobile) | No mobile support | Hamburger drawer menu |
| Table pagination | Fixed rows-per-page | Persistent rows-per-page preference (saved to localStorage per table) |
| Sorting | Basic string sort | Alphanumeric natural sort (e.g., "device2" before "device10") |
| Destructive actions | Execute immediately | Confirmation dialog required |
| Action feedback | Toast notifications | Toast notifications (unchanged) |
| Table sections | Always visible | Collapsible/expandable card sections |
| Row details | Flat columns only | Expandable rows with drill-down detail |
| Data refresh | 5-second polling | 5-second polling (unchanged) |
