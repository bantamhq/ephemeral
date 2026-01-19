.PHONY: build run clean test test-api test-repos test-tokens test-namespaces test-folders test-labels test-content workspace-setup workspace-run

# Build the binary
build:
	go build -o workspace/ephemeral ./cmd/ephemeral

# Run the server (from workspace)
run: build
	cd workspace && ./ephemeral serve

# Clean build artifacts and test data
clean:
	rm -rf workspace/ephemeral workspace/data

# Full clean (including test repos)
clean-all:
	rm -rf workspace/

# Setup workspace for first time
workspace-setup:
	mkdir -p workspace
	cp config.example.toml workspace/config.toml
	@echo "Workspace created. Edit workspace/config.toml if needed."

# Build and run from workspace
workspace-run: workspace-setup build
	cd workspace && ./ephemeral serve

# Run unit tests
test:
	go test ./...

# Run all API integration tests (self-contained: builds, starts server, runs tests, cleans up)
# Usage: make test-api
test-api:
	@./tests/api/run_all.sh

# Individual API test suites (server must be running)
# Usage: make test-repos TOKEN=eph_xxx
test-repos:
	@./tests/api/repos.sh $(TOKEN)

test-tokens:
	@./tests/api/tokens.sh $(TOKEN)

test-namespaces:
	@./tests/api/namespaces.sh $(TOKEN)

test-folders:
	@./tests/api/folders.sh $(TOKEN)

test-labels:
	@./tests/api/labels.sh $(TOKEN)

test-content:
	@./tests/api/content.sh $(TOKEN)

# Seed test data (server must be running)
# Usage: make seed TOKEN=eph_xxx
seed:
	@cd workspace && ./seed.sh $(TOKEN)

# Development mode - rebuild and run on changes (requires entr)
watch:
	find . -name "*.go" | entr -r make run