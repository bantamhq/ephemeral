#!/bin/bash
# Tokens API Tests
set -e

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
source "$SCRIPT_DIR/lib.sh"

require_token
trap cleanup EXIT

echo ""
echo -e "${BLUE}═══════════════════════════════════════${NC}"
echo -e "${BLUE}  Tokens API Tests${NC}"
echo -e "${BLUE}═══════════════════════════════════════${NC}"

###############################################################################
section "List"
###############################################################################

RESPONSE=$(auth_curl "$API/tokens")
expect_contains "$RESPONSE" '"data"' "returns data array"

# Should contain at least the root token
expect_json_length "$RESPONSE" '.data' "1" "contains at least one token"

###############################################################################
section "Create"
###############################################################################

# Create read-only token
RESPONSE=$(auth_curl -X POST -H "Content-Type: application/json" \
    -d '{"name":"test-readonly","scope":"read-only"}' \
    "$API/tokens")

TOKEN1_ID=$(get_id "$RESPONSE")
if [ -n "$TOKEN1_ID" ]; then
    track_token "$TOKEN1_ID"
fi
expect_contains "$RESPONSE" '"token":"eph_' "returns token value"
expect_json "$RESPONSE" '.data.scope' "read-only" "scope is read-only"

# Create token with default scope
RESPONSE=$(auth_curl -X POST -H "Content-Type: application/json" \
    -d '{"name":"test-default-scope"}' \
    "$API/tokens")

TOKEN_DEFAULT_ID=$(get_id "$RESPONSE")
if [ -n "$TOKEN_DEFAULT_ID" ]; then
    track_token "$TOKEN_DEFAULT_ID"
fi
expect_json "$RESPONSE" '.data.scope' "read-only" "default scope is read-only"

# Create repos scope token
RESPONSE=$(auth_curl -X POST -H "Content-Type: application/json" \
    -d '{"name":"test-repos","scope":"repos"}' \
    "$API/tokens")

TOKEN2_ID=$(get_id "$RESPONSE")
if [ -n "$TOKEN2_ID" ]; then
    track_token "$TOKEN2_ID"
fi
expect_json "$RESPONSE" '.data.scope' "repos" "scope is repos"

# Create token with expiration
RESPONSE=$(auth_curl -X POST -H "Content-Type: application/json" \
    -d '{"name":"test-expiring","scope":"read-only","expires_in_seconds":3600}' \
    "$API/tokens")

TOKEN3_ID=$(get_id "$RESPONSE")
if [ -n "$TOKEN3_ID" ]; then
    track_token "$TOKEN3_ID"
fi
expect_contains "$RESPONSE" '"expires_at"' "has expiration"

# Invalid scope should fail
RESPONSE=$(auth_curl -X POST -H "Content-Type: application/json" \
    -d '{"name":"bad-scope","scope":"invalid"}' \
    "$API/tokens")

expect_contains "$RESPONSE" "Invalid scope" "invalid scope rejected"

# Negative expiration should fail
RESPONSE=$(auth_curl -X POST -H "Content-Type: application/json" \
    -d '{"name":"bad-expiry","scope":"read-only","expires_in_seconds":-10}' \
    "$API/tokens")

expect_contains "$RESPONSE" "cannot be negative" "negative expiration rejected"

###############################################################################
section "Scope Restrictions"
###############################################################################

# Create a repos-scoped token to test with
RESPONSE=$(auth_curl -X POST -H "Content-Type: application/json" \
    -d '{"name":"test-repos-scope","scope":"repos"}' \
    "$API/tokens")

REPOS_TOKEN=$(echo "$RESPONSE" | jq -r '.data.token')
REPOS_TOKEN_ID=$(get_id "$RESPONSE")
if [ -n "$REPOS_TOKEN_ID" ]; then
    track_token "$REPOS_TOKEN_ID"
fi

# repos scope cannot create tokens
RESPONSE=$(auth_curl_with "$REPOS_TOKEN" -X POST \
    -H "Content-Type: application/json" \
    -d '{"name":"should-fail","scope":"read-only"}' \
    "$API/tokens")

expect_contains "$RESPONSE" "Insufficient permissions\|Forbidden" "repos cannot create tokens"

# repos scope cannot list tokens
RESPONSE=$(auth_curl_with "$REPOS_TOKEN" "$API/tokens")
expect_contains "$RESPONSE" "Insufficient permissions\|Forbidden" "repos cannot list tokens"

# repos scope can create repos
RESPONSE=$(auth_curl_with "$REPOS_TOKEN" -X POST \
    -H "Content-Type: application/json" \
    -d '{"name":"repos-scope-created","public":false}' \
    "$API/repos")

