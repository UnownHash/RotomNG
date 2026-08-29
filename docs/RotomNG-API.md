## HTTP API Service

The HTTP API provides REST endpoints for managing devices, controllers, jobs, and system configuration.

### Base URL
```
http://localhost:7072/api
```

### Authentication

Rotom supports optional header-based authentication for the API.

#### Configuration

Authentication is configured in the TOML configuration file using the `secret` parameter:

```toml
# HTTP API authentication (optional)
[http_listener]
address = ":7072"
secret = "your-api-secret-here"  # Optional authentication secret
```

#### HTTP API Authentication

The HTTP API supports header-based authentication when configured. When a `secret` is configured in `http_listener.secret`, all API endpoints will require authentication.

**Required Headers** (when authentication is enabled):
- `X-Rotom-Secret: your-secret-here`

**Authentication Behavior**:
- If no `secret` is configured: No authentication required
- If `secret` is configured: Authentication header is required for all API requests
- Missing or invalid header returns `401 Unauthorized`

**Authentication Responses**:
- `401 Unauthorized`: Missing or invalid authentication header when authentication is required

A request is accepted if it carries **any one** of the following credentials:

| Credential | Intended for |
| --- | --- |
| `X-Rotom-Secret: <secret>` | Machine clients (Dragonite, scripts, Prometheus) |
| `Authorization: Bearer <token>` | Clients that prefer a short-lived token to a static secret |
| Session cookie + `X-Rotom-Session: 1` | The web UI |

#### Session Endpoints

These three endpoints are **not** gated by the auth middleware — they are how a
browser obtains a credential in the first place.

```http
GET  /api/auth/me
POST /api/auth/login
POST /api/auth/logout
```

`GET /api/auth/me` reports what the caller needs to do:

```json
{ "status": "ok", "auth_required": true, "authenticated": false }
```

`POST /api/auth/login` exchanges the configured secret for a session:

```http
POST /api/auth/login
Content-Type: application/json

{ "secret": "your-api-secret-here" }
```

On success the response sets an `HttpOnly`, `SameSite=Strict` cookie holding a
signed HS256 token. The cookie is also flagged `Secure` when the request
arrives over TLS (directly, or via a proxy sending `X-Forwarded-Proto: https`).
A wrong secret returns `401`; if no secret is configured at all, login returns
`400`, since there is no session to create.

Notes on session tokens:

- **Rotation is revocation.** The token signing key is derived from the secret,
  so changing `http_listener.secret` — including via config reload —
  invalidates every outstanding session immediately. There is no other
  revocation mechanism; tokens are stateless.
- **Session lifetime** defaults to one day and is set by
  `http_listener.ui_session_ttl`:

  ```toml
  [http_listener]
  secret = "your-api-secret-here"
  ui_session_ttl = "12h"   # default "24h"
  ```

  Go duration syntax has no day unit, so write `"24h"` rather than `"1d"` —
  the latter fails to parse at startup. The value applies to sessions minted
  after it takes effect: a token's expiry is signed into its claims, so
  shortening the TTL does not retroactively end sessions already issued.
  Rotating the secret is what does that.
- **The `X-Rotom-Session` header is required** on cookie-authenticated
  requests. The cookie alone is deliberately not sufficient: requiring a custom
  header means a cross-site form post cannot ride a logged-in operator's
  session. Requests authenticated by `X-Rotom-Secret` or `Authorization` do not
  need it.
- **Bearer tokens** are the same tokens the cookie carries, so a client can
  call `/api/auth/login` and use the returned cookie value as a bearer token if
  it would rather not hold the long-lived secret.
- **Login is not rate limited.** It is an unauthenticated endpoint, so use a
  high-entropy secret, and put the listener behind a proxy that throttles if it
  is exposed to untrusted networks. Failed attempts are logged at `WARN` with
  the client IP.

#### Web UI

The UI signs in through these endpoints. When a secret is configured it shows a
sign-in form; when one is not, it loads straight through as before. Because the
token lives in an `HttpOnly` cookie, the secret is never stored anywhere page
JavaScript can read it.

