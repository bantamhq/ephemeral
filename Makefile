.PHONY: build run clean test test-api test-auth test-repos test-tokens test-namespaces test-folders test-content workspace-setup dev dev-tui seed watch

# Build the binary
build:
	go build -o workspace/eph ./cmd/eph

# Run the server (from workspace)
run: build
	cd workspace && ./eph serve

# Clean build artifacts and test data
clean:
	rm -rf workspace/eph workspace/data workspace/client.toml

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
	@cd workspace && EPHEMERAL_CONFIG=./client.toml ./eph

# Run unit tests
test:
	go test ./...

# Run all API integration tests (self-contained: builds, starts server, runs tests, cleans up)
# Usage: make test-api
test-api:
	@./scripts/tests/run_all.sh

# Individual API test suites (server must be running)
# Usage: make test-repos TOKEN=eph_xxx
test-repos:
	@./scripts/tests/repos.sh $(TOKEN)

test-tokens:
	@./scripts/tests/tokens.sh $(TOKEN)

test-namespaces:
	@./scripts/tests/namespaces.sh $(TOKEN)

test-folders:
	@./scripts/tests/folders.sh $(TOKEN)

test-content:
	@./scripts/tests/content.sh $(TOKEN)

test-auth:
	@./scripts/tests/auth.sh

# Seed test data (server must be running)
# Usage: make seed TOKEN=eph_xxx
seed:
	@./scripts/seed.sh $(TOKEN)

# Development mode - rebuild and run on changes (requires entr)
watch:
	find . -name "*.go" | entr -r make run