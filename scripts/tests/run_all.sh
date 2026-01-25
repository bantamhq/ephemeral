#!/bin/bash
# Run all API tests with automatic server lifecycle management
set -e

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
PROJECT_ROOT="$(cd "$SCRIPT_DIR/../.." && pwd)"

# Colors
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m'

# Cleanup function
cleanup() {
    if [ -n "$SERVER_PID" ] && kill -0 "$SERVER_PID" 2>/dev/null; then
        echo -e "${YELLOW}Stopping server (PID $SERVER_PID)...${NC}"
        kill "$SERVER_PID" 2>/dev/null || true
        wait "$SERVER_PID" 2>/dev/null || true
    fi
    if [ -n "$TEST_DIR" ] && [ -d "$TEST_DIR" ]; then
        echo -e "${YELLOW}Cleaning up $TEST_DIR...${NC}"
        rm -rf "$TEST_DIR"
    fi
}

trap cleanup EXIT

TEST_PORT=18080

# Check if port is already in use
if lsof -i ":$TEST_PORT" >/dev/null 2>&1; then
    echo -e "${RED}Error: Port $TEST_PORT is already in use${NC}"
    echo "Another process is listening on this port:"
    lsof -i ":$TEST_PORT"
    echo ""
    echo "Kill the process or wait for it to finish before running tests."
    exit 1
fi

# Create temp directory for test
TEST_DIR=$(mktemp -d)
echo -e "${BLUE}Test directory: $TEST_DIR${NC}"

# Build the binary
echo -e "${BLUE}Building eph...${NC}"
cd "$PROJECT_ROOT"
go build -o "$TEST_DIR/eph" ./cmd/eph

# Create config
cat > "$TEST_DIR/server.toml" << EOF
[server]
port = $TEST_PORT
host = "127.0.0.1"

[storage]
data_dir = "./data"
EOF

# Start server and capture output
echo -e "${BLUE}Starting server...${NC}"
cd "$TEST_DIR"
./eph serve > server.log 2>&1 &
SERVER_PID=$!

# Wait for server to start and extract admin token
ADMIN_TOKEN=""
for i in {1..30}; do
    if [ -f server.log ]; then
        ADMIN_TOKEN=$(grep -m1 "^eph_" server.log 2>/dev/null | tr -d ' ' || true)
        if [ -n "$ADMIN_TOKEN" ] && [[ "$ADMIN_TOKEN" == eph_* ]]; then
            break
        fi
    fi
    sleep 0.2
done

# Also try reading from file
if [ -z "$ADMIN_TOKEN" ] && [ -f data/admin-token ]; then
    ADMIN_TOKEN=$(cat data/admin-token)
fi

if [ -z "$ADMIN_TOKEN" ] || [[ "$ADMIN_TOKEN" != eph_* ]]; then
    echo -e "${RED}Failed to get admin token from server${NC}"
    echo "Server log:"
    cat server.log
    exit 1
fi

# Wait for server to be ready
for i in {1..30}; do
    if curl -s "http://127.0.0.1:$TEST_PORT/health" > /dev/null 2>&1; then
        break
    fi
    sleep 0.2
done

if ! curl -s "http://127.0.0.1:$TEST_PORT/health" > /dev/null 2>&1; then
    echo -e "${RED}Server failed to start${NC}"
    echo "Server log:"
    cat server.log
    exit 1
fi

echo -e "${GREEN}Server ready${NC}"

# Create test namespace
echo -e "${BLUE}Creating test namespace...${NC}"
NS_RESPONSE=$(curl -s -X POST \
    -H "Authorization: Bearer $ADMIN_TOKEN" \
    -H "Content-Type: application/json" \
    -d '{"name":"test"}' \
    "http://127.0.0.1:$TEST_PORT/api/v1/admin/namespaces")

NS_ID=$(echo "$NS_RESPONSE" | grep -o '"id":"[^"]*"' | head -1 | cut -d'"' -f4)

