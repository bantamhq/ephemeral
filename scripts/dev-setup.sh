#!/bin/bash
# Setup dev environment using CLI commands
set -e

cd "$(dirname "$0")/../workspace"

# Kill anything already running on port 8080
lsof -ti :8080 | xargs kill 2>/dev/null || true

echo "Initializing server..."
./eph admin init --non-interactive

echo "Creating dev user..."
TOKEN_OUTPUT=$(./eph admin user add dev 2>&1)
USER_TOKEN=$(echo "$TOKEN_OUTPUT" | grep "^Token: " | cut -d' ' -f2)

if [ -z "$USER_TOKEN" ]; then
    echo "Failed to create user or extract token"
    echo "$TOKEN_OUTPUT"
    exit 1
fi

echo "Creating client config..."
cat > client.toml << EOF
server = "http://localhost:8080"
token = "$USER_TOKEN"
default_namespace = "dev"
EOF

echo "Starting server for seeding..."
./eph serve > server.log 2>&1 &
SERVER_PID=$!

# Wait for server to be ready
echo "Waiting for server..."
for i in {1..50}; do
    if curl -s "http://localhost:8080/health" > /dev/null 2>&1; then
        echo "Server ready"
        break
    fi
    sleep 0.2
done

if ! curl -s "http://localhost:8080/health" > /dev/null 2>&1; then
    echo "Server failed to start"
    cat server.log
    exit 1
fi

echo "Seeding data..."
../scripts/seed.sh "$USER_TOKEN"

echo ""
echo "Stopping temp server..."
kill $SERVER_PID 2>/dev/null || true
rm -f server.log

echo ""
echo "Setup complete!"
echo "Starting server (Ctrl+C to stop)"
echo "Run TUI in another terminal: make dev-tui"
echo ""
exec ./eph serve
