ALL=rotom-ng

all: $(ALL)

deps:
	@if [ ! -f ../../vendor/modules.txt ]; then go mod download; fi

# Only re-generate protos on demand.
generate-protos:
	@go generate ./libs/protos/...

rotom-ng: rotom-ng-ui deps
	@go generate ./apps/rotom-ng/...
	@CGO_ENABLED=0 go build -ldflags="-s -w" -o rotom-ng ./apps/rotom-ng
	@echo rotom-ng has been built.

rotom-ng-ui:
	@bun install && bun run build

docker:
	@docker build -f apps/rotom-ng/Dockerfile -t rotom-ng:latest .

clean:
	@rm -rf rotom-ng libs/rotom_ui/static apps/rotom-ng/app/version/version.go libs/protos/rotom.pb.go
	@rm -rf .nx node_modules libs/base-ui/node_modules apps/rotom-ng-ui/src/version.ts
