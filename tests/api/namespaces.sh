#!/bin/bash
# Namespaces API Tests
set -e

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
source "$SCRIPT_DIR/lib.sh"

require_token
trap cleanup EXIT

echo ""
echo -e "${BLUE}═══════════════════════════════════════${NC}"
echo -e "${BLUE}  Namespaces API Tests${NC}"
echo -e "${BLUE}═══════════════════════════════════════${NC}"

###############################################################################
section "List"
###############################################################################

RESPONSE=$(auth_curl "$API/namespaces")
expect_contains "$RESPONSE" '"data"' "returns data array"
expect_contains "$RESPONSE" '"default"' "contains default namespace"

###############################################################################
section "Create"
###############################################################################

# Create a namespace
RESPONSE=$(auth_curl -X POST -H "Content-Type: application/json" \
    -d '{"name":"test-namespace"}' \
    "$API/namespaces")

NS_ID=$(get_id "$RESPONSE")
if [ -n "$NS_ID" ]; then
    track_namespace "$NS_ID"
    pass "create namespace"
else
    fail "create namespace" "valid ID" "$RESPONSE"
fi

expect_json "$RESPONSE" '.data.Name' "test-namespace" "name matches"

# Create with limits
RESPONSE=$(auth_curl -X POST -H "Content-Type: application/json" \
    -d '{"name":"test-ns-limits","repo_limit":10,"storage_limit_bytes":1073741824}' \
    "$API/namespaces")

NS2_ID=$(get_id "$RESPONSE")
if [ -n "$NS2_ID" ]; then
    track_namespace "$NS2_ID"
fi
expect_json "$RESPONSE" '.data.RepoLimit' "10" "repo_limit set"
expect_json "$RESPONSE" '.data.StorageLimitBytes' "1073741824" "storage_limit set"

# Duplicate name should fail
RESPONSE=$(auth_curl -X POST -H "Content-Type: application/json" \
    -d '{"name":"test-namespace"}' \
    "$API/namespaces")

expect_contains "$RESPONSE" "already exists" "duplicate name rejected"

# Empty name should fail
RESPONSE=$(auth_curl -X POST -H "Content-Type: application/json" \
    -d '{"name":""}' \
    "$API/namespaces")

expect_contains "$RESPONSE" "required" "empty name rejected"

###############################################################################
section "Get"
###############################################################################

# Get namespace by ID
RESPONSE=$(auth_curl "$API/namespaces/$NS_ID")
expect_json "$RESPONSE" '.data.ID' "$NS_ID" "returns correct namespace"
expect_json "$RESPONSE" '.data.Name' "test-namespace" "name matches"

# Get non-existent namespace
RESPONSE=$(auth_curl "$API/namespaces/nonexistent-id")
expect_contains "$RESPONSE" "not found" "non-existent returns 404"

###############################################################################
section "Delete"
###############################################################################

# Create a namespace to delete
RESPONSE=$(auth_curl -X POST -H "Content-Type: application/json" \
    -d '{"name":"test-ns-delete"}' \
    "$API/namespaces")

DELETE_NS_ID=$(get_id "$RESPONSE")

# Delete it
auth_curl -X DELETE "$API/namespaces/$DELETE_NS_ID" > /dev/null
pass "namespace deleted"

# Verify it's gone
RESPONSE=$(auth_curl "$API/namespaces/$DELETE_NS_ID")
expect_contains "$RESPONSE" "not found" "namespace no longer exists"

###############################################################################
section "Auth (Admin Required)"
###############################################################################

# Create a repos-scoped token
RESPONSE=$(auth_curl -X POST -H "Content-Type: application/json" \
    -d '{"name":"test-repos-ns","scope":"repos"}' \
    "$API/tokens")

REPOS_TOKEN=$(echo "$RESPONSE" | jq -r '.data.token')
REPOS_TOKEN_ID=$(get_id "$RESPONSE")
track_token "$REPOS_TOKEN_ID"

# repos scope cannot list namespaces
RESPONSE=$(auth_curl_with "$REPOS_TOKEN" "$API/namespaces")
expect_contains "$RESPONSE" "Admin access required\|Forbidden" "repos cannot list namespaces"

# repos scope cannot create namespaces
RESPONSE=$(auth_curl_with "$REPOS_TOKEN" -X POST \
    -H "Content-Type: application/json" \
    -d '{"name":"should-fail"}' \
    "$API/namespaces")
expect_contains "$RESPONSE" "Admin access required\|Forbidden" "repos cannot create namespaces"

###############################################################################
summary
