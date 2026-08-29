# Configuration

Configuration for RotomNG lives in [configs](../configs).

There is an example config file that should be copied to rotom-ng.toml and
edited to your liking. All sections are optional and have sensible defaults.

If you are migrating from OG Rotom, there is a conversion script at
[configs/rotom-og-to-ng.py](../configs/rotom-og-to-ng.py) that will convert
your existing config to the new format. Run it with two arguments, your old
config file followed by `rotom-ng.toml`, to write the converted config directly
to `rotom-ng.toml` (for example, `python3 configs/rotom-og-to-ng.py old-config.json rotom-ng.toml`).

# Building and starting

## Docker

### Image tags

RotomNG images are published to `ghcr.io/unownhash/rotomng/rotom-ng`.
Available tags:

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

If you prefer to build the image yourself instead of pulling from GHCR:

```
$ docker build -f apps/rotom-ng/Dockerfile -t rotom-ng .
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
the Go binary.

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
