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
	install -d /usr/share/bash-completion/completions /usr/share/zsh/vendor-completions /usr/share/fish/vendor_completions.d
	install -m644 completions/debforge.bash /usr/share/bash-completion/completions/debforge
	install -m644 completions/_debforge /usr/share/zsh/vendor-completions/_debforge
	install -m644 completions/debforge.fish /usr/share/fish/vendor_completions.d/debforge.fish
