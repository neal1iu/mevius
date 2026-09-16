BINARY  := mevius
PKG     := ./...
CMD     := ./cmd/mevius
GOBIN   := $(shell go env GOPATH)/bin
LINT    := $(GOBIN)/golangci-lint

.PHONY: build test lint run tidy clean

build:
	go build -o $(BINARY) $(CMD)

test:
	go test $(PKG)

lint:
	$(LINT) run

run:
	go run $(CMD)

tidy:
	go mod tidy

clean:
	rm -f $(BINARY)
