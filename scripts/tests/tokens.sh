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
section "Admin: Create User Token (Simple Mode)"
###############################################################################

# Create token using simple mode (namespace_id only)
RESPONSE=$(admin_curl -X POST -H "Content-Type: application/json" \
    -d "{\"namespace_id\":\"$NS_ID\",\"name\":\"test-simple\"}" \
    "$ADMIN_API/tokens")

TOKEN1_ID=$(get_id "$RESPONSE")
if [ -n "$TOKEN1_ID" ]; then
    track_token "$TOKEN1_ID"
fi
expect_contains "$RESPONSE" '"token":"eph_' "returns token value"
expect_json "$RESPONSE" '.data.is_admin' "false" "is not admin"
expect_contains "$RESPONSE" '"namespace_grants"' "has namespace_grants"
expect_contains "$RESPONSE" '"namespace:write"' "has namespace:write permission"
expect_contains "$RESPONSE" '"repo:admin"' "has repo:admin permission"

# Create token with expiration
RESPONSE=$(admin_curl -X POST -H "Content-Type: application/json" \
    -d "{\"namespace_id\":\"$NS_ID\",\"name\":\"test-expiring\",\"expires_in_seconds\":3600}" \
    "$ADMIN_API/tokens")

TOKEN2_ID=$(get_id "$RESPONSE")
if [ -n "$TOKEN2_ID" ]; then
    track_token "$TOKEN2_ID"
fi
expect_contains "$RESPONSE" '"expires_at"' "has expiration"

###############################################################################
section "Admin: Create User Token (Full Mode)"
###############################################################################

# Create token with explicit grants
RESPONSE=$(admin_curl -X POST -H "Content-Type: application/json" \
    -d "{\"name\":\"test-full-mode\",\"namespace_grants\":[{\"namespace_id\":\"$NS_ID\",\"allow\":[\"namespace:read\",\"repo:read\"],\"is_primary\":true}]}" \
    "$ADMIN_API/tokens")

TOKEN3_ID=$(get_id "$RESPONSE")
if [ -n "$TOKEN3_ID" ]; then
    track_token "$TOKEN3_ID"
fi
expect_contains "$RESPONSE" '"namespace:read"' "has namespace:read"
expect_contains "$RESPONSE" '"repo:read"' "has repo:read"
expect_not_contains "$RESPONSE" '"namespace:write"' "does not have namespace:write"

###############################################################################
section "Admin: Token Validation"
###############################################################################

# Negative expiration should fail
RESPONSE=$(admin_curl -X POST -H "Content-Type: application/json" \
    -d "{\"namespace_id\":\"$NS_ID\",\"name\":\"bad-expiry\",\"expires_in_seconds\":-10}" \
    "$ADMIN_API/tokens")

expect_contains "$RESPONSE" "cannot be negative" "negative expiration rejected"

# User token without namespace or grants should fail
RESPONSE=$(admin_curl -X POST -H "Content-Type: application/json" \
    -d '{"name":"no-namespace"}' \
    "$ADMIN_API/tokens")

expect_contains "$RESPONSE" "namespace_id\|grants" "user token requires namespace or grants"

# Admin token with namespace should fail
RESPONSE=$(admin_curl -X POST -H "Content-Type: application/json" \
    -d "{\"namespace_id\":\"$NS_ID\",\"name\":\"admin-with-ns\",\"is_admin\":true}" \
    "$ADMIN_API/tokens")

expect_contains "$RESPONSE" "cannot have" "admin token rejects namespace"

# Cannot mix simple and full mode
RESPONSE=$(admin_curl -X POST -H "Content-Type: application/json" \
    -d "{\"namespace_id\":\"$NS_ID\",\"namespace_grants\":[{\"namespace_id\":\"$NS_ID\",\"allow\":[\"repo:read\"]}]}" \
    "$ADMIN_API/tokens")

expect_contains "$RESPONSE" "Cannot use both" "mixed mode rejected"

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

###############################################################################
section "Permission Restrictions"
###############################################################################

# Create a token with repo:read only to test restrictions
RESPONSE=$(admin_curl -X POST -H "Content-Type: application/json" \
    -d "{\"name\":\"test-readonly\",\"namespace_grants\":[{\"namespace_id\":\"$NS_ID\",\"allow\":[\"namespace:read\",\"repo:read\"],\"is_primary\":true}]}" \
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
    -d "{\"namespace_id\":\"$NS_ID\",\"name\":\"to-delete\"}" \
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
    -d "{\"namespace_id\":\"$NS_ID\",\"name\":\"expire-soon\",\"expires_in_seconds\":1}" \
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
section "Grant Management"
###############################################################################

# Create a token for grant management tests
RESPONSE=$(admin_curl -X POST -H "Content-Type: application/json" \
    -d "{\"namespace_id\":\"$NS_ID\",\"name\":\"grant-test\"}" \
    "$ADMIN_API/tokens")

GRANT_TOKEN_ID=$(get_id "$RESPONSE")
if [ -n "$GRANT_TOKEN_ID" ]; then
    track_token "$GRANT_TOKEN_ID"
fi

# List namespace grants
RESPONSE=$(admin_curl "$ADMIN_API/tokens/$GRANT_TOKEN_ID/namespace-grants")
expect_contains "$RESPONSE" '"data"' "can list namespace grants"
expect_contains "$RESPONSE" "$NS_ID" "grant contains namespace ID"

# Create second namespace for grant test
RESPONSE=$(admin_curl -X POST -H "Content-Type: application/json" \
    -d '{"name":"grant-test-ns"}' \
    "$ADMIN_API/namespaces")

NS2_ID=$(get_id "$RESPONSE")
if [ -n "$NS2_ID" ]; then
    track_namespace "$NS2_ID"
fi

# Add namespace grant
RESPONSE=$(admin_curl -X POST -H "Content-Type: application/json" \
    -d "{\"namespace_id\":\"$NS2_ID\",\"allow\":[\"repo:read\"],\"is_primary\":false}" \
    "$ADMIN_API/tokens/$GRANT_TOKEN_ID/namespace-grants")

expect_contains "$RESPONSE" "$NS2_ID" "grant added for second namespace"

# Delete namespace grant
RESPONSE=$(admin_curl -X DELETE "$ADMIN_API/tokens/$GRANT_TOKEN_ID/namespace-grants/$NS2_ID")
pass "namespace grant deleted"

###############################################################################
summary