Operators fronting Rotom with a reverse proxy can skip the UI login entirely by
having the proxy inject `X-Rotom-Secret` on `/api`, and handle authentication
themselves.

### Configuration Endpoints

#### Get Configuration
```http
GET /api/config
```

**Headers** (when authentication is enabled):
```http
X-Rotom-Secret: your-secret-here
```

Returns the current system configuration including version, tuning parameters, and rate limits.

**Response:**
```json
{
  "status": "ok",
  "config": {
    "version": "1.0.0",
    "sha": "abc1234",
    "instance": "optional-instance-name",
    "tuning": {
      "profiling": false
    },
    "rate_limit": {
      "enable": true,
      "max_selections": 100,
      "duration": "1m"
    },
    "device_listener": {
      "ping_interval": "30s",
      "pong_wait": "30s"
    },
    "controller_listener": {
      "ping_interval": "30s",
      "pong_wait": "30s",
      "registration_timeout": "1m0s",
      "data_timeout": "2m0s"
    }
  }
}
```

**Notes:**
- `sha`: Git commit SHA of the running build
- `instance`: Only present if configured
- `rate_limit`: Always present
- `device_listener` / `controller_listener`: websocket keep-alive settings. `ping_interval` and `pong_wait` are durations; the read timeout fires if no pong or data message is received within `ping_interval + pong_wait`. The `device_listener` settings also govern MITM worker connections, which connect on the device listener.
- `registration_timeout`: max time allowed for a controller to complete registration before the normal ping-based read timeout applies.
- `data_timeout`: controller connections only. The connection is considered dead if no data message is received within this period, independent of ping/pong keep-alive activity. Defaults to `"2m"`; set to `"0s"` to disable.

#### Reload Configuration
```http
PUT /api/config/reload
```

**Headers** (when authentication is enabled):
```http
X-Rotom-Secret: your-secret-here
```

Reloads the configuration from the config file and returns the updated configuration. Note: This does not reload jobs. Use the dedicated job reload endpoint to reload jobs.

**Response:** Same as GET /api/config

### Status Endpoints

#### Get System Status
```http
GET /api/status
```

**Headers** (when authentication is enabled):
```http
X-Rotom-Secret: your-secret-here
```

Returns comprehensive status information including all devices and controllers, plus aggregate request statistics across all workers (`global_stats`).

