#!/bin/bash
set -e

# Repos & Tokens API Test Suite
# Usage: ./repos_tokens_api_test.sh [token]

BASE_URL="${BASE_URL:-http://localhost:8080}"
TOKEN="${1:-$TOKEN}"
PASS=0
FAIL=0

if [ -z "$TOKEN" ]; then
    echo -n "Enter token: "
    read -r TOKEN
fi

if [ -z "$TOKEN" ]; then
    echo "Error: Token is required"
    exit 1
fi

# Colors
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m'

pass() {
    echo -e "${GREEN}PASS${NC}: $1"
    PASS=$((PASS + 1))
}

fail() {
    echo -e "${RED}FAIL${NC}: $1"
    echo "  Expected: $2"
    echo "  Got: $3"
    FAIL=$((FAIL + 1))
}

section() {
    echo ""
    echo -e "${YELLOW}=== $1 ===${NC}"
}

auth_curl() {
    curl -s -u "x-token:$TOKEN" "$@"
}

anon_curl() {
    curl -s "$@"
}

expect_contains() {
    local response="$1"
    local expected="$2"
    local test_name="$3"

    if echo "$response" | grep -q "$expected"; then
        pass "$test_name"
    else
        fail "$test_name" "contains '$expected'" "$response"
    fi
}

expect_not_contains() {
    local response="$1"
    local not_expected="$2"
    local test_name="$3"

    if echo "$response" | grep -q "$not_expected"; then
        fail "$test_name" "not contains '$not_expected'" "$response"
    else
        pass "$test_name"
    fi
}

expect_json() {
    local response="$1"
    local jq_query="$2"
    local expected="$3"
    local test_name="$4"

    local actual=$(echo "$response" | jq -r "$jq_query" 2>/dev/null)
    if [ "$actual" = "$expected" ]; then
        pass "$test_name"
    else
        fail "$test_name" "$expected" "$actual"
    fi
}

expect_status() {
    local response="$1"
    local expected="$2"
    local test_name="$3"

    local status=$(echo "$response" | jq -r '.error // "ok"' 2>/dev/null)
    if [ "$expected" = "ok" ] && [ "$status" = "ok" ]; then
        pass "$test_name"
    elif [ "$expected" != "ok" ] && echo "$status" | grep -qi "$expected"; then
        pass "$test_name"
    else
        fail "$test_name" "$expected" "$status"
    fi
}

# Track created resources for cleanup
CREATED_REPOS=""
CREATED_TOKENS=""

cleanup() {
    echo ""
    section "Cleanup"
    for repo_id in $CREATED_REPOS; do
        auth_curl -X DELETE "$BASE_URL/api/v1/repos/$repo_id" > /dev/null 2>&1 || true
        echo "Deleted repo: $repo_id"
    done
    for token_id in $CREATED_TOKENS; do
        auth_curl -X DELETE "$BASE_URL/api/v1/tokens/$token_id" > /dev/null 2>&1 || true
        echo "Deleted token: $token_id"
    done
}

trap cleanup EXIT

###############################################################################
# REPOS API - CREATE
###############################################################################

section "Repos API - Create"

# Create a repo
RESPONSE=$(auth_curl -X POST \
    -H "Content-Type: application/json" \
    -d '{"name":"test-repo-1","public":false}' \
    "$BASE_URL/api/v1/repos")

REPO1_ID=$(echo "$RESPONSE" | jq -r '.data.ID')
if [ -n "$REPO1_ID" ] && [ "$REPO1_ID" != "null" ]; then
    CREATED_REPOS="$CREATED_REPOS $REPO1_ID"
    pass "create: repo created successfully"
else
    fail "create: repo created successfully" "valid ID" "$RESPONSE"
fi

expect_json "$RESPONSE" '.data.Name' "test-repo-1" "create: name matches"
expect_json "$RESPONSE" '.data.Public' "false" "create: public=false"

# Create public repo
RESPONSE=$(auth_curl -X POST \
    -H "Content-Type: application/json" \
    -d '{"name":"test-repo-public","public":true}' \
    "$BASE_URL/api/v1/repos")

REPO2_ID=$(echo "$RESPONSE" | jq -r '.data.ID')
if [ -n "$REPO2_ID" ] && [ "$REPO2_ID" != "null" ]; then
    CREATED_REPOS="$CREATED_REPOS $REPO2_ID"
fi
expect_json "$RESPONSE" '.data.Public' "true" "create: public=true"

