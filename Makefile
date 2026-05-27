APP := helena
PKG := ./cmd/helena

.PHONY: run build test vet fmt lint tidy clean

run:
	go run $(PKG)

build:
	go build -o bin/$(APP) $(PKG)

test:
	go test ./...

vet:
	go vet ./...

fmt:
	gofmt -w .

lint:
	golangci-lint run

tidy:
	go mod tidy

clean:
	rm -rf bin dist
