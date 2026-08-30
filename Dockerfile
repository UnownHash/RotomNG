# Builds both RotomNG binaries. Select one with --target:
#
#   docker build --target rotom-ng    -t rotom-ng .
#   docker build --target rotom-ng-ui -t rotom-ng-ui .
#
# The two images shared all but ten lines when they lived in separate files --
# the whole bun stage and the entire Go module setup -- so they are one file
# with a target apiece. BuildKit only builds the stages a target depends on, so
# asking for one does not build the other.
#
# rotom-ng is the last stage, and therefore what a bare "docker build ." with no
# --target produces: the connection manager is the primary service, and the
# admin UI is optional.

# Build stage for Node.js frontend
FROM oven/bun:1-alpine AS node-builder

# Add build argument for dev mode
ARG DEV_MODE=false

WORKDIR /app

# Copy these first so we can take advantage of layer caching
# after the deps are installed.
COPY package.json bun.lock ./
COPY libs/base-ui/package.json libs/base-ui/package.json
COPY apps/rotom-ng-ui/package.json apps/rotom-ng-ui/package.json
RUN bun install --frozen-lockfile

COPY tsconfig.base.json nx.json biome.json ./
COPY libs/base-ui libs/base-ui
COPY apps/rotom-ng-ui apps/rotom-ng-ui
COPY apps/rotom-ng/app/version/version.txt apps/rotom-ng/app/version/version.txt

# Set NODE_ENV and build configuration based on DEV_MODE
RUN if [ "$DEV_MODE" = "true" ]; then \
        echo "Building in development mode (no minification)..." && \
        NODE_ENV=development bun run build:dev; \
    else \
        echo "Building in production mode (with minification)..." && \
        NODE_ENV=production bun run build; \
    fi

# Everything both Go binaries need. Split from the build stages below so that
# building one target does not compile the other.
FROM golang:1.26-alpine AS go-base

WORKDIR /app

# Install git to add sha to binary
RUN apk --no-cache add git protobuf-dev
RUN go install google.golang.org/protobuf/cmd/protoc-gen-go@latest
# Copy go mod files first for better caching
COPY go.mod go.sum ./
COPY vendo[r] vendor
RUN if [ ! -f vendor/modules.txt ]; then rm -rf vendor && go mod download ; fi
COPY libs libs
COPY --from=node-builder /app/libs/rotom_ui/static libs/rotom_ui/static
# apps/rotom-ng is needed by both: the admin server imports its version package.
COPY apps/rotom-ng apps/rotom-ng
COPY .git .git

FROM go-base AS build-rotom-ng
RUN CGO_ENABLED=0 go build -a -ldflags='-s -w' -o rotom-ng ./apps/rotom-ng

FROM go-base AS build-rotom-ng-ui
COPY apps/rotom-ng-ui-server apps/rotom-ng-ui-server
RUN CGO_ENABLED=0 go build -a -ldflags='-s -w' -o rotom-ng-ui ./apps/rotom-ng-ui-server

# Common runtime. ca-certificates is needed for HTTPS requests.
FROM alpine AS runtime-base
RUN apk --no-cache add ca-certificates

# The multi-instance admin UI. Optional; see docs/RotomNG-UI-Server.md.
FROM runtime-base AS rotom-ng-ui
WORKDIR /rotom-ng-ui
COPY --from=build-rotom-ng-ui /app/rotom-ng-ui .
CMD ["./rotom-ng-ui"]

# The connection manager. Last, so it is also the default target.
FROM runtime-base AS rotom-ng
WORKDIR /rotom-ng
COPY --from=build-rotom-ng /app/rotom-ng .
CMD ["./rotom-ng"]