SCOPE_REPO_ID=$(get_id "$RESPONSE")
if [ -n "$SCOPE_REPO_ID" ]; then
    track_repo "$SCOPE_REPO_ID"
    pass "repos scope can create repos"
else
    fail "repos scope can create repos" "success" "$RESPONSE"
fi

# Create a read-only token
RESPONSE=$(auth_curl -X POST -H "Content-Type: application/json" \
    -d '{"name":"test-readonly-scope","scope":"read-only"}' \
    "$API/tokens")

RO_TOKEN=$(echo "$RESPONSE" | jq -r '.data.token')
RO_TOKEN_ID=$(get_id "$RESPONSE")
if [ -n "$RO_TOKEN_ID" ]; then
    track_token "$RO_TOKEN_ID"
fi

# read-only cannot create repos
RESPONSE=$(auth_curl_with "$RO_TOKEN" -X POST \
    -H "Content-Type: application/json" \
    -d '{"name":"should-fail","public":false}' \
    "$API/repos")

expect_contains "$RESPONSE" "Insufficient permissions\|Forbidden" "read-only cannot create repos"

# read-only can list repos
RESPONSE=$(auth_curl_with "$RO_TOKEN" "$API/repos")
expect_contains "$RESPONSE" '"data"' "read-only can list repos"

# Create full-scope token
RESPONSE=$(auth_curl -X POST -H "Content-Type: application/json" \
    -d '{"name":"test-full-scope","scope":"full"}' \
    "$API/tokens")

FULL_TOKEN=$(echo "$RESPONSE" | jq -r '.data.token')
FULL_TOKEN_ID=$(get_id "$RESPONSE")
if [ -n "$FULL_TOKEN_ID" ]; then
    track_token "$FULL_TOKEN_ID"
fi

# full scope cannot create admin tokens
RESPONSE=$(auth_curl_with "$FULL_TOKEN" -X POST \
    -H "Content-Type: application/json" \
    -d '{"name":"should-fail","scope":"admin"}' \
    "$API/tokens")

expect_contains "$RESPONSE" "Cannot create token with higher scope" "full cannot create admin token"

###############################################################################
section "Delete"
###############################################################################

# Create a token to delete
RESPONSE=$(auth_curl -X POST -H "Content-Type: application/json" \
    -d '{"name":"to-delete","scope":"read-only"}' \
    "$API/tokens")

DELETE_TOKEN_ID=$(get_id "$RESPONSE")

# Delete it
auth_curl -X DELETE "$API/tokens/$DELETE_TOKEN_ID" > /dev/null
pass "token deleted"

# Verify it's gone from list
RESPONSE=$(auth_curl "$API/tokens")
expect_not_contains "$RESPONSE" "$DELETE_TOKEN_ID" "token no longer in list"

# Delete non-existent token
RESPONSE=$(auth_curl -X DELETE "$API/tokens/nonexistent-id")
expect_contains "$RESPONSE" "not found" "non-existent returns 404"

###############################################################################
section "Self-Delete Protection"
###############################################################################

# Create a token and use it to try to delete itself
RESPONSE=$(auth_curl -X POST -H "Content-Type: application/json" \
    -d '{"name":"self-delete-test","scope":"full"}' \
    "$API/tokens")

SELF_TOKEN=$(echo "$RESPONSE" | jq -r '.data.token')
SELF_TOKEN_ID=$(get_id "$RESPONSE")
track_token "$SELF_TOKEN_ID"

# Try to delete self
RESPONSE=$(auth_curl_with "$SELF_TOKEN" -X DELETE "$API/tokens/$SELF_TOKEN_ID")
expect_contains "$RESPONSE" "Cannot delete current token" "self-delete prevented"

###############################################################################
section "Expiration"
###############################################################################

RESPONSE=$(auth_curl -X POST -H "Content-Type: application/json" \
    -d '{"name":"expire-soon","scope":"read-only","expires_in_seconds":1}' \
    "$API/tokens")

EXP_TOKEN=$(echo "$RESPONSE" | jq -r '.data.token')
EXP_TOKEN_ID=$(get_id "$RESPONSE")
if [ -n "$EXP_TOKEN_ID" ]; then
    track_token "$EXP_TOKEN_ID"
fi

sleep 2

RESPONSE=$(auth_curl_with "$EXP_TOKEN" "$API/repos")
expect_contains "$RESPONSE" "Token expired" "expired token rejected"

###############################################################################
summary
