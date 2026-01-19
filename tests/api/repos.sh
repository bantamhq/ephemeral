#!/bin/bash
# Repos API Tests
set -e

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
source "$SCRIPT_DIR/lib.sh"

require_token
trap cleanup EXIT

echo ""
echo -e "${BLUE}═══════════════════════════════════════${NC}"
echo -e "${BLUE}  Repos API Tests${NC}"
echo -e "${BLUE}═══════════════════════════════════════${NC}"

###############################################################################
section "Create"
###############################################################################

# Create a private repo
RESPONSE=$(auth_curl -X POST -H "Content-Type: application/json" \
    -d '{"name":"test-repo-1","public":false}' \
    "$API/repos")

REPO1_ID=$(get_id "$RESPONSE")
if [ -n "$REPO1_ID" ]; then
    track_repo "$REPO1_ID"
    pass "create private repo"
else
    fail "create private repo" "valid ID" "$RESPONSE"
fi

expect_json "$RESPONSE" '.data.name' "test-repo-1" "name matches"
expect_json "$RESPONSE" '.data.public' "false" "public=false"

# Create a public repo
RESPONSE=$(auth_curl -X POST -H "Content-Type: application/json" \
    -d '{"name":"test-repo-public","public":true}' \
    "$API/repos")

REPO2_ID=$(get_id "$RESPONSE")
if [ -n "$REPO2_ID" ]; then
    track_repo "$REPO2_ID"
fi
expect_json "$RESPONSE" '.data.public' "true" "public=true"

# Duplicate name should fail
RESPONSE=$(auth_curl -X POST -H "Content-Type: application/json" \
    -d '{"name":"test-repo-1","public":false}' \
    "$API/repos")

expect_contains "$RESPONSE" "already exists" "duplicate name rejected"

# Empty name should fail
RESPONSE=$(auth_curl -X POST -H "Content-Type: application/json" \
    -d '{"name":"","public":false}' \
    "$API/repos")

expect_contains "$RESPONSE" "required" "empty name rejected"

###############################################################################
section "Get"
###############################################################################

# Get repo by ID
RESPONSE=$(auth_curl "$API/repos/$REPO1_ID")
expect_json "$RESPONSE" '.data.id' "$REPO1_ID" "returns correct repo"
expect_json "$RESPONSE" '.data.name' "test-repo-1" "name matches"

# Get non-existent repo
RESPONSE=$(auth_curl "$API/repos/nonexistent-id")
expect_contains "$RESPONSE" "not found" "non-existent returns 404"

###############################################################################
section "List"
###############################################################################

# List repos
RESPONSE=$(auth_curl "$API/repos")
expect_contains "$RESPONSE" "test-repo-1" "contains test-repo-1"
expect_contains "$RESPONSE" "test-repo-public" "contains test-repo-public"

# Has data array
expect_json_length "$RESPONSE" '.data' "2" "returns at least 2 repos"

###############################################################################
section "Update"
###############################################################################

# Update name
RESPONSE=$(auth_curl -X PATCH -H "Content-Type: application/json" \
    -d '{"name":"test-repo-renamed"}' \
    "$API/repos/$REPO1_ID")

expect_json "$RESPONSE" '.data.name' "test-repo-renamed" "name changed"

# Update public flag
RESPONSE=$(auth_curl -X PATCH -H "Content-Type: application/json" \
    -d '{"public":true}' \
    "$API/repos/$REPO1_ID")

expect_json "$RESPONSE" '.data.public' "true" "public changed to true"

# Verify changes persisted
RESPONSE=$(auth_curl "$API/repos/$REPO1_ID")
expect_json "$RESPONSE" '.data.name' "test-repo-renamed" "name persisted"
expect_json "$RESPONSE" '.data.public' "true" "public persisted"

# Update non-existent repo
RESPONSE=$(auth_curl -X PATCH -H "Content-Type: application/json" \
    -d '{"name":"foo"}' \
    "$API/repos/nonexistent-id")

expect_contains "$RESPONSE" "not found" "non-existent returns 404"

###############################################################################
section "Delete"
###############################################################################

# Create a repo to delete
RESPONSE=$(auth_curl -X POST -H "Content-Type: application/json" \
    -d '{"name":"test-repo-delete","public":false}' \
    "$API/repos")

DELETE_REPO_ID=$(get_id "$RESPONSE")

# Delete it
auth_curl -X DELETE "$API/repos/$DELETE_REPO_ID" > /dev/null
pass "repo deleted"

# Verify it's gone
RESPONSE=$(auth_curl "$API/repos/$DELETE_REPO_ID")
expect_contains "$RESPONSE" "not found" "repo no longer exists"

# Delete non-existent repo
RESPONSE=$(auth_curl -X DELETE "$API/repos/nonexistent-id")
expect_contains "$RESPONSE" "not found" "non-existent returns 404"

###############################################################################
section "Auth"
###############################################################################

# Anonymous list should fail
RESPONSE=$(anon_curl "$API/repos")
expect_contains "$RESPONSE" "Authentication required\|Unauthorized" "anonymous list denied"

# Anonymous create should fail
RESPONSE=$(anon_curl -X POST -H "Content-Type: application/json" \
    -d '{"name":"anon-repo","public":false}' \
    "$API/repos")
expect_contains "$RESPONSE" "Authentication required\|Unauthorized" "anonymous create denied"

###############################################################################
summary
