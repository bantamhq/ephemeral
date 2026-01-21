#!/bin/bash
# Setup dev environment: start server, create namespace and token, seed, run server
set -e

cd "$(dirname "$0")/../workspace"

# Kill anything already running on port 8080
lsof -ti :8080 | xargs kill 2>/dev/null || true

echo "Starting server to capture admin token..."
./eph serve > server.log 2>&1 &
SERVER_PID=$!

# Wait for admin token
ADMIN_TOKEN=""
for i in {1..20}; do
    if [ -f server.log ]; then
        ADMIN_TOKEN=$(grep -m1 "^eph_" server.log 2>/dev/null | tr -d ' ')
        if [ -n "$ADMIN_TOKEN" ]; then
            break
        fi
    fi
    sleep 0.3
done

if [ -z "$ADMIN_TOKEN" ]; then
    # Try reading from file
    if [ -f data/admin-token ]; then
        ADMIN_TOKEN=$(cat data/admin-token)
    fi
fi

if [ -z "$ADMIN_TOKEN" ]; then
    echo "Failed to get admin token"
    kill $SERVER_PID 2>/dev/null || true
    cat server.log 2>/dev/null || echo "No log file"
    exit 1
fi

# Wait for server to be ready
for i in {1..30}; do
    if curl -s "http://localhost:8080/health" > /dev/null 2>&1; then
        break
    fi
    sleep 0.2
done

echo "Creating namespace..."
NS_RESPONSE=$(curl -s -X POST \
    -H "Authorization: Bearer $ADMIN_TOKEN" \
    -H "Content-Type: application/json" \
    -d '{"name":"dev"}' \
    "http://localhost:8080/api/v1/admin/namespaces")

NS_ID=$(echo "$NS_RESPONSE" | grep -o '"id":"[^"]*"' | head -1 | cut -d'"' -f4)

if [ -z "$NS_ID" ]; then
    echo "Failed to create namespace"
    echo "$NS_RESPONSE"
    kill $SERVER_PID 2>/dev/null || true
    exit 1
fi

echo "Creating user token..."
TOKEN_RESPONSE=$(curl -s -X POST \
    -H "Authorization: Bearer $ADMIN_TOKEN" \
    -H "Content-Type: application/json" \
    -d "{\"namespace_id\":\"$NS_ID\",\"name\":\"dev-token\",\"scope\":\"full\"}" \
    "http://localhost:8080/api/v1/admin/tokens")

USER_TOKEN=$(echo "$TOKEN_RESPONSE" | grep -o '"token":"[^"]*"' | head -1 | cut -d'"' -f4)

if [ -z "$USER_TOKEN" ]; then
    echo "Failed to create user token"
    echo "$TOKEN_RESPONSE"
    kill $SERVER_PID 2>/dev/null || true
    exit 1
fi

echo "Creating client config..."
cat > client.toml << EOF
server = "http://localhost:8080"
token = "$USER_TOKEN"
default_namespace = "dev"
EOF

echo "Seeding data..."
EPHEMERAL_DATA_DIR="./data" ../scripts/seed.sh "$USER_TOKEN"

echo ""
echo "Setup complete. Stopping temp server..."
kill $SERVER_PID 2>/dev/null || true
rm -f server.log

echo ""
echo "Starting server (Ctrl+C to stop)"
echo "Run TUI in another terminal: make dev-tui"
echo ""
exec ./eph serve
