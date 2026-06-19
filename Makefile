BINARY := debforge
VERSION := $(shell git describe --tags --always 2>/dev/null || echo "0.1.0-dev")
LDFLAGS := -ldflags="-X github.com/hmwassim/debforge/internal/commands.Version=$(VERSION)"
GOMODCACHE := $(CURDIR)/.debforge/mod
GOCACHE    := $(CURDIR)/.debforge/buildcache

.PHONY: build clean fmt vet

build:
	GOMODCACHE=$(GOMODCACHE) GOCACHE=$(GOCACHE) \
	  go build -o $(BINARY) $(LDFLAGS) ./cmd/debforge/

clean:
	rm -f $(BINARY)
	chmod -R u+w $(CURDIR)/.debforge 2>/dev/null; rm -rf $(CURDIR)/.debforge

fmt:
	gofmt -l .

vet:
	go vet ./...
