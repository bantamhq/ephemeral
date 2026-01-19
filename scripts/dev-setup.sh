#!/bin/bash
# Setup dev environment: start server, capture token, create config, seed
set -e

cd "$(dirname "$0")/../workspace"

echo "Starting server to capture token..."
./ephemeral serve > server.log 2>&1 &
SERVER_PID=$!

# Wait for token
for i in {1..20}; do
    if [ -f server.log ] && grep -q "ROOT TOKEN GENERATED" server.log 2>/dev/null; then
        break
    fi
    sleep 0.3
done

TOKEN=$(grep -A1 "ROOT TOKEN GENERATED" server.log 2>/dev/null | tail -1 | tr -d ' ')

if [ -z "$TOKEN" ]; then
    echo "Failed to get token"
    kill $SERVER_PID 2>/dev/null || true
    cat server.log 2>/dev/null || echo "No log file"
    exit 1
fi

echo "Creating client config..."
cat > client.toml << EOF
current_context = "dev"

[contexts.dev]
server = "http://localhost:8080"
token = "$TOKEN"
namespace = "default"
EOF

echo "Seeding data..."
../scripts/seed.sh "$TOKEN"

echo ""
echo "Setup complete. Stopping temp server..."
kill $SERVER_PID 2>/dev/null || true
rm -f server.log

echo ""
echo "Starting server (Ctrl+C to stop)"
echo "Run TUI in another terminal: make dev-tui"
echo ""
exec ./ephemeral serve
