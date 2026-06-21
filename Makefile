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

# NOTE: /opt/debforge and /usr/local/bin/debforge below must match
# DefaultRootDir / DefaultLinkPath in internal/self/config.go and the
# equivalent values in inshall.sh. Make cannot import Go constants, so
# these three places are kept in sync by hand - if you change one, change
# all three.
install: build
	install -d /opt/debforge/bin
	cp bin/$(BINARY) /opt/debforge/bin/$(BINARY)
	ln -sf /opt/debforge/bin/$(BINARY) /usr/local/bin/$(BINARY)
