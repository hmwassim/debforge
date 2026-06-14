BINARY := debforge
VERSION := $(shell git describe --tags --always 2>/dev/null || echo "0.1.0-dev")
LDFLAGS := -ldflags="-X github.com/hmwassim/debforge/pkg/cli.Version=$(VERSION)"

.PHONY: build clean

build:
	go build -o $(BINARY) $(LDFLAGS) ./cmd/debforge/

clean:
	rm -f $(BINARY)
