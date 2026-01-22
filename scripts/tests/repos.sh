#!/bin/bash
# Repos API Tests
set -e

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
source "$SCRIPT_DIR/lib.sh"

require_token
require_admin_token
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

# Create repo with description
RESPONSE=$(auth_curl -X POST -H "Content-Type: application/json" \
    -d '{"name":"test-repo-desc","description":"A test repository with description","public":false}' \
    "$API/repos")

REPO_DESC_ID=$(get_id "$RESPONSE")
if [ -n "$REPO_DESC_ID" ]; then
    track_repo "$REPO_DESC_ID"
    pass "create repo with description"
else
    fail "create repo with description" "valid ID" "$RESPONSE"
fi

expect_json "$RESPONSE" '.data.description' "A test repository with description" "description matches"

# Create a public repo
RESPONSE=$(auth_curl -X POST -H "Content-Type: application/json" \
    -d '{"name":"test-repo-public","public":true}' \
    "$API/repos")

REPO2_ID=$(get_id "$RESPONSE")
if [ -n "$REPO2_ID" ]; then
    track_repo "$REPO2_ID"
fi
expect_json "$RESPONSE" '.data.public' "true" "public=true"

# Create repo with uppercase name (should normalize)
RESPONSE=$(auth_curl -X POST -H "Content-Type: application/json" \
    -d '{"name":"Test-Repo-Upper","public":false}' \
    "$API/repos")

REPO3_ID=$(get_id "$RESPONSE")
if [ -n "$REPO3_ID" ]; then
    track_repo "$REPO3_ID"
fi
expect_json "$RESPONSE" '.data.name' "test-repo-upper" "name normalized to lowercase"

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

# Path separators should fail
RESPONSE=$(auth_curl -X POST -H "Content-Type: application/json" \
    -d '{"name":"bad/name","public":false}' \
    "$API/repos")

expect_contains "$RESPONSE" "path separators" "path separators rejected"

# Dot-dot should fail
RESPONSE=$(auth_curl -X POST -H "Content-Type: application/json" \
    -d '{"name":"bad..name","public":false}' \
    "$API/repos")

expect_contains "$RESPONSE" "cannot contain '..'" "dot-dot rejected"

# Description too long should fail (>512 chars)
LONG_DESC=$(printf 'x%.0s' {1..513})
RESPONSE=$(auth_curl -X POST -H "Content-Type: application/json" \
    -d "{\"name\":\"test-long-desc\",\"description\":\"$LONG_DESC\"}" \
    "$API/repos")

expect_contains "$RESPONSE" "512 characters" "description >512 chars rejected"

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

# expand=folders returns folder associations
RESPONSE=$(auth_curl -X POST -H "Content-Type: application/json" \
    -d '{"name":"expand-folder"}' \
    "$API/folders")

EXPAND_FOLDER_ID=$(get_id "$RESPONSE")
if [ -n "$EXPAND_FOLDER_ID" ]; then
    track_folder "$EXPAND_FOLDER_ID"
fi

auth_curl -X POST -H "Content-Type: application/json" \
    -d "{\"folder_ids\":[\"$EXPAND_FOLDER_ID\"]}" \
    "$API/repos/$REPO1_ID/folders" > /dev/null

RESPONSE=$(auth_curl "$API/repos?expand=folders")
FOLDER_COUNT=$(echo "$RESPONSE" | jq -r --arg id "$REPO1_ID" '.data[] | select(.id == $id) | (.folders // []) | length')
[ "$FOLDER_COUNT" = "1" ] && pass "expand=folders includes folder list" || fail "expand=folders includes folder list" "1" "$FOLDER_COUNT"

# Pagination
RESPONSE=$(auth_curl "$API/repos?limit=1")
expect_json "$RESPONSE" '.has_more' "true" "limit=1 has more"

NEXT_CURSOR=$(echo "$RESPONSE" | jq -r '.next_cursor')
if [ -n "$NEXT_CURSOR" ] && [ "$NEXT_CURSOR" != "null" ]; then
    pass "limit=1 returns next_cursor"
