.PHONY: build run clean test workspace-setup workspace-run

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

# Run tests (when we add them)
test:
	go test ./...

# Development mode - rebuild and run on changes (requires entr)
watch:
	find . -name "*.go" | entr -r make run