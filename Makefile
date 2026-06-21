BINARY   := debforge
VERSION  := $(shell git describe --tags --always 2>/dev/null || echo "0.1.0-dev")
LDFLAGS  := -ldflags="-X main.version=$(VERSION)"

.PHONY: build clean test install

build:
	go build $(LDFLAGS) -o bin/$(BINARY) ./cmd/debforge/

clean:
	rm -rf bin/

test:
	go test ./...

install: build
	install -d /opt/debforge/bin
	cp bin/$(BINARY) /opt/debforge/bin/$(BINARY)
	ln -sf /opt/debforge/bin/$(BINARY) /usr/local/bin/$(BINARY)