**Response:**
```json
{
  "devices": [
    {
      "id": "device-123",
      "origin": "192.168.1.100",
      "version": "1.0",
      "public_ip": "203.0.113.1",
      "worker_count": 2,
      "worker_in_use_count": 1,
      "worker_in_use_percent": 50.0,
      "worker_in_use_weight": 5,
      "worker_max_weight": 10,
      "worker_in_use_weight_percent": 50.0,
      "last_connected_at_ms": 1694123456789,
      "last_seen_at_ms": 1694123456789,
      "enabled": true,
      "is_connected": true,
      "can_be_used": true,
      "is_in_use": true,
      "last_memory": {
        "free": 1024000,
        "mitm": 512000,
        "start": 256000
      },
      "session": {
        "connected_at_ms": 1694123456789,
        "message_last_received_at_ms": 1694123456789,
        "messages_received": 100,
        "bytes_received": 50000,
        "message_last_sent_at_ms": 1694123456789,
        "messages_sent": 95,
        "bytes_sent": 48000
      },
      "workers": [
        {
          "id": "worker-456",
          "device_id": "device-123",
          "origin": "192.168.1.100",
          "version_code": 12345,
          "version_name": "1.2.3",
          "user_agent": "PokemodApp/1.2.3",
          "last_connected_at_ms": 1694123456789,
          "last_seen_at_ms": 1694123456789,
          "message_last_received_at_ms": 1694123456789,
          "messages_received": 100,
          "bytes_received": 50000,
          "message_last_sent_at_ms": 1694123456789,
          "messages_sent": 95,
          "bytes_sent": 48000,
          "is_connected": true,
          "is_in_use": true,
          "platform": "android",
          "weight": 5,
          "can_be_used": true,
          "session": {
            "connected_at_ms": 1694123456789,
            "message_last_received_at_ms": 1694123456789,
            "messages_received": 80,
            "bytes_received": 40000,
            "message_last_sent_at_ms": 1694123456789,
            "messages_sent": 80,
            "bytes_sent": 40000,
            "controller": {
              "id": "controller-789",
              "uuid": "550e8400-e29b-41d4-a716-446655440000",
              "user_agent": "Controller/1.0",
              "weight": 5,
              "proto_major_version": 1,
              "proto_minor_version": 0,
              "account_username": "player123",
              "account_source": "ptc",
              "connected_at_ms": 1694123456789,
              "message_last_received_at_ms": 1694123456789,
              "messages_received": 50,
              "bytes_received": 25000,
              "message_last_sent_at_ms": 1694123456789,
              "messages_sent": 48,
              "bytes_sent": 24000
            }
          },
          "time_windowed_stats": {
            "requests_rate_over_30_seconds": 2.5,
            "requests_rate_over_1_min": 2.3,
            "requests_rate_over_5_min": 2.1,
            "requests_rate_over_15_min": 2.0,
            "request_ms_avg_over_30_seconds": 150.5,
            "request_ms_avg_over_1_min": 155.2,
            "request_ms_avg_over_5_min": 160.1,
            "request_ms_avg_over_15_min": 165.0
          }
        }
      ]
    }
  ],
  "controllers": [
    {
      "id": "controller-789",
      "uuid": "550e8400-e29b-41d4-a716-446655440000",
      "user_agent": "Controller/1.0",
      "weight": 5,
      "proto_major_version": 1,
      "proto_minor_version": 0,
      "account_username": "player123",
      "account_source": "ptc",
      "connected_at_ms": 1694123456789,
      "message_last_received_at_ms": 1694123456789,
      "messages_received": 50,
      "bytes_received": 25000,
      "message_last_sent_at_ms": 1694123456789,
      "messages_sent": 48,
      "bytes_sent": 24000
    }
  ],
  "global_stats": {
    "requests_rate_over_30_seconds": 12.5,
    "requests_rate_over_1_min": 11.8,
    "requests_rate_over_5_min": 10.4,
    "requests_rate_over_15_min": 9.7,
    "request_ms_avg_over_30_seconds": 152.3,
    "request_ms_avg_over_1_min": 158.0,
    "request_ms_avg_over_5_min": 161.6,
    "request_ms_avg_over_15_min": 164.2
  }
}
```

