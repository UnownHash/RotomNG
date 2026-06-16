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

# Build stage for Go application
FROM golang:1.26-alpine AS go-builder

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
COPY apps/rotom-ng apps/rotom-ng
COPY .git .git
RUN CGO_ENABLED=0 go build -a -ldflags='-s -w' -o rotom-ng ./apps/rotom-ng

# Final runtime stage
FROM alpine

# Install ca-certificates for HTTPS requests
RUN apk --no-cache add ca-certificates

WORKDIR /rotom-ng

# Copy the Go binary from go-builder stage
COPY --from=go-builder /app/rotom-ng .

# Run the binary
CMD ["./rotom-ng"]