if [ -z "$NS_ID" ]; then
    echo -e "${RED}Failed to create test namespace${NC}"
    echo "$NS_RESPONSE"
    exit 1
fi

# Create test user
echo -e "${BLUE}Creating test user...${NC}"
USER_RESPONSE=$(curl -s -X POST \
    -H "Authorization: Bearer $ADMIN_TOKEN" \
    -H "Content-Type: application/json" \
    -d '{"username":"test"}' \
    "http://127.0.0.1:$TEST_PORT/api/v1/admin/users")

USER_ID=$(echo "$USER_RESPONSE" | grep -o '"id":"[^"]*"' | head -1 | cut -d'"' -f4)

if [ -z "$USER_ID" ]; then
    echo -e "${RED}Failed to create test user${NC}"
    echo "$USER_RESPONSE"
    exit 1
fi

# Grant user access to namespace
echo -e "${BLUE}Granting namespace access...${NC}"
GRANT_RESPONSE=$(curl -s -X POST \
    -H "Authorization: Bearer $ADMIN_TOKEN" \
    -H "Content-Type: application/json" \
    -d "{\"namespace_id\":\"$NS_ID\",\"allow\":[\"namespace:admin\",\"repo:admin\"],\"is_primary\":true}" \
    "http://127.0.0.1:$TEST_PORT/api/v1/admin/users/$USER_ID/namespace-grants")

# Create user token
echo -e "${BLUE}Creating user token...${NC}"
TOKEN_RESPONSE=$(curl -s -X POST \
    -H "Authorization: Bearer $ADMIN_TOKEN" \
    -H "Content-Type: application/json" \
    -d '{"name":"test-token"}' \
    "http://127.0.0.1:$TEST_PORT/api/v1/admin/users/$USER_ID/tokens")

TOKEN=$(echo "$TOKEN_RESPONSE" | grep -o '"token":"[^"]*"' | head -1 | cut -d'"' -f4)

if [ -z "$TOKEN" ]; then
    echo -e "${RED}Failed to create user token${NC}"
    echo "$TOKEN_RESPONSE"
    exit 1
fi

# Export for test scripts
export TOKEN
export ADMIN_TOKEN
export NS_ID
export BASE_URL="http://127.0.0.1:$TEST_PORT"

echo ""
echo -e "${BLUE}╔═══════════════════════════════════════╗${NC}"
echo -e "${BLUE}║     Ephemeral API Integration Tests   ║${NC}"
echo -e "${BLUE}╚═══════════════════════════════════════╝${NC}"
echo ""
echo -e "Server: ${YELLOW}$BASE_URL${NC}"
echo -e "Namespace: ${YELLOW}test${NC}"
echo ""

FAILED_SUITES=""

run_suite() {
    local name="$1"
    local script="$2"

    if ! bash "$SCRIPT_DIR/$script"; then
        FAILED_SUITES="$FAILED_SUITES $name"
    fi
}

# Run each test suite
run_suite "System" "system.sh"
run_suite "Auth" "auth.sh"
run_suite "Repos" "repos.sh"
run_suite "Admin-Tokens" "tokens.sh"
run_suite "Admin-Users" "users.sh"
run_suite "Admin-Namespaces" "namespaces.sh"
run_suite "Folders" "folders.sh"
run_suite "Content" "content.sh"

# Final summary
echo ""
echo -e "${BLUE}╔═══════════════════════════════════════╗${NC}"
echo -e "${BLUE}║           Final Summary               ║${NC}"
echo -e "${BLUE}╚═══════════════════════════════════════╝${NC}"
echo ""

if [ -z "$FAILED_SUITES" ]; then
    echo -e "${GREEN}All test suites passed!${NC}"
    exit 0
else
    echo -e "${RED}Failed suites:$FAILED_SUITES${NC}"
    exit 1
fi
