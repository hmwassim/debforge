BINARY := debforge

.PHONY: build clean

build:
	go build -o $(BINARY) ./cmd/debforge/

clean:
	rm -f $(BINARY)
