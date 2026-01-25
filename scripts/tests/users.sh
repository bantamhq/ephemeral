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

# First create a namespace for the user
RESPONSE=$(admin_curl -X POST -H "Content-Type: application/json" \
    -d '{"name":"test-user-ns-1"}' \
    "$ADMIN_API/namespaces")

USER_NS_ID=$(get_id "$RESPONSE")
if [ -n "$USER_NS_ID" ]; then
    track_namespace "$USER_NS_ID"
fi

RESPONSE=$(admin_curl -X POST -H "Content-Type: application/json" \
    -d "{\"namespace_id\":\"$USER_NS_ID\"}" \
    "$ADMIN_API/users")

USER1_ID=$(get_id "$RESPONSE")
if [ -n "$USER1_ID" ]; then
    pass "create user"
else
    fail "create user" "valid ID" "$RESPONSE"
fi

expect_json "$RESPONSE" '.data.primary_namespace_id' "$USER_NS_ID" "primary_namespace_id matches"

# Empty namespace_id should fail
RESPONSE=$(admin_curl -X POST -H "Content-Type: application/json" \
    -d '{"namespace_id":""}' \
    "$ADMIN_API/users")

expect_contains "$RESPONSE" "required" "empty namespace_id rejected"

###############################################################################
section "Admin: Get User"
###############################################################################

RESPONSE=$(admin_curl "$ADMIN_API/users/$USER1_ID")
expect_json "$RESPONSE" '.data.id' "$USER1_ID" "returns correct user"
expect_json "$RESPONSE" '.data.primary_namespace_id' "$USER_NS_ID" "primary_namespace_id matches"

# Get non-existent user
RESPONSE=$(admin_curl "$ADMIN_API/users/nonexistent-id")
expect_contains "$RESPONSE" "not found" "non-existent returns 404"

###############################################################################
section "Admin: Delete User"
###############################################################################

# Create namespace for user to delete
RESPONSE=$(admin_curl -X POST -H "Content-Type: application/json" \
    -d '{"name":"test-user-ns-delete"}' \
    "$ADMIN_API/namespaces")

DELETE_USER_NS_ID=$(get_id "$RESPONSE")

# Create user to delete
RESPONSE=$(admin_curl -X POST -H "Content-Type: application/json" \
    -d "{\"namespace_id\":\"$DELETE_USER_NS_ID\"}" \
    "$ADMIN_API/users")

DELETE_USER_ID=$(get_id "$RESPONSE")

# Delete it
admin_curl -X DELETE "$ADMIN_API/users/$DELETE_USER_ID" > /dev/null
pass "user deleted"

# Verify it's gone
RESPONSE=$(admin_curl "$ADMIN_API/users/$DELETE_USER_ID")
expect_contains "$RESPONSE" "not found" "user no longer exists"

# Clean up delete user namespace
admin_curl -X DELETE "$ADMIN_API/namespaces/test-user-ns-delete" > /dev/null 2>&1

###############################################################################
section "Admin: User Tokens"
###############################################################################

# List tokens for user (returns array, not object with data field)
RESPONSE=$(admin_curl "$ADMIN_API/users/$USER1_ID/tokens")
# Initial list may be empty, just check it's a valid array
TOKEN_COUNT=$(echo "$RESPONSE" | jq -r 'if .data then .data | length else 0 end')
pass "can list user tokens"

# Create token for user (response: {"token": "...", "metadata": {...}})
RESPONSE=$(admin_curl -X POST -H "Content-Type: application/json" \
    -d '{}' \
    "$ADMIN_API/users/$USER1_ID/tokens")

TOKEN1_ID=$(echo "$RESPONSE" | jq -r '.data.metadata.id')
USER1_TOKEN=$(echo "$RESPONSE" | jq -r '.data.token')
if [ -n "$TOKEN1_ID" ] && [ "$TOKEN1_ID" != "null" ]; then
    track_token "$TOKEN1_ID"
    pass "create user token"
else
    fail "create user token" "valid ID" "$RESPONSE"
fi

expect_contains "$RESPONSE" '"token":"eph_' "returns token value"

# Create token with expiration
RESPONSE=$(admin_curl -X POST -H "Content-Type: application/json" \
    -d '{"expires_in_seconds":3600}' \
    "$ADMIN_API/users/$USER1_ID/tokens")

