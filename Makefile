ALL=rotom-ng rotom-ng-ui

all: $(ALL)

deps:
	@if [ ! -f ../../vendor/modules.txt ]; then go mod download; fi

# Only re-generate protos on demand.
generate-protos:
	@go generate ./libs/protos/...

# The React bundle, shared by both binaries: each embeds libs/rotom_ui/static,
# and the UI decides at runtime which service it is talking to.
ui:
	@bun install && bun run build

rotom-ng: ui deps
	@go generate ./apps/rotom-ng/...
	@CGO_ENABLED=0 go build -ldflags="-s -w" -o rotom-ng ./apps/rotom-ng
	@echo rotom-ng has been built.

# The admin UI server: serves the same UI for several rotom-ng instances and
# proxies to them. Shares rotom-ng's version package, hence the same generate.
rotom-ng-ui: ui deps
	@go generate ./apps/rotom-ng/...
	@CGO_ENABLED=0 go build -ldflags="-s -w" -o rotom-ng-ui ./apps/rotom-ng-ui-server
	@echo rotom-ng-ui has been built.

docker:
	@docker build --target rotom-ng -t rotom-ng:latest .

docker-ui:
	@docker build --target rotom-ng-ui -t rotom-ng-ui:latest .

clean:
	@rm -rf rotom-ng rotom-ng-ui libs/rotom_ui/static
	@rm -rf .nx node_modules libs/base-ui/node_modules apps/rotom-ng-ui/src/version.ts