# Duplicate name should fail
RESPONSE=$(auth_curl -X POST \
    -H "Content-Type: application/json" \
    -d '{"name":"test-repo-1","public":false}' \
    "$BASE_URL/api/v1/repos")

expect_contains "$RESPONSE" "already exists" "create: duplicate name rejected"

# Empty name should fail
RESPONSE=$(auth_curl -X POST \
    -H "Content-Type: application/json" \
    -d '{"name":"","public":false}' \
    "$BASE_URL/api/v1/repos")

expect_contains "$RESPONSE" "required" "create: empty name rejected"

###############################################################################
# REPOS API - GET
###############################################################################

section "Repos API - Get"

# Get repo by ID
RESPONSE=$(auth_curl "$BASE_URL/api/v1/repos/$REPO1_ID")
expect_json "$RESPONSE" '.data.ID' "$REPO1_ID" "get: returns correct repo"
expect_json "$RESPONSE" '.data.Name' "test-repo-1" "get: name matches"

# Get non-existent repo
RESPONSE=$(auth_curl "$BASE_URL/api/v1/repos/nonexistent-id")
expect_contains "$RESPONSE" "not found" "get: non-existent returns 404"

###############################################################################
# REPOS API - LIST
###############################################################################

section "Repos API - List"

# List repos
RESPONSE=$(auth_curl "$BASE_URL/api/v1/repos")
expect_contains "$RESPONSE" "test-repo-1" "list: contains test-repo-1"
expect_contains "$RESPONSE" "test-repo-public" "list: contains test-repo-public"

# List with pagination
RESPONSE=$(auth_curl "$BASE_URL/api/v1/repos?limit=1")
COUNT=$(echo "$RESPONSE" | jq '.data | length')
if [ "$COUNT" -ge 1 ]; then
    pass "list: pagination returns results"
else
    fail "list: pagination returns results" ">=1 results" "$COUNT"
fi

###############################################################################
# REPOS API - UPDATE
###############################################################################

section "Repos API - Update"

# Update name
RESPONSE=$(auth_curl -X PATCH \
    -H "Content-Type: application/json" \
    -d '{"name":"test-repo-renamed"}' \
    "$BASE_URL/api/v1/repos/$REPO1_ID")

expect_json "$RESPONSE" '.data.Name' "test-repo-renamed" "update: name changed"

# Update public flag
RESPONSE=$(auth_curl -X PATCH \
    -H "Content-Type: application/json" \
    -d '{"public":true}' \
    "$BASE_URL/api/v1/repos/$REPO1_ID")

expect_json "$RESPONSE" '.data.Public' "true" "update: public changed to true"

# Verify changes persisted
RESPONSE=$(auth_curl "$BASE_URL/api/v1/repos/$REPO1_ID")
expect_json "$RESPONSE" '.data.Name' "test-repo-renamed" "update: name persisted"
expect_json "$RESPONSE" '.data.Public' "true" "update: public persisted"

# Update non-existent repo
RESPONSE=$(auth_curl -X PATCH \
    -H "Content-Type: application/json" \
    -d '{"name":"foo"}' \
    "$BASE_URL/api/v1/repos/nonexistent-id")

expect_contains "$RESPONSE" "not found" "update: non-existent returns 404"

###############################################################################
# REPOS API - DELETE
###############################################################################

section "Repos API - Delete"

# Create a repo to delete
RESPONSE=$(auth_curl -X POST \
    -H "Content-Type: application/json" \
    -d '{"name":"test-repo-delete","public":false}' \
    "$BASE_URL/api/v1/repos")

DELETE_REPO_ID=$(echo "$RESPONSE" | jq -r '.data.ID')

# Delete it
RESPONSE=$(auth_curl -X DELETE "$BASE_URL/api/v1/repos/$DELETE_REPO_ID")
pass "delete: repo deleted successfully"

# Verify it's gone
RESPONSE=$(auth_curl "$BASE_URL/api/v1/repos/$DELETE_REPO_ID")
expect_contains "$RESPONSE" "not found" "delete: repo no longer exists"

# Delete non-existent repo
RESPONSE=$(auth_curl -X DELETE "$BASE_URL/api/v1/repos/nonexistent-id")
expect_contains "$RESPONSE" "not found" "delete: non-existent returns 404"

###############################################################################
# REPOS API - AUTH
###############################################################################

section "Repos API - Auth"

