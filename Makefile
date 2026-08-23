.PHONY: all fmt vet test test-race build run docker-build docker-run clean

GO ?= go
BINARY := bin/forest-operations

all: fmt vet test build

fmt:
	@test -z "$$($(GO) fmt ./...)"

vet:
	$(GO) vet ./...

test:
	$(GO) test -count=1 ./...

test-race:
	$(GO) test -race -count=1 ./...

build:
	mkdir -p bin
	CGO_ENABLED=0 $(GO) build -trimpath -o $(BINARY) ./cmd/server

run:
	$(GO) run ./cmd/server

docker-build:
	docker build --platform linux/arm64 -t shenzhen-forest-operations:local .

docker-run: docker-build
	docker run --rm -p 8080:8080 \
		-e FOREST_BOOTSTRAP_ADMIN_EMAIL=admin@example.com \
		-e FOREST_BOOTSTRAP_ADMIN_PASSWORD=change-this-development-password \
		shenzhen-forest-operations:local

clean:
	rm -rf bin coverage.out
