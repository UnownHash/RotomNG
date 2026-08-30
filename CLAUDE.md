# CLAUDE.md

## Project Overview

RotomNG is a distributed MITM proxy connection manager. Monorepo with a Go backend (`apps/rotom-ng/`) and React/TypeScript frontend (`apps/rotom-ng-ui/`), sharing libraries in `libs/`.

A second Go service, `apps/rotom-ng-ui-server/`, builds the `rotom-ng-ui` binary: it embeds the **same** UI bundle and reverse-proxies to several rotom-ng instances, acting as a multi-instance admin panel. Note the name collision -- `apps/rotom-ng-ui/` is the React app, `apps/rotom-ng-ui-server/` is the Go service whose binary is named after it. See `docs/RotomNG-UI-Server.md`.

There is one UI, not two. It detects its mode at runtime from `/api/config`: the admin service always returns an `instances` key (empty list included), a plain rotom-ng never does. Anything the UI gates on config must read `useActiveConfig()` (the selected instance's config), not `useConfig()` (the server's own).

## Build Commands

```bash
make                 # Build both binaries
make rotom-ng        # Build UI + rotom-ng binary (UI must build first)
make rotom-ng-ui     # Build UI + rotom-ng-ui admin binary
make ui              # Build React UI only (bun install && bun run build)
make docker          # Build rotom-ng Docker image (Dockerfile --target rotom-ng)
make docker-ui       # Build rotom-ng-ui Docker image (--target rotom-ng-ui)
make clean           # Remove build artifacts and generated files
```

## Go

- **Go 1.26+**, module path: `github.com/UnownHash/RotomNG`
- **Code generation required before build/test**: `go generate ./apps/rotom-ng/... ./libs/...`
  - `apps/rotom-ng-ui-server/` has no generate step of its own; it imports rotom-ng's `version` package
  - Generates protobuf code (`libs/protos/rotom.pb.go`) and version info
  - Requires `protoc` and `protoc-gen-go` installed
- **Test**: `go test -race ./...`
- **Vet**: `go vet ./...`

### Linting (CRITICAL)

All Go code **must** pass `golangci-lint` using the repo's `.golangci.yml`. Run:

```bash
golangci-lint run -c .golangci.yml ./...
```

Key linter settings to keep in mind when writing Go code:

- **All linters enabled by default** with selective disables -- be conservative
- `gocyclo` max complexity: 20
- `goconst` triggers at 3+ occurrences of strings with length >= 3
- `nestif` max complexity: 5 -- keep nesting shallow
- `nakedret` max function lines: 30 -- use named returns only in short functions
- `wrapcheck` is enabled -- wrap errors from external packages with `fmt.Errorf("context: %w", err)`
  - Exceptions: `libs/ws/` and `libs/testutil/` are excluded from wrapcheck
- `errcheck` is enabled -- handle returned errors (except `Close()`, `fmt.Fprint*`, `SetReadDeadline`)
- `gofmt` formatter is enforced
- Tests (`_test.go`) have relaxed rules (errcheck, bodyclose, gosec, etc. are excluded)

### Go Conventions

- **Errors**: Define sentinel errors at package level with constructor/checker functions:
  ```go
  var errThingNotFound = errors.New("thing not found")
  func NewErrThingNotFound() error { return errThingNotFound }
  func IsErrThingNotFound(err error) bool { return errors.Is(err, errThingNotFound) }
  ```
- **Logging**: Use `log/slog` (not zerolog/logrus). Store loggers in context via `logging.ContextWithLogger()`
- **Config**: Uses `koanf` with TOML. Config structs implement `Validate()` and `SetDefaults()`
- **HTTP framework**: Gin (`github.com/gin-gonic/gin`)
- **Testing**: Standard `testing` package only (no testify). Table-driven tests preferred
- **Dependency injection**: Constructor functions (`NewXxx(cfg XxxConfig)`) over globals

## TypeScript / React

- **Package manager**: `bun` (use `bun install --frozen-lockfile`)
- **Build tool**: Vite via Nx
- **Formatter/Linter**: Biome
  - Indentation: **2 spaces**
  - Quote style: **double quotes**
  - Run: `bun run check` (format + lint), `bun run lint`, `bun run format`
- **Shared components**: `libs/base-ui/` (Radix UI, Tailwind CSS v4, Lucide icons)
- **Path aliases**: `@rotom-ng/base-ui` -> `libs/base-ui/src/index.ts`, `@/*` -> lib src root
- **Dev server**: `bun run dev` (port 4201, proxies `/api` to `localhost:7072`)
  - `bun run dev:mock` runs against MSW; `?mock=medium,multi` serves the admin service's config so multi-instance mode can be worked on without one, and `?mock=medium,multi,live` flaps an instance to exercise the unreachable states
- **Build output**: `libs/rotom_ui/static/` (embedded into Go binary)

## CI Pipeline

Every push runs: test (Go) -> lint (golangci-lint) -> build (multi-arch Docker). All must pass.

Both images -- `rotom-ng` and `rotom-ng-ui` -- are built on every push and
published together from the same commit, so a given tag means the same version
of both. The publish, manual-publish, and release workflows each matrix over
image x platform, so adding a third image means one more matrix entry rather
than a new job.

Both come from the single root `Dockerfile`, one build target apiece, sharing
the bun and Go builder stages. The matrix entry name is the build target, the
GHCR repo, and the binary name all at once, so a new image means adding a
target and listing it. `rotom-ng` is the last stage and therefore the default
target for a bare `docker build .`.

## Generated Files (do not edit manually)

- `libs/protos/rotom.pb.go` -- generated from `libs/protos/rotom.proto`
- `apps/rotom-ng/app/version/version.go` -- generated from `version.txt`
- `apps/rotom-ng-ui/src/version.ts` -- generated during UI build
- `libs/rotom_ui/static/` -- built UI assets
