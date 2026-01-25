#!/bin/bash
# Auth API Tests
set -e

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
source "$SCRIPT_DIR/lib.sh"

require_admin_token

echo ""
echo -e "${BLUE}═══════════════════════════════════════${NC}"
echo -e "${BLUE}  Auth API Tests${NC}"
echo -e "${BLUE}═══════════════════════════════════════${NC}"

###############################################################################
section "Auth Discovery (/.well-known/ephemeral-auth)"
###############################################################################

RESPONSE=$(anon_curl "$BASE_URL/.well-known/ephemeral-auth")
expect_contains "$RESPONSE" '"auth_method"' "discovery returns auth_method"
expect_contains "$RESPONSE" '"token"' "standalone server returns token method"
expect_not_contains "$RESPONSE" '"server_url"' "standalone server has no server_url"
expect_not_contains "$RESPONSE" '"auth_endpoint"' "standalone server has no auth_endpoint"

###############################################################################
section "Auth Sessions - Create"
###############################################################################

RESPONSE=$(anon_curl -X POST \
    -H "Content-Type: application/json" \
    -d '{"expires_in_seconds":300}' \
    "$API/auth/sessions")

SESSION_ID=$(echo "$RESPONSE" | jq -r '.data.id')
if [ -n "$SESSION_ID" ] && [ "$SESSION_ID" != "null" ]; then
    pass "create auth session returns ID"
else
    fail "create auth session returns ID" "non-empty ID" "$RESPONSE"
fi

expect_json "$RESPONSE" '.data.status' "pending" "session status is pending"
expect_contains "$RESPONSE" '"expires_at"' "session has expiration"

# Default expiration (no expires_in_seconds)
RESPONSE=$(anon_curl -X POST \
    -H "Content-Type: application/json" \
    -d '{}' \
    "$API/auth/sessions")

expect_json "$RESPONSE" '.data.status' "pending" "default expiration works"

# Capped expiration (>600 seconds)
RESPONSE=$(anon_curl -X POST \
    -H "Content-Type: application/json" \
    -d '{"expires_in_seconds":9999}' \
    "$API/auth/sessions")

expect_json "$RESPONSE" '.data.status' "pending" "over-max expiration capped"

###############################################################################
section "Auth Sessions - Get"
###############################################################################

# Get pending session
RESPONSE=$(anon_curl "$API/auth/sessions/$SESSION_ID")
expect_json "$RESPONSE" '.data.id' "$SESSION_ID" "get session returns correct ID"
expect_json "$RESPONSE" '.data.status' "pending" "session still pending"
expect_not_contains "$RESPONSE" '"token":' "pending session has no token"

# Non-existent session
RESPONSE=$(anon_curl "$API/auth/sessions/nonexistent-id")
expect_contains "$RESPONSE" "not found" "non-existent session returns 404"

###############################################################################
section "Auth Sessions - Complete (Admin)"
###############################################################################

# Create a user to complete the session with
RESPONSE=$(admin_curl -X POST -H "Content-Type: application/json" \
    -d '{"username":"auth-test-user"}' \
    "$ADMIN_API/users")

USER_ID=$(echo "$RESPONSE" | jq -r '.data.id')
if [ -z "$USER_ID" ] || [ "$USER_ID" = "null" ]; then
    echo "Failed to create test user: $RESPONSE"
    exit 1
fi

# Create a fresh session to complete
RESPONSE=$(anon_curl -X POST \
    -H "Content-Type: application/json" \
    -d '{"expires_in_seconds":300}' \
    "$API/auth/sessions")

COMPLETE_SESSION_ID=$(echo "$RESPONSE" | jq -r '.data.id')

# Complete the session
RESPONSE=$(admin_curl -X POST -H "Content-Type: application/json" \
    -d "{\"user_id\":\"$USER_ID\"}" \
    "$ADMIN_API/auth/sessions/$COMPLETE_SESSION_ID/complete")

expect_json "$RESPONSE" '.data.status' "completed" "session completed"

# Get completed session (should return token and delete)
RESPONSE=$(anon_curl "$API/auth/sessions/$COMPLETE_SESSION_ID")
expect_json "$RESPONSE" '.data.status' "completed" "get completed session"
expect_contains "$RESPONSE" '"token":"eph_' "completed session has token"

# Session should be deleted after retrieval
RESPONSE=$(anon_curl "$API/auth/sessions/$COMPLETE_SESSION_ID")
expect_contains "$RESPONSE" "not found" "session deleted after token retrieval"

# Complete non-existent session
RESPONSE=$(admin_curl -X POST -H "Content-Type: application/json" \
    -d "{\"user_id\":\"$USER_ID\"}" \
    "$ADMIN_API/auth/sessions/nonexistent/complete")

expect_contains "$RESPONSE" "not found" "complete non-existent fails"

# Complete without user_id
RESPONSE=$(admin_curl -X POST -H "Content-Type: application/json" \
    -d '{}' \
    "$ADMIN_API/auth/sessions/$SESSION_ID/complete")

expect_contains "$RESPONSE" "user_id" "complete requires user_id"

# Complete with non-existent user
RESPONSE=$(admin_curl -X POST -H "Content-Type: application/json" \
    -d '{"user_id":"nonexistent-user"}' \
    "$ADMIN_API/auth/sessions/$SESSION_ID/complete")

expect_contains "$RESPONSE" "not found" "complete with bad user fails"

# Clean up test user
admin_curl -X DELETE "$ADMIN_API/users/$USER_ID" > /dev/null 2>&1

###############################################################################
section "Auth Sessions - Expiration"
###############################################################################

# Create short-lived session
RESPONSE=$(anon_curl -X POST \
    -H "Content-Type: application/json" \
    -d '{"expires_in_seconds":1}' \
    "$API/auth/sessions")

SHORT_SESSION_ID=$(echo "$RESPONSE" | jq -r '.data.id')

sleep 2

RESPONSE=$(anon_curl "$API/auth/sessions/$SHORT_SESSION_ID")
expect_contains "$RESPONSE" "expired\|not found" "expired session rejected"

###############################################################################
summary
