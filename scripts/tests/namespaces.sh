#!/bin/bash
# Namespaces API Tests (Admin + User)
set -e

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
source "$SCRIPT_DIR/lib.sh"

require_token
require_admin_token
trap cleanup EXIT

echo ""
echo -e "${BLUE}═══════════════════════════════════════${NC}"
echo -e "${BLUE}  Namespaces API Tests${NC}"
echo -e "${BLUE}═══════════════════════════════════════${NC}"

###############################################################################
section "Admin: List Namespaces"
###############################################################################

RESPONSE=$(admin_curl "$ADMIN_API/namespaces")
expect_contains "$RESPONSE" '"data"' "admin can list namespaces"
expect_contains "$RESPONSE" '"test"' "contains test namespace"

###############################################################################
section "Admin: Create Namespace"
###############################################################################

# Create a namespace
RESPONSE=$(admin_curl -X POST -H "Content-Type: application/json" \
    -d '{"name":"test-namespace"}' \
    "$ADMIN_API/namespaces")

NS_ID=$(get_id "$RESPONSE")
if [ -n "$NS_ID" ]; then
    track_namespace "$NS_ID"
    pass "create namespace"
else
    fail "create namespace" "valid ID" "$RESPONSE"
fi

expect_json "$RESPONSE" '.data.name' "test-namespace" "name matches"

# Create with limits
RESPONSE=$(admin_curl -X POST -H "Content-Type: application/json" \
    -d '{"name":"test-ns-limits","repo_limit":10,"storage_limit_bytes":1073741824}' \
    "$ADMIN_API/namespaces")

NS2_ID=$(get_id "$RESPONSE")
if [ -n "$NS2_ID" ]; then
    track_namespace "$NS2_ID"
fi
expect_json "$RESPONSE" '.data.repo_limit' "10" "repo_limit set"
expect_json "$RESPONSE" '.data.storage_limit_bytes' "1073741824" "storage_limit set"

# Duplicate name should fail
RESPONSE=$(admin_curl -X POST -H "Content-Type: application/json" \
    -d '{"name":"test-namespace"}' \
    "$ADMIN_API/namespaces")

expect_contains "$RESPONSE" "already exists" "duplicate name rejected"

# Empty name should fail
RESPONSE=$(admin_curl -X POST -H "Content-Type: application/json" \
    -d '{"name":""}' \
    "$ADMIN_API/namespaces")

expect_contains "$RESPONSE" "required" "empty name rejected"

# Invalid name should fail
RESPONSE=$(admin_curl -X POST -H "Content-Type: application/json" \
    -d '{"name":"bad/name"}' \
    "$ADMIN_API/namespaces")

expect_contains "$RESPONSE" "path separators" "invalid name rejected"

###############################################################################
section "Admin: Get Namespace"
###############################################################################

# Get namespace by ID
RESPONSE=$(admin_curl "$ADMIN_API/namespaces/$NS_ID")
expect_json "$RESPONSE" '.data.id' "$NS_ID" "returns correct namespace"
expect_json "$RESPONSE" '.data.name' "test-namespace" "name matches"

# Get non-existent namespace
RESPONSE=$(admin_curl "$ADMIN_API/namespaces/nonexistent-id")
expect_contains "$RESPONSE" "not found" "non-existent returns 404"

###############################################################################
section "Admin: Delete Namespace"
###############################################################################

# Create a namespace to delete
RESPONSE=$(admin_curl -X POST -H "Content-Type: application/json" \
    -d '{"name":"test-ns-delete"}' \
    "$ADMIN_API/namespaces")

DELETE_NS_ID=$(get_id "$RESPONSE")

# Delete it
admin_curl -X DELETE "$ADMIN_API/namespaces/$DELETE_NS_ID" > /dev/null
pass "namespace deleted"

# Verify it's gone
RESPONSE=$(admin_curl "$ADMIN_API/namespaces/$DELETE_NS_ID")
expect_contains "$RESPONSE" "not found" "namespace no longer exists"

###############################################################################
section "User: List My Namespaces"
###############################################################################

RESPONSE=$(auth_curl "$API/namespaces")
expect_contains "$RESPONSE" '"data"' "user can list their namespaces"
expect_contains "$RESPONSE" '"is_primary"' "includes is_primary field"

###############################################################################
section "User: Current Namespace"
###############################################################################

RESPONSE=$(auth_curl "$API/namespace")
expect_json "$RESPONSE" '.data.name' "test" "current namespace returned"

RESPONSE=$(anon_curl "$API/namespace")
expect_contains "$RESPONSE" "Authentication required" "anonymous current namespace denied"

###############################################################################
section "Admin Token Enforcement"
###############################################################################

# User token cannot access admin namespace endpoints
RESPONSE=$(auth_curl "$ADMIN_API/namespaces")
expect_contains "$RESPONSE" "Admin access required" "user cannot list admin namespaces"

RESPONSE=$(auth_curl -X POST \
    -H "Content-Type: application/json" \
    -d '{"name":"should-fail"}' \
    "$ADMIN_API/namespaces")
expect_contains "$RESPONSE" "Admin access required" "user cannot create namespaces"

# Admin token cannot access user endpoints
RESPONSE=$(admin_curl "$API/repos")
expect_contains "$RESPONSE" "Admin token cannot be used for this operation" "admin cannot access user routes"

###############################################################################
summary
