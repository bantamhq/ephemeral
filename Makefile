.PHONY: build run clean test test-content test-repos-tokens test-integration workspace-setup workspace-run

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

# Run Content API integration tests (server must be running)
# Usage: make test-content TOKEN=eph_xxx
test-content:
	@./tests/content_api_test.sh $(TOKEN)

# Run Repos & Tokens API integration tests (server must be running)
# Usage: make test-repos-tokens TOKEN=eph_xxx
test-repos-tokens:
	@./tests/repos_tokens_api_test.sh $(TOKEN)

# Run all integration tests (server must be running)
# Usage: make test-integration TOKEN=eph_xxx
test-integration: test-repos-tokens test-content

# Development mode - rebuild and run on changes (requires entr)
watch:
	find . -name "*.go" | entr -r make run