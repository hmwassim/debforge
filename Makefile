BINARY    := debforge
VERSION   := $(shell git describe --tags --always 2>/dev/null || echo "0.1.0-dev")
LDFLAGS   := -ldflags="-X main.version=$(VERSION)"
GO        := go
GOPATH    := $(CURDIR)/.gopath
GOMODCACHE := $(GOPATH)/mod
GOCACHE   := $(GOPATH)/buildcache
export GOPATH GOMODCACHE GOCACHE

.PHONY: build clean test install lint vet fmt race cover

build:
	$(GO) build $(LDFLAGS) -o bin/$(BINARY) ./cmd/debforge/

clean:
	rm -rf bin/

test:
	$(GO) test ./...

vet:
	$(GO) vet ./...

fmt:
	gofmt -l -w .

race:
	$(GO) test -race ./...

cover:
	$(GO) test -coverprofile=coverage.out ./...
	$(GO) tool cover -func=coverage.out | grep total

lint:
	@command -v golangci-lint >/dev/null 2>&1 || { echo "golangci-lint not installed. Install with: go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest"; exit 1; }
	golangci-lint run ./...

# NOTE: /opt/debforge and /usr/local/bin/debforge below must match
# DefaultRootDir / DefaultLinkPath in internal/self/config.go and the
# equivalent values in install.sh. Make cannot import Go constants, so
# these three places are kept in sync by hand - if you change one, change
# all three.
install: build
	install -d /opt/debforge/bin
	cp bin/$(BINARY) /opt/debforge/bin/$(BINARY)
	ln -sf /opt/debforge/bin/$(BINARY) /usr/local/bin/$(BINARY)
	install -d /usr/share/bash-completion/completions /usr/share/zsh/vendor-completions /usr/share/fish/vendor_completions.d
	install -m644 completions/debforge.bash /usr/share/bash-completion/completions/debforge
	install -m644 completions/_debforge /usr/share/zsh/vendor-completions/_debforge
	install -m644 completions/debforge.fish /usr/share/fish/vendor_completions.d/debforge.fish