`global_stats` holds aggregate request statistics across **all** workers, including
those that have since disconnected. Unlike summing the per-worker
`time_windowed_stats`, these totals stay accurate as workers connect and
disconnect, making it the correct source for overall req/s and average request
duration. The field is omitted when no global stats are available. See
[Time Windowed Statistics](#time-windowed-statistics) for the field definitions.

### Controller Endpoints

#### Get Controllers
```http
GET /api/controller
```

**Headers** (when authentication is enabled):
```http
X-Rotom-Secret: your-secret-here
```

Returns a list of all connected controllers.

**Response:**
```json
{
  "controllers": [
    {
      "id": "controller-789",
      "uuid": "550e8400-e29b-41d4-a716-446655440000",
      "user_agent": "Controller/1.0",
      "weight": 5,
      "proto_major_version": 1,
      "proto_minor_version": 0,
      "worker_id": "optional-worker-id",
      "account_username": "player123",
      "account_source": "ptc",
      "connected_at_ms": 1694123456789,
      "message_last_received_at_ms": 1694123456789,
      "messages_received": 50,
      "bytes_received": 25000,
      "message_last_sent_at_ms": 1694123456789,
      "messages_sent": 48,
      "bytes_sent": 24000
    }
  ]
}
```

#### Get Controller
```http
GET /api/controller/{uuid}
```

**Headers** (when authentication is enabled):
```http
X-Rotom-Secret: your-secret-here
```

Returns details for a specific controller.

**Path Parameters:**
- `uuid` (string): Controller UUID

**Response:**
```json
{
  "controller": {
    "id": "controller-789",
    "uuid": "550e8400-e29b-41d4-a716-446655440000",
    "user_agent": "Controller/1.0",
    "weight": 5,
    "proto_major_version": 1,
    "proto_minor_version": 0,
    "worker_id": "optional-worker-id",
    "account_username": "player123",
    "account_source": "ptc",
    "connected_at_ms": 1694123456789,
    "message_last_received_at_ms": 1694123456789,
    "messages_received": 50,
    "bytes_received": 25000,
    "message_last_sent_at_ms": 1694123456789,
    "messages_sent": 48,
    "bytes_sent": 24000
  }
}
```

**Error Responses:**
- `404`: Controller not found

#### Controller Actions
```http
PUT /api/controller/{uuid}/action/{action}
```

**Headers** (when authentication is enabled):
```http
X-Rotom-Secret: your-secret-here
```

Performs an action on a specific controller.

**Path Parameters:**
- `uuid` (string): Controller UUID
- `action` (string): Action to perform

**Available Actions:**
- `disconnect`: Disconnect controller (single controller only)
- `reconnect`: Tell controller to reconnect (single controller only)

**Response for disconnect:**
```json
{
  "status": "ok",
  "message": "Controller disconnected."
}
```

**Response for reconnect:**
```json
{
  "status": "ok",
  "message": "Controller set to be told to reconnect/restart session."
}
```

**Error Responses:**
- `404`: Controller not found or action not found
- `500`: Internal server error

### Device Endpoints

#### Get Devices
```http
GET /api/device
```

**Headers** (when authentication is enabled):
```http
X-Rotom-Secret: your-secret-here
```

Returns a list of all devices.

**Query Parameters:**
- `include_workers` (boolean): Include worker details in response (default: false)

**Response:**
```json
{
  "devices": [
    {
      "id": "device-123",
      "origin": "192.168.1.100",
      "version": "1.0",
      "public_ip": "203.0.113.1",
      "worker_count": 2,
      "worker_in_use_count": 1,
      "worker_in_use_percent": 50.0,
      "worker_in_use_weight": 5,
      "worker_max_weight": 10,
      "worker_in_use_weight_percent": 50.0,
      "last_connected_at_ms": 1694123456789,
      "last_seen_at_ms": 1694123456789,
      "enabled": true,
      "is_connected": true,
      "can_be_used": true,
      "is_in_use": true,
      "last_memory": {
        "free": 1024000,
        "mitm": 512000,
        "start": 256000
      },
      "session": {
        "connected_at_ms": 1694123456789,
        "message_last_received_at_ms": 1694123456789,
        "messages_received": 100,
        "bytes_received": 50000,
        "message_last_sent_at_ms": 1694123456789,
        "messages_sent": 95,
        "bytes_sent": 48000
      },
      "workers": []
    }
  ]
}
```

#### Get Device
```http
GET /api/device/{deviceId}
```

**Headers** (when authentication is enabled):
```http
X-Rotom-Secret: your-secret-here
```

Returns details for a specific device.

**Path Parameters:**
- `deviceId` (string): Device identifier

**Query Parameters:**
- `include_workers` (boolean): Include worker details in response (default: false)

**Response:** Same structure as single device in GET /api/device

**Error Responses:**
- `404`: Device not found

#### Device Actions
```http
PUT /api/device/{deviceId}/action/{action}
```

**Headers** (when authentication is enabled):
```http
X-Rotom-Secret: your-secret-here
```

Performs an action on a specific device.

**Path Parameters:**
- `deviceId` (string): Device identifier or "_" for bulk operations (limited actions)
- `action` (string): Action to perform

**Available Actions:**
- `delete`: Remove device (supports "_" for removing all unconnected devices)
- `restart`: Restart device app (single device only)
- `reboot`: Reboot device (single device only)
- `logcat`: Download device logs as ZIP file (single device only)
- `disable`: Disable device for selection (single device only)
- `enable`: Enable device for selection (single device only)
- `disconnect`: Disconnect device and all its workers (single device only)

**Response for most actions:**
```json
{
  "status": "ok",
  "message": "Action completed successfully",
  "device": {
    // Updated device object (for enable/disable actions)
  }
}
```

**Response for delete with "_":**
```json
{
  "status": "ok",
  "message": "Removed dead connections",
  "devices_count": 2
}
```

**Response for logcat:**
Binary ZIP file download with headers:
- `Content-Disposition`: attachment; filename="logcat-{deviceId}-{timestamp}.zip"
- `Content-Type`: application/zip

**Response for disconnect:**
```json
{
  "status": "ok",
  "message": "Device disconnected."
}
```

**Important Notes:**
- **Device disconnect behavior**: When a device is disconnected using the `disconnect` action, all workers associated with that device are automatically disconnected as well. This ensures that no orphaned worker connections remain when a device goes offline.

**Error Responses:**
- `400`: Invalid action or device state
- `404`: Device not found
- `500`: Internal server error

### Job Management Endpoints

#### Get Jobs
```http
GET /api/job
```

**Headers** (when authentication is enabled):
```http
X-Rotom-Secret: your-secret-here
```

Returns a list of all available jobs.

**Response:**
```json
{
  "jobs": [
    {
      "id": "job-1",
      "description": "Example job description",
      "exec": "command to execute"
    }
  ]
}
```

**Error Responses:**
- `404`: Jobs are not enabled

#### Get Job
```http
GET /api/job/{jobId}
```

**Headers** (when authentication is enabled):
```http
X-Rotom-Secret: your-secret-here
```

Returns details for a specific job.

**Path Parameters:**
- `jobId` (string): Job identifier

**Response:**
```json
{
  "job": {
    "id": "job-1",
    "description": "Example job description",
    "exec": "command to execute"
  }
}
```

**Error Responses:**
- `404`: Job not found or jobs not enabled

#### Reload Jobs
```http
PUT /api/job/-/reload
```

**Headers** (when authentication is enabled):
```http
X-Rotom-Secret: your-secret-here
```

Reloads all jobs from the configured jobs directory without requiring a full configuration reload.

**Response:**
```json
{
  "status": "ok",
  "message": "Jobs reloaded successfully"
}
```

**Error Responses:**
- `404`: Jobs are not enabled
- `500`: Failed to reload jobs (error details in response)

#### Run Job
```http
PUT /api/job/{jobId}/run
```

**Headers** (when authentication is enabled):
```http
X-Rotom-Secret: your-secret-here
```

Executes a job on specified devices.

**Path Parameters:**
- `jobId` (string): Job identifier

**Request Body:**
```json
{
  "device_ids": ["device-1", "device-2"]
}
```

**Response:**
```json
{
  "instances": [
    {
      "id": 12345,
      "job_id": "job-1",
      "started_at_ms": 1694123456789,
      "finished_at_ms": 0,
      "device_id": "device-1",
      "device_origin": "192.168.1.100",
      "result": "",
      "status": "running"
    }
  ]
}
```

**Error Responses:**
- `400`: Invalid request body or no device IDs provided
- `404`: Job not found or jobs not enabled

### Job Instance Endpoints

#### Get Job Instances
```http
GET /api/job-instance
```

**Headers** (when authentication is enabled):
```http
X-Rotom-Secret: your-secret-here
```

Returns a list of all job instances (sorted by ID, newest first).

**Response:**
```json
{
  "instances": [
    {
      "id": 12345,
      "job_id": "job-1",
      "started_at_ms": 1694123456789,
      "finished_at_ms": 1694123456890,
      "device_id": "device-1",
      "device_origin": "192.168.1.100",
      "result": "Job completed successfully",
      "status": "completed"
    }
  ]
}
```

**Error Responses:**
- `404`: Jobs are not enabled

#### Get Job Instance
```http
GET /api/job-instance/{jobInstanceId}
```

**Headers** (when authentication is enabled):
```http
X-Rotom-Secret: your-secret-here
```

Returns details for a specific job instance.

**Path Parameters:**
- `jobInstanceId` (uint64): Job instance identifier

**Response:**
```json
{
  "job": {
    "id": 12345,
    "job_id": "job-1",
    "started_at_ms": 1694123456789,
    "finished_at_ms": 1694123456890,
    "device_id": "device-1",
    "device_origin": "192.168.1.100",
    "result": "Job completed successfully",
    "status": "completed"
  }
}
```

**Error Responses:**
- `404`: Job instance not found or jobs not enabled

#### Clear Job Instance
```http
PUT /api/job-instance/{jobInstanceId}/clear
```

**Headers** (when authentication is enabled):
```http
X-Rotom-Secret: your-secret-here
```

Clears a specific job instance or all instances.

**Path Parameters:**
- `jobInstanceId` (string): Job instance identifier or "-" to clear all instances

**Response:**
```json
{
  "status": "success"
}
```

**Error Responses:**
- `404`: Job instance not found or jobs not enabled

### Monitoring Endpoints

#### Get Metrics
```http
GET /api/metrics
```

**Headers** (when authentication is enabled):
```http
X-Rotom-Secret: your-secret-here
```

Returns Prometheus metrics (only available if Prometheus is enabled in configuration).

**Response:** Prometheus metrics format

### Debug Endpoints

All debug endpoints require profiling to be enabled in the configuration.

**Headers** (when authentication is enabled):
```http
X-Rotom-Secret: your-secret-here
```

#### Profiling Index
```http
GET /api/debug/pprof/
```

Returns pprof profiling index page.

#### Command Line
```http
GET /api/debug/pprof/cmdline
```

Returns command line arguments.

#### CPU Profile
```http
GET /api/debug/pprof/profile
```

Returns CPU profile data.

#### Symbol Table
```http
GET /api/debug/pprof/symbol
```

Returns symbol table information.

#### Execution Trace
```http
GET /api/debug/pprof/trace
```

Returns execution trace data.

**Error Response for all debug endpoints when profiling is disabled:**
```json
{
  "error": "profiling disabled"
}
```

## Data Models

### Common Statistics
All connection objects include these common statistics:
```json
{
  "message_last_received_at_ms": 1694123456789,
  "messages_received": 100,
  "bytes_received": 50000,
  "message_last_sent_at_ms": 1694123456789,
  "messages_sent": 95,
  "bytes_sent": 48000
}
```

### Device Memory
```json
{
  "free": 1024000,
  "mitm": 512000,
  "start": 256000
}
```

### Time Windowed Statistics
Available for workers in `requests` mode or `proxy` mode with `inspect` proxy mode:
```json
{
  "requests_rate_over_30_seconds": 2.5,
  "requests_rate_over_1_min": 2.3,
  "requests_rate_over_5_min": 2.1,
  "requests_rate_over_15_min": 2.0,
  "request_ms_avg_over_30_seconds": 150.5,
  "request_ms_avg_over_1_min": 155.2,
  "request_ms_avg_over_5_min": 160.1,
  "request_ms_avg_over_15_min": 165.0
}
```

**Fields:**
- `requests_rate_over_*`: Request rate per second over the specified time window
- `request_ms_avg_over_*`: Average request duration in milliseconds over the specified time window

### Worker Fields
Additional worker-specific fields:
- `platform` (string): Device platform (e.g., "android", "ios")
- `weight` (int): Weight of the assigned controller — only present when worker is in use
- `time_windowed_stats` (object): Time-windowed statistics (only present for certain worker modes)

### Job Status Values
- `pending`: Job is queued but not started
- `running`: Job is currently executing
- `completed`: Job finished successfully
- `failed`: Job failed with error
- `cancelled`: Job was cancelled

## Error Handling

All API endpoints return consistent error responses:

```json
{
  "status": "error",
  "error": "Error description"
}
```

Common HTTP status codes:
- `200`: Success
- `400`: Bad Request (invalid parameters, device state issues)
- `404`: Not Found (resource doesn't exist, feature disabled)
- `500`: Internal Server Error

## Rate Limiting

Rate limiting can be configured per device and applies to worker selection. When enabled, it limits the number of worker selections per time duration.

## Configuration

The system supports runtime configuration reloading via the `/api/config/reload` endpoint. Configuration changes take effect immediately without requiring a restart.
