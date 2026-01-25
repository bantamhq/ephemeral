#!/bin/bash
# Users API Tests (Admin)
set -e

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
source "$SCRIPT_DIR/lib.sh"

require_token
require_admin_token
trap cleanup EXIT

echo ""
echo -e "${BLUE}═══════════════════════════════════════${NC}"
echo -e "${BLUE}  Users API Tests (Admin)${NC}"
echo -e "${BLUE}═══════════════════════════════════════${NC}"

###############################################################################
section "Admin: List Users"
###############################################################################

RESPONSE=$(admin_curl "$ADMIN_API/users")
expect_contains "$RESPONSE" '"data"' "returns data array"

###############################################################################
section "Admin: Create User"
###############################################################################

RESPONSE=$(admin_curl -X POST -H "Content-Type: application/json" \
    -d '{"username":"test-user-1"}' \
    "$ADMIN_API/users")

USER1_ID=$(get_id "$RESPONSE")
if [ -n "$USER1_ID" ]; then
    pass "create user"
else
    fail "create user" "valid ID" "$RESPONSE"
fi

expect_json "$RESPONSE" '.data.username' "test-user-1" "username matches"

# Duplicate username should fail
RESPONSE=$(admin_curl -X POST -H "Content-Type: application/json" \
    -d '{"username":"test-user-1"}' \
    "$ADMIN_API/users")

expect_contains "$RESPONSE" "already exists" "duplicate username rejected"

# Empty username should fail
RESPONSE=$(admin_curl -X POST -H "Content-Type: application/json" \
    -d '{"username":""}' \
    "$ADMIN_API/users")

expect_contains "$RESPONSE" "required" "empty username rejected"

###############################################################################
section "Admin: Get User"
###############################################################################

RESPONSE=$(admin_curl "$ADMIN_API/users/$USER1_ID")
expect_json "$RESPONSE" '.data.id' "$USER1_ID" "returns correct user"
expect_json "$RESPONSE" '.data.username' "test-user-1" "username matches"

# Get non-existent user
RESPONSE=$(admin_curl "$ADMIN_API/users/nonexistent-id")
expect_contains "$RESPONSE" "not found" "non-existent returns 404"

###############################################################################
section "Admin: Delete User"
###############################################################################

# Create user to delete
RESPONSE=$(admin_curl -X POST -H "Content-Type: application/json" \
    -d '{"username":"test-user-delete"}' \
    "$ADMIN_API/users")

DELETE_USER_ID=$(get_id "$RESPONSE")

# Delete it
admin_curl -X DELETE "$ADMIN_API/users/$DELETE_USER_ID" > /dev/null
pass "user deleted"

# Verify it's gone
RESPONSE=$(admin_curl "$ADMIN_API/users/$DELETE_USER_ID")
expect_contains "$RESPONSE" "not found" "user no longer exists"

###############################################################################
section "Admin: User Tokens"
###############################################################################

# List tokens for user (initially empty or minimal)
RESPONSE=$(admin_curl "$ADMIN_API/users/$USER1_ID/tokens")
expect_contains "$RESPONSE" '"data"' "can list user tokens"

# Create token for user
RESPONSE=$(admin_curl -X POST -H "Content-Type: application/json" \
    -d '{"name":"test-token-1"}' \
    "$ADMIN_API/users/$USER1_ID/tokens")

TOKEN1_ID=$(get_id "$RESPONSE")
if [ -n "$TOKEN1_ID" ]; then
    track_token "$TOKEN1_ID"
    pass "create user token"
else
    fail "create user token" "valid ID" "$RESPONSE"
fi

expect_contains "$RESPONSE" '"token":"eph_' "returns token value"
expect_json "$RESPONSE" '.data.name' "test-token-1" "token name matches"

# Create token with expiration
RESPONSE=$(admin_curl -X POST -H "Content-Type: application/json" \
    -d '{"name":"test-token-expiring","expires_in_seconds":3600}' \
    "$ADMIN_API/users/$USER1_ID/tokens")

TOKEN2_ID=$(get_id "$RESPONSE")
if [ -n "$TOKEN2_ID" ]; then
    track_token "$TOKEN2_ID"
fi
expect_contains "$RESPONSE" '"expires_at"' "token has expiration"

# Verify tokens in list
RESPONSE=$(admin_curl "$ADMIN_API/users/$USER1_ID/tokens")
expect_contains "$RESPONSE" '"test-token-1"' "token in list"

