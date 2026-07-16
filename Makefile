.PHONY: all deps generate format test check build live live/templ live/server live/tailwind

all: generate

deps:
	go mod download
	npm ci --ignore-scripts

generate:
	go tool templ generate
	npm run build

format:
	go fmt ./...
	npm run format
	$(MAKE) generate

test:
	go test -race ./...

check: generate
	go vet ./...
	go test -race ./...
	npm run format:check
	npm audit --audit-level=high
	git diff --exit-code -- templates/templates_templ.go static third_party

build: generate
	CGO_ENABLED=1 go build -trimpath -o htmxchat .

live/templ:
	go tool templ generate --watch

live/server:
	go tool air --build.exclude_dir node_modules

live/tailwind:
	npm run build:css -- --watch

live:
	$(MAKE) -j3 live/tailwind live/templ live/server
