BINARY  := reserve
GOFLAGS := -mod=vendor

.PHONY: build test test-cover lint clean run install help

## build: compile the reserve binary
build:
	go build $(GOFLAGS) -o $(BINARY) .

## test: run all tests with verbose output
test:
	go test $(GOFLAGS) -v ./tests/

## test-all: run tests across all packages
test-all:
	go test $(GOFLAGS) -v ./...

## test-cover: run tests with HTML coverage report
test-cover:
	go test $(GOFLAGS) -coverprofile=coverage.out ./tests/
	go tool cover -html=coverage.out -o coverage.html
	@echo "Coverage report: coverage.html"

## lint: vet all packages
lint:
	go vet $(GOFLAGS) ./...

## clean: remove build artifacts
clean:
	rm -f $(BINARY) coverage.out coverage.html

## run: build and run with args  (usage: make run ARGS="series get GDP")
run: build
	./$(BINARY) $(ARGS)

## install: install to $$GOPATH/bin
install:
	go install $(GOFLAGS) .

## help: show available targets
help:
	@grep -E '^## ' Makefile | sed 's/## /  /'
