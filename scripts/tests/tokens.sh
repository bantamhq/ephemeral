#!/bin/bash
# Tokens API Tests (Admin)
set -e

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
source "$SCRIPT_DIR/lib.sh"

require_token
require_admin_token
trap cleanup EXIT

echo ""
echo -e "${BLUE}═══════════════════════════════════════${NC}"
echo -e "${BLUE}  Tokens API Tests (Admin)${NC}"
echo -e "${BLUE}═══════════════════════════════════════${NC}"

###############################################################################
section "Admin: List Tokens"
###############################################################################

RESPONSE=$(admin_curl "$ADMIN_API/tokens")
expect_contains "$RESPONSE" '"data"' "returns data array"

# Should contain at least the admin token and test user token
expect_json_length "$RESPONSE" '.data' "2" "contains at least two tokens"

###############################################################################
section "Admin: Get Token"
###############################################################################

# Get first token ID from list
TOKEN_ID=$(echo "$RESPONSE" | jq -r '.data[0].id')

RESPONSE=$(admin_curl "$ADMIN_API/tokens/$TOKEN_ID")
expect_json "$RESPONSE" '.data.id' "$TOKEN_ID" "get token by ID"
expect_contains "$RESPONSE" '"created_at"' "token has created_at"

# Get non-existent token
RESPONSE=$(admin_curl "$ADMIN_API/tokens/nonexistent-id")
expect_contains "$RESPONSE" "not found" "non-existent returns 404"

###############################################################################
section "Admin: Delete Token"
###############################################################################

# Create a user and token to delete
RESPONSE=$(admin_curl -X POST -H "Content-Type: application/json" \
    -d '{"username":"token-delete-test"}' \
    "$ADMIN_API/users")

DELETE_USER_ID=$(get_id "$RESPONSE")

# Create token for user
RESPONSE=$(admin_curl -X POST -H "Content-Type: application/json" \
    -d '{"name":"to-delete"}' \
    "$ADMIN_API/users/$DELETE_USER_ID/tokens")

DELETE_TOKEN_ID=$(get_id "$RESPONSE")

# Delete it
admin_curl -X DELETE "$ADMIN_API/tokens/$DELETE_TOKEN_ID" > /dev/null
pass "token deleted"

# Verify it's gone from list
RESPONSE=$(admin_curl "$ADMIN_API/tokens")
expect_not_contains "$RESPONSE" "$DELETE_TOKEN_ID" "token no longer in list"

# Delete non-existent token
RESPONSE=$(admin_curl -X DELETE "$ADMIN_API/tokens/nonexistent-id")
expect_contains "$RESPONSE" "not found" "non-existent returns 404"

# Clean up user
admin_curl -X DELETE "$ADMIN_API/users/$DELETE_USER_ID" > /dev/null 2>&1

###############################################################################
section "Token Expiration"
###############################################################################

# Create user for expiration test
RESPONSE=$(admin_curl -X POST -H "Content-Type: application/json" \
    -d '{"username":"token-expire-test"}' \
    "$ADMIN_API/users")

EXPIRE_USER_ID=$(get_id "$RESPONSE")

# Create short-lived token
RESPONSE=$(admin_curl -X POST -H "Content-Type: application/json" \
    -d '{"name":"expire-soon","expires_in_seconds":1}' \
    "$ADMIN_API/users/$EXPIRE_USER_ID/tokens")

EXP_TOKEN=$(echo "$RESPONSE" | jq -r '.data.token')
EXP_TOKEN_ID=$(get_id "$RESPONSE")
if [ -n "$EXP_TOKEN_ID" ]; then
    track_token "$EXP_TOKEN_ID"
fi

sleep 2

RESPONSE=$(auth_curl_with "$EXP_TOKEN" "$API/repos")
expect_contains "$RESPONSE" "Token expired" "expired token rejected"

# Clean up user
admin_curl -X DELETE "$ADMIN_API/users/$EXPIRE_USER_ID" > /dev/null 2>&1

###############################################################################
section "Admin Token Enforcement"
###############################################################################

# User token cannot access admin token endpoints
RESPONSE=$(auth_curl "$ADMIN_API/tokens")
expect_contains "$RESPONSE" "Admin access required" "user cannot list admin tokens"

###############################################################################
summary
