#!/bin/sh
set -e

if ! command -v protoc >/dev/null 2>&1; then
    echo "protoc not found. Install it: apt install protobuf-compiler, or brew install protobuf." >&2
    exit 1
fi

# protoc-gen-go comes from the tool directive in go.mod, so it is built on
# demand at the same google.golang.org/protobuf version the generated code
# links against. Nothing to install by hand, and the two cannot drift apart.
plugin="$(go tool -n protoc-gen-go)"

exec protoc \
    --plugin=protoc-gen-go="$plugin" \
    --go_opt=paths=source_relative \
    --experimental_allow_proto3_optional \
    --go_opt=Mrotom.proto=github.com/UnownHash/RotomNG/libs/protos \
    --go_out=. \
    rotom.proto