###############################################################################
section "Admin: Namespace Grants"
###############################################################################

# Create a namespace for grant tests
RESPONSE=$(admin_curl -X POST -H "Content-Type: application/json" \
    -d '{"name":"grant-test-ns"}' \
    "$ADMIN_API/namespaces")

GRANT_NS_ID=$(get_id "$RESPONSE")
if [ -n "$GRANT_NS_ID" ]; then
    track_namespace "$GRANT_NS_ID"
fi

# List namespace grants (initially empty)
RESPONSE=$(admin_curl "$ADMIN_API/users/$USER1_ID/namespace-grants")
expect_contains "$RESPONSE" '"data"' "can list namespace grants"

# Create namespace grant
RESPONSE=$(admin_curl -X POST -H "Content-Type: application/json" \
    -d "{\"namespace_id\":\"$GRANT_NS_ID\",\"allow\":[\"namespace:read\",\"repo:read\"],\"is_primary\":true}" \
    "$ADMIN_API/users/$USER1_ID/namespace-grants")

expect_contains "$RESPONSE" "$GRANT_NS_ID" "grant created with namespace"
expect_contains "$RESPONSE" '"namespace:read"' "grant has namespace:read"
expect_contains "$RESPONSE" '"repo:read"' "grant has repo:read"

# List grants again
RESPONSE=$(admin_curl "$ADMIN_API/users/$USER1_ID/namespace-grants")
expect_contains "$RESPONSE" "$GRANT_NS_ID" "grant appears in list"

# Get specific grant
RESPONSE=$(admin_curl "$ADMIN_API/users/$USER1_ID/namespace-grants/$GRANT_NS_ID")
expect_contains "$RESPONSE" "$GRANT_NS_ID" "get specific grant"
expect_json "$RESPONSE" '.data.is_primary' "true" "grant is primary"

# Get non-existent grant
RESPONSE=$(admin_curl "$ADMIN_API/users/$USER1_ID/namespace-grants/nonexistent-ns")
expect_contains "$RESPONSE" "not found" "non-existent grant returns 404"

# Create second namespace for additional grant
RESPONSE=$(admin_curl -X POST -H "Content-Type: application/json" \
    -d '{"name":"grant-test-ns-2"}' \
    "$ADMIN_API/namespaces")

GRANT_NS2_ID=$(get_id "$RESPONSE")
if [ -n "$GRANT_NS2_ID" ]; then
    track_namespace "$GRANT_NS2_ID"
fi

# Add second grant
RESPONSE=$(admin_curl -X POST -H "Content-Type: application/json" \
    -d "{\"namespace_id\":\"$GRANT_NS2_ID\",\"allow\":[\"repo:admin\"],\"is_primary\":false}" \
    "$ADMIN_API/users/$USER1_ID/namespace-grants")

expect_contains "$RESPONSE" "$GRANT_NS2_ID" "second grant created"

# Delete namespace grant
admin_curl -X DELETE "$ADMIN_API/users/$USER1_ID/namespace-grants/$GRANT_NS2_ID" > /dev/null
pass "namespace grant deleted"

# Verify it's gone
RESPONSE=$(admin_curl "$ADMIN_API/users/$USER1_ID/namespace-grants/$GRANT_NS2_ID")
expect_contains "$RESPONSE" "not found" "grant no longer exists"

###############################################################################
section "Admin: Repo Grants"
###############################################################################

# First we need a repo - create via a token we generated
USER_TOKEN=$(admin_curl "$ADMIN_API/users/$USER1_ID/tokens" | jq -r '.data[0].token // empty')
if [ -z "$USER_TOKEN" ]; then
    # Create a token if none exists
    RESPONSE=$(admin_curl -X POST -H "Content-Type: application/json" \
        -d '{"name":"repo-grant-token"}' \
        "$ADMIN_API/users/$USER1_ID/tokens")
    USER_TOKEN=$(echo "$RESPONSE" | jq -r '.data.token')
    TOKEN_ID=$(get_id "$RESPONSE")
    track_token "$TOKEN_ID"
fi

# Create a repo using the user's token
RESPONSE=$(auth_curl_with "$USER_TOKEN" -X POST -H "Content-Type: application/json" \
    -d '{"name":"grant-test-repo","public":false}' \
    "$API/repos")

