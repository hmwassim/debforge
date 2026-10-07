VERSION ?= $(shell git describe --tags --dirty --always 2>/dev/null || echo dev)
LDFLAGS := -s -w -X main.version=$(VERSION)
GO ?= go

.PHONY: build test race vet lint live itest release clean

build:
	CGO_ENABLED=0 $(GO) build -trimpath -ldflags "$(LDFLAGS)" -o bin/debforge ./cmd/debforge

test:
	$(GO) test ./...

race:
	$(GO) test -race ./...

vet:
	$(GO) vet ./...

lint:
	golangci-lint run

# Checks upstream versions and Debian package names over the network.
live:
	DEBFORGE_LIVE=1 $(GO) test ./catalog/ -run Live -v

# End-to-end scenarios in a disposable debian:trixie container (needs docker).
itest: build
	./test/integration.sh bin/debforge

release: clean
	mkdir -p dist
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 $(GO) build -trimpath -ldflags "$(LDFLAGS)" -o dist/debforge-linux-amd64 ./cmd/debforge
	echo "$(VERSION)" > dist/VERSION
	cd dist && sha256sum debforge-linux-amd64 VERSION > SHA256SUMS

clean:
	rm -rf bin dist
