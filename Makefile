# Job Matcher — Build Targets
# Usage:
#   make build-windows   → dist/job-matcher.exe
#   make build-linux     → dist/job-matcher-linux
#   make build-all       → both
#   make run             → run locally (current OS)
#   make test            → run all tests
#   make test-verbose    → run tests with full output
#   make tidy            → go mod tidy

BINARY_NAME := job-matcher
DIST        := dist
MODULE      := github.com/QuaziBit/job-matcher-go

.PHONY: all build-windows build-linux build-all run test test-verbose tidy clean

all: build-all

build-windows:
	@mkdir -p $(DIST)
	GOOS=windows GOARCH=amd64 go build -ldflags="-s -w" -o $(DIST)/$(BINARY_NAME).exe .
	@echo "Built: $(DIST)/$(BINARY_NAME).exe"

build-linux:
	@mkdir -p $(DIST)
	GOOS=linux GOARCH=amd64 go build -ldflags="-s -w" -o $(DIST)/$(BINARY_NAME)-linux .
	@echo "Built: $(DIST)/$(BINARY_NAME)-linux"

build-all: build-windows build-linux
	@echo "All builds complete."
	@ls -lh $(DIST)/

run:
	go run .

test:
	go test ./... -count=1

test-verbose:
	go test ./... -v -count=1

test-pretty:
	go run ./cmd/testrunner

tidy:
	go mod tidy

clean:
	rm -rf $(DIST)