GRANT_REPO_ID=$(get_id "$RESPONSE")
if [ -n "$GRANT_REPO_ID" ]; then
    # Track for cleanup - but use user token
    info "Created repo: $GRANT_REPO_ID"
fi

# Create second user for repo grant tests
RESPONSE=$(admin_curl -X POST -H "Content-Type: application/json" \
    -d '{"username":"test-user-2"}' \
    "$ADMIN_API/users")

USER2_ID=$(get_id "$RESPONSE")

# Give user2 access to user1's namespace first
RESPONSE=$(admin_curl -X POST -H "Content-Type: application/json" \
    -d "{\"namespace_id\":\"$GRANT_NS_ID\",\"allow\":[\"namespace:read\"],\"is_primary\":true}" \
    "$ADMIN_API/users/$USER2_ID/namespace-grants")

# List repo grants (initially empty)
RESPONSE=$(admin_curl "$ADMIN_API/users/$USER2_ID/repo-grants")
expect_contains "$RESPONSE" '"data"' "can list repo grants"

# Create repo grant for user2 on user1's repo
RESPONSE=$(admin_curl -X POST -H "Content-Type: application/json" \
    -d "{\"repo_id\":\"$GRANT_REPO_ID\",\"allow\":[\"repo:read\"]}" \
    "$ADMIN_API/users/$USER2_ID/repo-grants")

expect_contains "$RESPONSE" "$GRANT_REPO_ID" "repo grant created"
expect_contains "$RESPONSE" '"repo:read"' "grant has repo:read"

# List repo grants again
RESPONSE=$(admin_curl "$ADMIN_API/users/$USER2_ID/repo-grants")
expect_contains "$RESPONSE" "$GRANT_REPO_ID" "repo grant in list"

# Get specific repo grant
RESPONSE=$(admin_curl "$ADMIN_API/users/$USER2_ID/repo-grants/$GRANT_REPO_ID")
expect_contains "$RESPONSE" "$GRANT_REPO_ID" "get specific repo grant"

# Get non-existent repo grant
RESPONSE=$(admin_curl "$ADMIN_API/users/$USER2_ID/repo-grants/nonexistent-repo")
expect_contains "$RESPONSE" "not found" "non-existent repo grant returns 404"

# Delete repo grant
admin_curl -X DELETE "$ADMIN_API/users/$USER2_ID/repo-grants/$GRANT_REPO_ID" > /dev/null
pass "repo grant deleted"

# Verify it's gone
RESPONSE=$(admin_curl "$ADMIN_API/users/$USER2_ID/repo-grants/$GRANT_REPO_ID")
expect_contains "$RESPONSE" "not found" "repo grant no longer exists"

# Clean up test repo
auth_curl_with "$USER_TOKEN" -X DELETE "$API/repos/$GRANT_REPO_ID" > /dev/null 2>&1

# Clean up user2
admin_curl -X DELETE "$ADMIN_API/users/$USER2_ID" > /dev/null 2>&1

###############################################################################
section "Permission Restrictions"
###############################################################################

# Create a read-only token for permission tests
RESPONSE=$(admin_curl -X POST -H "Content-Type: application/json" \
    -d '{"name":"test-readonly"}' \
    "$ADMIN_API/users/$USER1_ID/tokens")

RO_TOKEN=$(echo "$RESPONSE" | jq -r '.data.token')
RO_TOKEN_ID=$(get_id "$RESPONSE")
if [ -n "$RO_TOKEN_ID" ]; then
    track_token "$RO_TOKEN_ID"
fi

# Token can list repos
RESPONSE=$(auth_curl_with "$RO_TOKEN" "$API/repos")
expect_contains "$RESPONSE" '"data"' "token can list repos"

###############################################################################
section "Admin Token Enforcement"
###############################################################################

# User token cannot access admin user endpoints
RESPONSE=$(auth_curl "$ADMIN_API/users")
expect_contains "$RESPONSE" "Admin access required" "user cannot list admin users"

RESPONSE=$(auth_curl -X POST \
    -H "Content-Type: application/json" \
    -d '{"username":"should-fail"}' \
    "$ADMIN_API/users")
expect_contains "$RESPONSE" "Admin access required" "user cannot create users via admin API"

###############################################################################
section "Cleanup"
###############################################################################

# Clean up user1
admin_curl -X DELETE "$ADMIN_API/users/$USER1_ID" > /dev/null 2>&1
info "Cleaned up test user"

###############################################################################
summary
