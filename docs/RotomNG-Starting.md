# Configuration

Configuration for RotomNG lives in [configs](../configs).

There is an example config file that should be copied to rotom-ng.toml and
edited to your liking. All sections are optional and have sensible defaults.
That one file is all a rotom-ng needs, its web UI included.

(There is a second example, `rotom-ng-ui.toml.example`, for an optional extra
service that fronts several rotom-ng instances with a single UI. Ignore it
unless you run more than one — see
[Multi-instance admin UI](RotomNG-UI-Server.md).)

If you are migrating from OG Rotom, there is a conversion script at
[configs/rotom-og-to-ng.py](../configs/rotom-og-to-ng.py) that will convert
your existing config to the new format. Run it with two arguments, your old
config file followed by `rotom-ng.toml`, to write the converted config directly
to `rotom-ng.toml` (for example, `python3 configs/rotom-og-to-ng.py old-config.json rotom-ng.toml`).

# Building and starting

## Docker

### Image tags

Two images are published, both under `ghcr.io/unownhash/rotomng/`:

- `rotom-ng` — the connection manager. This is the one you want.
- `rotom-ng-ui` — the optional multi-instance admin UI, which fronts several
  `rotom-ng` instances and proxies to them. Skip it unless you run more than
  one; see [Multi-instance admin UI](RotomNG-UI-Server.md).

Both are built and tagged together from the same commit, so a given tag means
the same version of both. Available tags:

- `main` — built from every commit on the `main` branch, so it can be ahead of
  the most recent release.
- `latest` — the most recent stable release. Prereleases never move this tag,
  so use this instead of `main` if you only want released versions. This is the
  recommended tag for most users.
- `vX.Y.Z` (e.g. `v1.0.0`) — a specific stable release version. Pin to a
  version tag for reproducible deployments. The unprefixed form (`1.0.0`) is
  published too and refers to the same image.
- `testing` — built from every commit on the `testing` branch, so it too can be
  ahead of the most recent prerelease. May contain pre-release or in-progress
  changes; intended for testing only.
- `vX.Y.ZbetaN` / `vX.Y.ZalphaN` (e.g. `v1.0.1beta1`) — a specific pre-release
  version.

### Running with Docker Compose

Rename the docker compose example file to docker-compose.yml, edit it,
and:

```
$ docker compose pull
$ docker compose up -d
```

### Building locally

If you prefer to build the images yourself instead of pulling from GHCR:

```
$ make docker      # or: docker build --target rotom-ng -t rotom-ng .
$ make docker-ui   # the admin UI image, only if you need it
```

## Non-docker (local install)

### Requirements

1. You will need to have at least golang 1.26.3 installed. You may need to install it manually. See the [instructions and download links](https://go.dev/dl/).
2. Ensure node v20.8 or higher is installed (v24+ recommended).
3. Install [Bun](https://bun.sh/) v1.3 or higher. Bun is used to build the frontend UI.

### Building

From the RotomNG root directory:

```
$ make
```

This will install frontend dependencies via Bun, build the UI, and compile
both Go binaries: `rotom-ng`, and the `rotom-ng-ui` admin server described in
[Multi-instance admin UI](RotomNG-UI-Server.md). `make rotom-ng` builds just
the first, `make rotom-ng-ui` just the second.

### Starting with pm2

```
$ pm2 start ./rotom-ng --name rotom-ng
```

### Starting directly

```
$ ./rotom-ng
```

You can also specify a config file path:

```
$ ./rotom-ng /path/to/rotom-ng.toml
```

### Running several instances behind one UI

If you run more than one rotom-ng, `rotom-ng-ui` serves a single web UI that
switches between them. See [Multi-instance admin UI](RotomNG-UI-Server.md).
