.PHONY: build run clean test test-api test-repos test-tokens test-namespaces test-folders test-content workspace-setup dev dev-tui seed watch

# Build the binary
build:
	go build -o workspace/ephemeral ./cmd/ephemeral

# Run the server (from workspace)
run: build
	cd workspace && ./ephemeral serve

# Clean build artifacts and test data
clean:
	rm -rf workspace/ephemeral workspace/data workspace/client.toml

# Full clean (including test repos)
clean-all:
	rm -rf workspace/

# Setup workspace for first time
workspace-setup:
	mkdir -p workspace
	@test -f workspace/server.toml || cp server.example.toml workspace/server.toml

# Development environment: clean, build, capture token, create config, seed, run server
# Ctrl+C to stop. Run TUI in another terminal: make dev-tui
dev: clean workspace-setup build
	@./scripts/dev-setup.sh

# Run TUI with dev config
dev-tui:
	@cd workspace && EPHEMERAL_CONFIG=./client.toml ./ephemeral

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

test-content:
	@./tests/api/content.sh $(TOKEN)

# Seed test data (server must be running)
# Usage: make seed TOKEN=eph_xxx
seed:
	@./scripts/seed.sh $(TOKEN)

# Development mode - rebuild and run on changes (requires entr)
watch:
	find . -name "*.go" | entr -r make run