TOKEN2_ID=$(echo "$RESPONSE" | jq -r '.data.metadata.id // .metadata.id // empty')
if [ -n "$TOKEN2_ID" ]; then
    track_token "$TOKEN2_ID"
fi
expect_contains "$RESPONSE" '"expires_at"' "token has expiration"

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

# List namespace grants (returns array; user already has grant on their primary namespace)
RESPONSE=$(admin_curl "$ADMIN_API/users/$USER1_ID/namespace-grants")
# Response is an array, check it's valid JSON array
GRANT_COUNT=$(echo "$RESPONSE" | jq -r 'if .data then .data | length else (. | length) end')
pass "can list namespace grants"

# Create namespace grant
RESPONSE=$(admin_curl -X POST -H "Content-Type: application/json" \
    -d "{\"namespace_id\":\"$GRANT_NS_ID\",\"allow\":[\"namespace:read\",\"repo:read\"]}" \
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
    -d "{\"namespace_id\":\"$GRANT_NS2_ID\",\"allow\":[\"repo:admin\"]}" \
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

# Use the token we created earlier (USER1_TOKEN from Admin: User Tokens section)
# If we don't have it, create a new one
if [ -z "$USER1_TOKEN" ] || [ "$USER1_TOKEN" = "null" ]; then
    RESPONSE=$(admin_curl -X POST -H "Content-Type: application/json" \
        -d '{}' \
        "$ADMIN_API/users/$USER1_ID/tokens")
    USER1_TOKEN=$(echo "$RESPONSE" | jq -r '.data.token')
    TOKEN_ID=$(echo "$RESPONSE" | jq -r '.data.metadata.id')
    if [ -n "$TOKEN_ID" ] && [ "$TOKEN_ID" != "null" ]; then
        track_token "$TOKEN_ID"
    fi
fi

# Create a repo using the user's token
RESPONSE=$(auth_curl_with "$USER1_TOKEN" -X POST -H "Content-Type: application/json" \
    -d '{"name":"grant-test-repo","public":false}' \
    "$API/repos")

GRANT_REPO_ID=$(get_id "$RESPONSE")
if [ -n "$GRANT_REPO_ID" ]; then
    info "Created repo: $GRANT_REPO_ID"
fi

# Create second namespace and user for repo grant tests
RESPONSE=$(admin_curl -X POST -H "Content-Type: application/json" \
    -d '{"name":"test-user-ns-2"}' \
    "$ADMIN_API/namespaces")

USER2_NS_ID=$(get_id "$RESPONSE")
if [ -n "$USER2_NS_ID" ]; then
    track_namespace "$USER2_NS_ID"
fi

RESPONSE=$(admin_curl -X POST -H "Content-Type: application/json" \
    -d "{\"namespace_id\":\"$USER2_NS_ID\"}" \
    "$ADMIN_API/users")

USER2_ID=$(get_id "$RESPONSE")

# Give user2 access to user1's namespace first
RESPONSE=$(admin_curl -X POST -H "Content-Type: application/json" \
    -d "{\"namespace_id\":\"$GRANT_NS_ID\",\"allow\":[\"namespace:read\"]}" \
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
auth_curl_with "$USER1_TOKEN" -X DELETE "$API/repos/$GRANT_REPO_ID" > /dev/null 2>&1

# Clean up user2
admin_curl -X DELETE "$ADMIN_API/users/$USER2_ID" > /dev/null 2>&1

###############################################################################
section "Permission Restrictions"
###############################################################################

# Create a token for permission tests
RESPONSE=$(admin_curl -X POST -H "Content-Type: application/json" \
    -d '{}' \
    "$ADMIN_API/users/$USER1_ID/tokens")

PERM_TOKEN=$(echo "$RESPONSE" | jq -r '.data.token')
PERM_TOKEN_ID=$(echo "$RESPONSE" | jq -r '.data.metadata.id // .metadata.id // empty')
if [ -n "$PERM_TOKEN_ID" ]; then
    track_token "$PERM_TOKEN_ID"
fi

# Token can list repos
RESPONSE=$(auth_curl_with "$PERM_TOKEN" "$API/repos")
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