else
    fail "limit=1 returns next_cursor" "non-empty cursor" "$NEXT_CURSOR"
fi

RESPONSE=$(auth_curl "$API/repos?limit=1&cursor=$NEXT_CURSOR")
NEXT_NAME=$(echo "$RESPONSE" | jq -r '.data[0].name')
if [ "$NEXT_NAME" != "$NEXT_CURSOR" ]; then
    pass "cursor returns next page"
else
    fail "cursor returns next page" "different name" "$NEXT_NAME"
fi

###############################################################################
section "Update"
###############################################################################

# Update name
RESPONSE=$(auth_curl -X PATCH -H "Content-Type: application/json" \
    -d '{"name":"test-repo-renamed"}' \
    "$API/repos/$REPO1_ID")

expect_json "$RESPONSE" '.data.name' "test-repo-renamed" "name changed"

# Update with invalid name should fail
RESPONSE=$(auth_curl -X PATCH -H "Content-Type: application/json" \
    -d '{"name":"bad/name"}' \
    "$API/repos/$REPO1_ID")

expect_contains "$RESPONSE" "path separators" "update: path separators rejected"

# Update public flag
RESPONSE=$(auth_curl -X PATCH -H "Content-Type: application/json" \
    -d '{"public":true}' \
    "$API/repos/$REPO1_ID")

expect_json "$RESPONSE" '.data.public' "true" "public changed to true"

# Update description
RESPONSE=$(auth_curl -X PATCH -H "Content-Type: application/json" \
    -d '{"description":"Updated description"}' \
    "$API/repos/$REPO_DESC_ID")

expect_json "$RESPONSE" '.data.description' "Updated description" "description updated"

# Update description to empty
RESPONSE=$(auth_curl -X PATCH -H "Content-Type: application/json" \
    -d '{"description":""}' \
    "$API/repos/$REPO_DESC_ID")

expect_json "$RESPONSE" '.data.description' "" "description cleared"

# Update description too long should fail
LONG_DESC=$(printf 'x%.0s' {1..513})
RESPONSE=$(auth_curl -X PATCH -H "Content-Type: application/json" \
    -d "{\"description\":\"$LONG_DESC\"}" \
    "$API/repos/$REPO_DESC_ID")

expect_contains "$RESPONSE" "512 characters" "update: description >512 chars rejected"

# Rename to existing name should fail
RESPONSE=$(auth_curl -X PATCH -H "Content-Type: application/json" \
    -d '{"name":"test-repo-renamed"}' \
    "$API/repos/$REPO2_ID")

expect_contains "$RESPONSE" "already exists" "update: duplicate name rejected"

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

# Create read-only token via admin API (namespace:read + repo:read only)
RESPONSE=$(admin_curl -X POST -H "Content-Type: application/json" \
    -d "{\"name\":\"repo-readonly\",\"namespace_grants\":[{\"namespace_id\":\"$NS_ID\",\"allow\":[\"namespace:read\",\"repo:read\"],\"is_primary\":true}]}" \
    "$ADMIN_API/tokens")

RO_TOKEN=$(echo "$RESPONSE" | jq -r '.data.token')
RO_TOKEN_ID=$(get_id "$RESPONSE")
if [ -n "$RO_TOKEN_ID" ]; then
    track_token "$RO_TOKEN_ID"
fi

# read-only cannot update repos
RESPONSE=$(auth_curl_with "$RO_TOKEN" -X PATCH -H "Content-Type: application/json" \
    -d '{"public":false}' \
    "$API/repos/$REPO2_ID")
expect_contains "$RESPONSE" "Insufficient permissions\|Forbidden" "read-only cannot update repos"

# read-only cannot delete repos
RESPONSE=$(auth_curl_with "$RO_TOKEN" -X DELETE "$API/repos/$REPO2_ID")
expect_contains "$RESPONSE" "Insufficient permissions\|Forbidden" "read-only cannot delete repos"

###############################################################################
summary
