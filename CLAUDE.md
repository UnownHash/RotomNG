# CLAUDE.md

## Project Overview

RotomNG is a distributed MITM proxy connection manager. Monorepo with a Go backend (`apps/rotom-ng/`) and React/TypeScript frontend (`apps/rotom-ng-ui/`), sharing libraries in `libs/`.

## Build Commands

```bash
make rotom-ng        # Build UI + Go binary (UI must build first)
make rotom-ng-ui     # Build React UI only (bun install && bun run build)
make docker          # Build Docker image
make clean           # Remove build artifacts and generated files
```

## Go

- **Go 1.26+**, module path: `github.com/UnownHash/RotomNG`
- **Code generation required before build/test**: `go generate ./apps/rotom-ng/... ./libs/...`
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
- **Build output**: `libs/rotom_ui/static/` (embedded into Go binary)

## CI Pipeline

Every push runs: test (Go) -> lint (golangci-lint) -> build (multi-arch Docker). All must pass.

## Generated Files (do not edit manually)

- `libs/protos/rotom.pb.go` -- generated from `libs/protos/rotom.proto`
- `apps/rotom-ng/app/version/version.go` -- generated from `version.txt`
- `apps/rotom-ng-ui/src/version.ts` -- generated during UI build
- `libs/rotom_ui/static/` -- built UI assets