# Anonymous access should fail
RESPONSE=$(anon_curl "$BASE_URL/api/v1/repos")
expect_contains "$RESPONSE" "Authentication required\|Unauthorized" "auth: anonymous list denied"

RESPONSE=$(anon_curl -X POST \
    -H "Content-Type: application/json" \
    -d '{"name":"anon-repo","public":false}' \
    "$BASE_URL/api/v1/repos")
expect_contains "$RESPONSE" "Authentication required\|Unauthorized" "auth: anonymous create denied"

###############################################################################
# TOKENS API - LIST
###############################################################################

section "Tokens API - List"

RESPONSE=$(auth_curl "$BASE_URL/api/v1/tokens")
expect_contains "$RESPONSE" '"data"' "list: returns data array"
# Should contain at least the root token
COUNT=$(echo "$RESPONSE" | jq '.data | length')
if [ "$COUNT" -ge 1 ]; then
    pass "list: contains at least one token"
else
    fail "list: contains at least one token" ">=1" "$COUNT"
fi

###############################################################################
# TOKENS API - CREATE
###############################################################################

section "Tokens API - Create"

# Create read-only token
RESPONSE=$(auth_curl -X POST \
    -H "Content-Type: application/json" \
    -d '{"name":"test-readonly","scope":"read-only"}' \
    "$BASE_URL/api/v1/tokens")

TOKEN1_ID=$(echo "$RESPONSE" | jq -r '.data.id')
if [ -n "$TOKEN1_ID" ] && [ "$TOKEN1_ID" != "null" ]; then
    CREATED_TOKENS="$CREATED_TOKENS $TOKEN1_ID"
fi
expect_contains "$RESPONSE" '"token":"eph_' "create: returns token value"
expect_json "$RESPONSE" '.data.scope' "read-only" "create: scope is read-only"

# Create repos scope token
RESPONSE=$(auth_curl -X POST \
    -H "Content-Type: application/json" \
    -d '{"name":"test-repos","scope":"repos"}' \
    "$BASE_URL/api/v1/tokens")

TOKEN2_ID=$(echo "$RESPONSE" | jq -r '.data.id')
if [ -n "$TOKEN2_ID" ] && [ "$TOKEN2_ID" != "null" ]; then
    CREATED_TOKENS="$CREATED_TOKENS $TOKEN2_ID"
fi
expect_json "$RESPONSE" '.data.scope' "repos" "create: scope is repos"

# Create token with expiration
RESPONSE=$(auth_curl -X POST \
    -H "Content-Type: application/json" \
    -d '{"name":"test-expiring","scope":"read-only","expires_in_seconds":3600}' \
    "$BASE_URL/api/v1/tokens")

TOKEN3_ID=$(echo "$RESPONSE" | jq -r '.data.id')
if [ -n "$TOKEN3_ID" ] && [ "$TOKEN3_ID" != "null" ]; then
    CREATED_TOKENS="$CREATED_TOKENS $TOKEN3_ID"
fi
expect_contains "$RESPONSE" '"expires_at"' "create: has expiration"

# Invalid scope should fail
RESPONSE=$(auth_curl -X POST \
    -H "Content-Type: application/json" \
    -d '{"name":"bad-scope","scope":"invalid"}' \
    "$BASE_URL/api/v1/tokens")

expect_contains "$RESPONSE" "Invalid scope" "create: invalid scope rejected"

###############################################################################
# TOKENS API - SCOPE RESTRICTIONS
###############################################################################

section "Tokens API - Scope Restrictions"

# Get a repos-scoped token to test with
RESPONSE=$(auth_curl -X POST \
    -H "Content-Type: application/json" \
    -d '{"name":"test-repos-scope","scope":"repos"}' \
    "$BASE_URL/api/v1/tokens")

REPOS_TOKEN=$(echo "$RESPONSE" | jq -r '.data.token')
REPOS_TOKEN_ID=$(echo "$RESPONSE" | jq -r '.data.id')
if [ -n "$REPOS_TOKEN_ID" ] && [ "$REPOS_TOKEN_ID" != "null" ]; then
    CREATED_TOKENS="$CREATED_TOKENS $REPOS_TOKEN_ID"
fi

# repos scope should not be able to create tokens
RESPONSE=$(curl -s -u "x-token:$REPOS_TOKEN" -X POST \
    -H "Content-Type: application/json" \
    -d '{"name":"should-fail","scope":"read-only"}' \
    "$BASE_URL/api/v1/tokens")

