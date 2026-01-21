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
section "Admin: Create User Token"
###############################################################################

# Create read-only token
RESPONSE=$(admin_curl -X POST -H "Content-Type: application/json" \
    -d "{\"namespace_id\":\"$NS_ID\",\"name\":\"test-readonly\",\"scope\":\"read-only\"}" \
    "$ADMIN_API/tokens")

TOKEN1_ID=$(get_id "$RESPONSE")
if [ -n "$TOKEN1_ID" ]; then
    track_token "$TOKEN1_ID"
fi
expect_contains "$RESPONSE" '"token":"eph_' "returns token value"
expect_json "$RESPONSE" '.data.scope' "read-only" "scope is read-only"
expect_json "$RESPONSE" '.data.is_admin' "false" "is not admin"

# Create repos scope token
RESPONSE=$(admin_curl -X POST -H "Content-Type: application/json" \
    -d "{\"namespace_id\":\"$NS_ID\",\"name\":\"test-repos\",\"scope\":\"repos\"}" \
    "$ADMIN_API/tokens")

TOKEN2_ID=$(get_id "$RESPONSE")
if [ -n "$TOKEN2_ID" ]; then
    track_token "$TOKEN2_ID"
fi
expect_json "$RESPONSE" '.data.scope' "repos" "scope is repos"

# Create full scope token
RESPONSE=$(admin_curl -X POST -H "Content-Type: application/json" \
    -d "{\"namespace_id\":\"$NS_ID\",\"name\":\"test-full\",\"scope\":\"full\"}" \
    "$ADMIN_API/tokens")

FULL_TOKEN=$(echo "$RESPONSE" | jq -r '.data.token')
FULL_TOKEN_ID=$(get_id "$RESPONSE")
if [ -n "$FULL_TOKEN_ID" ]; then
    track_token "$FULL_TOKEN_ID"
fi
expect_json "$RESPONSE" '.data.scope' "full" "scope is full"

# Create token with expiration
RESPONSE=$(admin_curl -X POST -H "Content-Type: application/json" \
    -d "{\"namespace_id\":\"$NS_ID\",\"name\":\"test-expiring\",\"scope\":\"read-only\",\"expires_in_seconds\":3600}" \
    "$ADMIN_API/tokens")

TOKEN3_ID=$(get_id "$RESPONSE")
if [ -n "$TOKEN3_ID" ]; then
    track_token "$TOKEN3_ID"
fi
expect_contains "$RESPONSE" '"expires_at"' "has expiration"

###############################################################################
section "Admin: Token Validation"
###############################################################################

# Invalid scope should fail
RESPONSE=$(admin_curl -X POST -H "Content-Type: application/json" \
    -d "{\"namespace_id\":\"$NS_ID\",\"name\":\"bad-scope\",\"scope\":\"invalid\"}" \
    "$ADMIN_API/tokens")

expect_contains "$RESPONSE" "Invalid scope" "invalid scope rejected"

# Negative expiration should fail
RESPONSE=$(admin_curl -X POST -H "Content-Type: application/json" \
    -d "{\"namespace_id\":\"$NS_ID\",\"name\":\"bad-expiry\",\"scope\":\"read-only\",\"expires_in_seconds\":-10}" \
    "$ADMIN_API/tokens")

expect_contains "$RESPONSE" "cannot be negative" "negative expiration rejected"

# User token without namespace should fail
RESPONSE=$(admin_curl -X POST -H "Content-Type: application/json" \
    -d '{"name":"no-namespace","scope":"full"}' \
    "$ADMIN_API/tokens")

expect_contains "$RESPONSE" "namespace_id" "user token requires namespace"

# Admin token with namespace should fail
RESPONSE=$(admin_curl -X POST -H "Content-Type: application/json" \
    -d "{\"namespace_id\":\"$NS_ID\",\"name\":\"admin-with-ns\",\"is_admin\":true}" \
    "$ADMIN_API/tokens")

expect_contains "$RESPONSE" "cannot have a namespace_id" "admin token rejects namespace"

###############################################################################
section "Admin: Create Admin Token"
###############################################################################

# Create admin token
RESPONSE=$(admin_curl -X POST -H "Content-Type: application/json" \
    -d '{"name":"test-admin","is_admin":true}' \
    "$ADMIN_API/tokens")

ADMIN_TOKEN2_ID=$(get_id "$RESPONSE")
if [ -n "$ADMIN_TOKEN2_ID" ]; then
    track_token "$ADMIN_TOKEN2_ID"
fi
expect_json "$RESPONSE" '.data.is_admin' "true" "is admin token"
expect_json "$RESPONSE" '.data.scope' "full" "admin has full scope"

###############################################################################
section "Scope Restrictions"
###############################################################################

# Create a repos-scoped token to test with
RESPONSE=$(admin_curl -X POST -H "Content-Type: application/json" \
    -d "{\"namespace_id\":\"$NS_ID\",\"name\":\"test-repos-scope\",\"scope\":\"repos\"}" \
    "$ADMIN_API/tokens")

REPOS_TOKEN=$(echo "$RESPONSE" | jq -r '.data.token')
REPOS_TOKEN_ID=$(get_id "$RESPONSE")
if [ -n "$REPOS_TOKEN_ID" ]; then
    track_token "$REPOS_TOKEN_ID"
fi

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
RESPONSE=$(admin_curl -X POST -H "Content-Type: application/json" \
    -d "{\"namespace_id\":\"$NS_ID\",\"name\":\"test-readonly-scope\",\"scope\":\"read-only\"}" \
    "$ADMIN_API/tokens")

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

###############################################################################
section "Admin: Delete Token"
###############################################################################

# Create a token to delete
RESPONSE=$(admin_curl -X POST -H "Content-Type: application/json" \
    -d "{\"namespace_id\":\"$NS_ID\",\"name\":\"to-delete\",\"scope\":\"read-only\"}" \
    "$ADMIN_API/tokens")

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

###############################################################################
section "Token Expiration"
###############################################################################

RESPONSE=$(admin_curl -X POST -H "Content-Type: application/json" \
    -d "{\"namespace_id\":\"$NS_ID\",\"name\":\"expire-soon\",\"scope\":\"read-only\",\"expires_in_seconds\":1}" \
    "$ADMIN_API/tokens")

EXP_TOKEN=$(echo "$RESPONSE" | jq -r '.data.token')
EXP_TOKEN_ID=$(get_id "$RESPONSE")
if [ -n "$EXP_TOKEN_ID" ]; then
    track_token "$EXP_TOKEN_ID"
fi

sleep 2

RESPONSE=$(auth_curl_with "$EXP_TOKEN" "$API/repos")
expect_contains "$RESPONSE" "Token expired" "expired token rejected"

###############################################################################
section "Admin Token Enforcement"
###############################################################################

# User token cannot access admin token endpoints
RESPONSE=$(auth_curl "$ADMIN_API/tokens")
expect_contains "$RESPONSE" "Admin access required" "user cannot list admin tokens"

RESPONSE=$(auth_curl -X POST \
    -H "Content-Type: application/json" \
    -d "{\"namespace_id\":\"$NS_ID\",\"name\":\"should-fail\"}" \
    "$ADMIN_API/tokens")
expect_contains "$RESPONSE" "Admin access required" "user cannot create tokens via admin API"

###############################################################################
summary
