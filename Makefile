BINARY  := mevius
PKG     := ./...
CMD     := ./cmd/mevius

.PHONY: build test lint run tidy clean

build:
	go build -o $(BINARY) $(CMD)

test:
	go test $(PKG)

lint:
	go vet $(PKG)
	npm --prefix web run lint

run:
	go run $(CMD)

tidy:
	go mod tidy

clean:
	rm -f $(BINARY)
