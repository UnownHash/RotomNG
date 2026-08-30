# RotomNG UI server (multi-instance admin)

> **This is optional, and most deployments do not need it.** A single
> `rotom-ng` already serves its own web UI on its `http_listener` (`:7072` by
> default). If that is your setup, there is nothing here for you.

`rotom-ng-ui` is a second, separate binary for operators running **several
rotom-ng instances**. It serves the same web UI for all of them at once, with a
menu in the header to switch between them, so you get one page instead of a
browser tab per instance.

It manages no devices, workers, or controllers of its own. It serves the UI and
proxies every API call to whichever instance is selected; the instances
themselves are unchanged and unaware of it, and keep working exactly as they do
today whether or not you run this.

It embeds the same UI bundle `rotom-ng` does — there is one UI, not two. It
decides at runtime which kind of server answered, so adding this service changes
nothing about a single-instance deployment.

## Do you want this?

| | |
| --- | --- |
| One rotom-ng | No. Use its own UI on `:7072`. |
| Several, and you are happy with a tab each | No. Nothing breaks either way. |
| Several, and you want one page for all of them | Yes — read on. |

## Configuration

Copy [configs/rotom-ng-ui.toml.example](../configs/rotom-ng-ui.toml.example) to
`rotom-ng-ui.toml` and edit it. The minimum is one instance:

```toml
[http_listener]
address = ":7073"

[[instances]]
base_url = "http://10.0.0.10:7072"
api_secret = "that-instances-api-secret"

[[instances]]
base_url = "http://10.0.0.11:7072"
```

`[http_listener]` is the same section rotom-ng has, with the same `address`,
`secret`, and `ui_session_ttl` keys, and the same behaviour — see
[the API reference](RotomNG-API.md#authentication).

### Instances

Each `[[instances]]` block names one rotom-ng:

- `base_url` — the root of that instance's `http_listener`, **without** the
  `/api` suffix. `/api` is appended when requests are proxied. This is also how
  the UI identifies an instance, so each one must be distinct; the service
  refuses to start on a duplicate.
- `api_secret` — that instance's own `http_listener.secret`. Omit it when the
  instance has no secret configured.

Two independent secrets are in play, and it is worth keeping them straight:

| Secret | Guards | Sent by |
| --- | --- | --- |
| `http_listener.secret` | the admin UI itself | the operator's browser, or an API client |
| `instances[].api_secret` | one rotom-ng | this service, on every proxied request |

The admin service's own credentials — its session cookie and any bearer token —
are stripped from proxied requests and never reach an instance.

### Instance monitoring

Each instance's `/api/config` is polled on an interval (default 10s, see
`[instance_monitor]`). An instance is **reachable** only once that call has
succeeded. The UI will not let an operator switch to an unreachable instance,
and says so plainly if the selected one goes down.

The reply to that poll is also where the service learns each instance's name
and configuration, which is what lets the UI's per-instance features — the Jobs
tab, worker stats — follow the instance the operator selected. An instance that
does not set `instance` in its own config is listed in the picker by URL.

## `GET /api/config`

Served on the same path rotom-ng serves its own, and carrying the same fields
that make sense for a service with no connections of its own, plus `instances`:

```json
{
  "status": "ok",
  "config": {
    "version": "1.0.1beta1",
    "sha": "6e501e2",
    "instance": "admin",
    "instances": [
      {
        "instance": "east",
        "url": "http://10.0.0.10:7072",
        "reachable": true,
        "config": { "version": "1.0.1beta1", "jobs": { "enable": true }, "...": "..." }
      },
      {
        "instance": "west",
        "url": "http://10.0.0.11:7072",
        "reachable": false,
        "config": { "...": "..." }
      },
      { "instance": "", "url": "http://10.0.0.12:7072", "reachable": false }
    ]
  }
}
```

- `instances` is **always present** here, as an empty list when none are
  configured, and is **never** present in a rotom-ng reply. That is exactly how
  the UI tells the two apart.
- `config` on an entry is that instance's own `/api/config` config object,
  passed through verbatim. It is absent until the instance has been reached
  once, and is retained while an instance is unreachable so the UI does not
  reshape itself every time one restarts.

## Every other endpoint

Everything else under `/api` is proxied to the selected instance, unchanged —
see [the API reference](RotomNG-API.md) for what that is. Nothing is
allowlisted, so an endpoint added to rotom-ng works here without a change.

Select an instance with the `X-Rotom-Instance` header, set to its `base_url`
(an instance name works too, but names can repeat and base URLs cannot):

```
$ curl -H 'X-Rotom-Instance: http://10.0.0.10:7072' \
       -H 'X-Rotom-Secret: your-admin-secret' \
       http://localhost:7073/api/status
```

Omit the header and the first reachable instance answers, which is convenient
for scripts that do not care which one they get.

Failures are distinguished, since they mean different things:

| Status | Meaning |
| --- | --- |
| `404` | the header named an instance that is not configured |
| `502` | the instance was reached for but did not answer |
| `503` | no instances are configured, or none are reachable and none was named |

## Building and running

```
$ make rotom-ng-ui       # builds the UI, then the binary
$ ./rotom-ng-ui          # reads configs/rotom-ng-ui.toml by default
$ ./rotom-ng-ui /path/to/rotom-ng-ui.toml
```

`make` on its own builds both binaries.

Or with Docker, using the published image:

```
$ docker pull ghcr.io/unownhash/rotomng/rotom-ng-ui:latest
```

It is tagged alongside `rotom-ng` from the same commit, so matching tags mean
matching versions. To build it yourself instead:

```
$ make docker-ui
```

## Reloading

`SIGHUP`, or `PUT /api/config/reload`, re-reads the config file. Instances
added, removed, or given a new secret are picked up without a restart;
instances that survive the reload keep their reachability and cached config,
so a reload does not blank the UI for the ones it left alone.