expect_contains "$RESPONSE" "Insufficient permissions\|Forbidden" "scope: repos cannot create tokens"

# repos scope should be able to create repos
RESPONSE=$(curl -s -u "x-token:$REPOS_TOKEN" -X POST \
    -H "Content-Type: application/json" \
    -d '{"name":"repos-scope-created","public":false}' \
    "$BASE_URL/api/v1/repos")

SCOPE_REPO_ID=$(echo "$RESPONSE" | jq -r '.data.ID')
if [ -n "$SCOPE_REPO_ID" ] && [ "$SCOPE_REPO_ID" != "null" ]; then
    CREATED_REPOS="$CREATED_REPOS $SCOPE_REPO_ID"
    pass "scope: repos can create repos"
else
    fail "scope: repos can create repos" "success" "$RESPONSE"
fi

# Get a read-only token
RESPONSE=$(auth_curl -X POST \
    -H "Content-Type: application/json" \
    -d '{"name":"test-readonly-scope","scope":"read-only"}' \
    "$BASE_URL/api/v1/tokens")

RO_TOKEN=$(echo "$RESPONSE" | jq -r '.data.token')
RO_TOKEN_ID=$(echo "$RESPONSE" | jq -r '.data.id')
if [ -n "$RO_TOKEN_ID" ] && [ "$RO_TOKEN_ID" != "null" ]; then
    CREATED_TOKENS="$CREATED_TOKENS $RO_TOKEN_ID"
fi

# read-only should not be able to create repos
RESPONSE=$(curl -s -u "x-token:$RO_TOKEN" -X POST \
    -H "Content-Type: application/json" \
    -d '{"name":"should-fail","public":false}' \
    "$BASE_URL/api/v1/repos")

expect_contains "$RESPONSE" "Insufficient permissions\|Forbidden" "scope: read-only cannot create repos"

# read-only should be able to list repos
RESPONSE=$(curl -s -u "x-token:$RO_TOKEN" "$BASE_URL/api/v1/repos")
expect_contains "$RESPONSE" '"data"' "scope: read-only can list repos"

###############################################################################
# TOKENS API - DELETE
###############################################################################

section "Tokens API - Delete"

# Create a token to delete
RESPONSE=$(auth_curl -X POST \
    -H "Content-Type: application/json" \
    -d '{"name":"to-delete","scope":"read-only"}' \
    "$BASE_URL/api/v1/tokens")

DELETE_TOKEN_ID=$(echo "$RESPONSE" | jq -r '.data.id')

# Delete it
auth_curl -X DELETE "$BASE_URL/api/v1/tokens/$DELETE_TOKEN_ID" > /dev/null
pass "delete: token deleted"

# Verify it's gone from list
RESPONSE=$(auth_curl "$BASE_URL/api/v1/tokens")
expect_not_contains "$RESPONSE" "$DELETE_TOKEN_ID" "delete: token no longer in list"

# Delete non-existent token
RESPONSE=$(auth_curl -X DELETE "$BASE_URL/api/v1/tokens/nonexistent-id")
expect_contains "$RESPONSE" "not found" "delete: non-existent returns 404"

###############################################################################
# TOKENS API - CANNOT DELETE SELF
###############################################################################

section "Tokens API - Self-Delete Protection"

# Create a token and use it to try to delete itself
RESPONSE=$(auth_curl -X POST \
    -H "Content-Type: application/json" \
    -d '{"name":"self-delete-test","scope":"full"}' \
    "$BASE_URL/api/v1/tokens")

SELF_TOKEN=$(echo "$RESPONSE" | jq -r '.data.token')
SELF_TOKEN_ID=$(echo "$RESPONSE" | jq -r '.data.id')
CREATED_TOKENS="$CREATED_TOKENS $SELF_TOKEN_ID"

# Try to delete self
RESPONSE=$(curl -s -u "x-token:$SELF_TOKEN" -X DELETE "$BASE_URL/api/v1/tokens/$SELF_TOKEN_ID")
expect_contains "$RESPONSE" "Cannot delete current token" "self-delete: prevented"

###############################################################################
# SUMMARY
###############################################################################

echo ""
echo "========================================"
echo -e "Results: ${GREEN}$PASS passed${NC}, ${RED}$FAIL failed${NC}"
echo "========================================"

if [ $FAIL -gt 0 ]; then
    exit 1
fi
