BINARY ?= bin/daprd-ydb
SOCKETS_FOLDER ?= /tmp/dapr-components-sockets
COMPOSE ?= docker compose -f deploy/docker-compose.yml

.PHONY: build lint test conformance binding-integration run clean tidy ydb-up ydb-down

## build: compile the pluggable component binary
build:
	go build -o $(BINARY) ./cmd/daprd-ydb

## lint: run golangci-lint over the module
lint:
	golangci-lint run ./...

## test: run unit tests (excludes the conformance suite, which needs YDB)
test:
	go test ./cmd/... ./internal/...

## conformance: bring up YDB and run the Dapr state conformance suite
conformance: ydb-up
	go test -tags=conformance -v ./tests/conformance/...

## binding-integration: bring up YDB and run the output-binding integration tests
binding-integration: ydb-up
	go test -tags=integration -v ./tests/integration/...

## run: build and run the component, creating the sockets folder
## Sets both the modern (plural) and SDK (singular) env vars so a custom folder
## stays in sync regardless of which name is honored.
run: build
	mkdir -p $(SOCKETS_FOLDER)
	DAPR_COMPONENTS_SOCKETS_FOLDER=$(SOCKETS_FOLDER) DAPR_COMPONENT_SOCKETS_FOLDER=$(SOCKETS_FOLDER) ./$(BINARY)

## ydb-up: start a local YDB instance for tests/dev
ydb-up:
	$(COMPOSE) up -d ydb

## ydb-down: stop the local YDB instance
ydb-down:
	$(COMPOSE) down

## tidy: sync go.mod/go.sum
tidy:
	go mod tidy

## clean: remove build artifacts
clean:
	rm -rf bin
