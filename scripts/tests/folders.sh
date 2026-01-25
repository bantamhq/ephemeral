#!/bin/bash
# Folders API Tests (flat folders with M2M repo assignments)
set -e

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
source "$SCRIPT_DIR/lib.sh"

require_token
require_admin_token
trap cleanup EXIT

echo ""
echo -e "${BLUE}═══════════════════════════════════════${NC}"
echo -e "${BLUE}  Folders API Tests${NC}"
echo -e "${BLUE}═══════════════════════════════════════${NC}"

###############################################################################
section "Create"
###############################################################################

# Create a folder
RESPONSE=$(auth_curl -X POST -H "Content-Type: application/json" \
    -d '{"name":"projects"}' \
    "$API/folders")

FOLDER1_ID=$(get_id "$RESPONSE")
if [ -n "$FOLDER1_ID" ]; then
    track_folder "$FOLDER1_ID"
    pass "create folder"
else
    fail "create folder" "valid ID" "$RESPONSE"
fi

expect_json "$RESPONSE" '.data.name' "projects" "name matches"

# Create folder with color
RESPONSE=$(auth_curl -X POST -H "Content-Type: application/json" \
    -d '{"name":"web","color":"#61DAFB"}' \
    "$API/folders")

FOLDER2_ID=$(get_id "$RESPONSE")
if [ -n "$FOLDER2_ID" ]; then
    track_folder "$FOLDER2_ID"
fi
expect_json "$RESPONSE" '.data.name' "web" "folder with color name"
expect_json "$RESPONSE" '.data.color' "#61DAFB" "color set"

# Create more folders for M2M tests
RESPONSE=$(auth_curl -X POST -H "Content-Type: application/json" \
    -d '{"name":"backend","color":"#00ADD8"}' \
    "$API/folders")

FOLDER3_ID=$(get_id "$RESPONSE")
if [ -n "$FOLDER3_ID" ]; then
    track_folder "$FOLDER3_ID"
fi

# Empty name should fail
RESPONSE=$(auth_curl -X POST -H "Content-Type: application/json" \
    -d '{"name":""}' \
    "$API/folders")

expect_contains "$RESPONSE" "required" "empty name rejected"

# Invalid characters should fail (spaces)
RESPONSE=$(auth_curl -X POST -H "Content-Type: application/json" \
    -d '{"name":"my folder"}' \
    "$API/folders")

expect_contains "$RESPONSE" "alphanumeric" "spaces in name rejected"

# Invalid characters should fail (special chars)
RESPONSE=$(auth_curl -X POST -H "Content-Type: application/json" \
    -d '{"name":"folder@123"}' \
    "$API/folders")

expect_contains "$RESPONSE" "alphanumeric" "special chars rejected"

# Name starting with non-alphanumeric should fail
RESPONSE=$(auth_curl -X POST -H "Content-Type: application/json" \
    -d '{"name":"-myfolder"}' \
    "$API/folders")

expect_contains "$RESPONSE" "start with alphanumeric" "leading hyphen rejected"

# Duplicate folder name should fail
RESPONSE=$(auth_curl -X POST -H "Content-Type: application/json" \
    -d '{"name":"projects"}' \
    "$API/folders")

expect_contains "$RESPONSE" "already exists" "duplicate folder rejected"

###############################################################################
section "List"
###############################################################################

RESPONSE=$(auth_curl "$API/folders")
expect_contains "$RESPONSE" '"projects"' "contains projects folder"
expect_contains "$RESPONSE" '"web"' "contains web folder"
expect_contains "$RESPONSE" '"backend"' "contains backend folder"

# Pagination
RESPONSE=$(auth_curl "$API/folders?limit=1")
expect_json "$RESPONSE" '.has_more' "true" "limit=1 has more"

NEXT_CURSOR=$(echo "$RESPONSE" | jq -r '.next_cursor')
if [ -n "$NEXT_CURSOR" ] && [ "$NEXT_CURSOR" != "null" ]; then
    pass "limit=1 returns next_cursor"
else
    fail "limit=1 returns next_cursor" "non-empty cursor" "$NEXT_CURSOR"
fi

RESPONSE=$(auth_curl "$API/folders?limit=1&cursor=$NEXT_CURSOR")
NEXT_NAME=$(echo "$RESPONSE" | jq -r '.data[0].name')
if [ "$NEXT_NAME" != "$NEXT_CURSOR" ]; then
    pass "cursor returns next page"
else
    fail "cursor returns next page" "different name" "$NEXT_NAME"
fi

###############################################################################
section "Get"
###############################################################################

# Get folder by ID
RESPONSE=$(auth_curl "$API/folders/$FOLDER1_ID")
expect_json "$RESPONSE" '.data.id' "$FOLDER1_ID" "returns correct folder"
expect_json "$RESPONSE" '.data.name' "projects" "name matches"

# Get non-existent folder
RESPONSE=$(auth_curl "$API/folders/nonexistent-id")
expect_contains "$RESPONSE" "not found" "non-existent returns 404"

###############################################################################
section "Update"
###############################################################################

# Update name
RESPONSE=$(auth_curl -X PATCH -H "Content-Type: application/json" \
    -d '{"name":"all-projects"}' \
    "$API/folders/$FOLDER1_ID")

expect_json "$RESPONSE" '.data.name' "all-projects" "name changed"

# Verify persisted
RESPONSE=$(auth_curl "$API/folders/$FOLDER1_ID")
expect_json "$RESPONSE" '.data.name' "all-projects" "name persisted"

# Update color
RESPONSE=$(auth_curl -X PATCH -H "Content-Type: application/json" \
    -d '{"color":"#FF0000"}' \
    "$API/folders/$FOLDER1_ID")

expect_json "$RESPONSE" '.data.color' "#FF0000" "color changed"

# Clear color
RESPONSE=$(auth_curl -X PATCH -H "Content-Type: application/json" \
    -d '{"color":""}' \
    "$API/folders/$FOLDER1_ID")

expect_json "$RESPONSE" '.data.color' "null" "color cleared"

# Update with invalid name should fail
RESPONSE=$(auth_curl -X PATCH -H "Content-Type: application/json" \
    -d '{"name":"invalid name with spaces"}' \
    "$API/folders/$FOLDER1_ID")

expect_contains "$RESPONSE" "alphanumeric" "update: invalid name rejected"

# Rename to existing name should fail
RESPONSE=$(auth_curl -X PATCH -H "Content-Type: application/json" \
    -d '{"name":"web"}' \
    "$API/folders/$FOLDER1_ID")

expect_contains "$RESPONSE" "already exists" "update: duplicate name rejected"

###############################################################################
section "Repo-Folder M2M"
###############################################################################

# Create a test repo
RESPONSE=$(auth_curl -X POST -H "Content-Type: application/json" \
    -d '{"name":"test-m2m-repo","public":false}' \
    "$API/repos")

REPO_ID=$(get_id "$RESPONSE")
track_repo "$REPO_ID"

# Initially repo has no folders
RESPONSE=$(auth_curl "$API/repos/$REPO_ID/folders")
FOLDER_COUNT=$(echo "$RESPONSE" | jq -r 'if .data then .data | length else 0 end')
[ "$FOLDER_COUNT" = "0" ] && pass "repo starts with no folders" || fail "repo starts with no folders" "0" "$FOLDER_COUNT"

# Add folders to repo
RESPONSE=$(auth_curl -X POST -H "Content-Type: application/json" \
    -d "{\"folder_ids\":[\"$FOLDER2_ID\",\"$FOLDER3_ID\"]}" \
    "$API/repos/$REPO_ID/folders")

expect_contains "$RESPONSE" '"web"' "repo has web folder"
expect_contains "$RESPONSE" '"backend"' "repo has backend folder"

FOLDER_COUNT=$(echo "$RESPONSE" | jq -r '.data | length')
[ "$FOLDER_COUNT" = "2" ] && pass "repo has 2 folders" || fail "repo has 2 folders" "2" "$FOLDER_COUNT"

# List folders for repo
RESPONSE=$(auth_curl "$API/repos/$REPO_ID/folders")
expect_contains "$RESPONSE" '"web"' "list repo folders: has web"

# Set folders (replace all)
RESPONSE=$(auth_curl -X PUT -H "Content-Type: application/json" \
    -d "{\"folder_ids\":[\"$FOLDER1_ID\"]}" \
    "$API/repos/$REPO_ID/folders")

FOLDER_COUNT=$(echo "$RESPONSE" | jq -r '.data | length')
[ "$FOLDER_COUNT" = "1" ] && pass "set replaces all folders" || fail "set replaces all folders" "1" "$FOLDER_COUNT"
expect_contains "$RESPONSE" '"all-projects"' "repo now has all-projects folder"

# Remove folder from repo
auth_curl -X DELETE "$API/repos/$REPO_ID/folders/$FOLDER1_ID" > /dev/null
pass "remove folder from repo"

RESPONSE=$(auth_curl "$API/repos/$REPO_ID/folders")
FOLDER_COUNT=$(echo "$RESPONSE" | jq -r '.data | length')
[ "$FOLDER_COUNT" = "0" ] && pass "repo has no folders after removal" || fail "repo has no folders after removal" "0" "$FOLDER_COUNT"

# Invalid folder ID should fail
RESPONSE=$(auth_curl -X POST -H "Content-Type: application/json" \
    -d '{"folder_ids":["nonexistent"]}' \
    "$API/repos/$REPO_ID/folders")

expect_contains "$RESPONSE" "not found" "invalid folder rejected"

###############################################################################
section "Delete"
###############################################################################

# Create folder to delete
RESPONSE=$(auth_curl -X POST -H "Content-Type: application/json" \
    -d '{"name":"to-delete"}' \
    "$API/folders")

DELETE_FOLDER_ID=$(get_id "$RESPONSE")

# Delete empty folder
auth_curl -X DELETE "$API/folders/$DELETE_FOLDER_ID" > /dev/null
pass "empty folder deleted"

# Verify it's gone
RESPONSE=$(auth_curl "$API/folders/$DELETE_FOLDER_ID")
expect_contains "$RESPONSE" "not found" "folder no longer exists"

# Non-empty folder without force should fail
# First add web folder to the repo
auth_curl -X PUT -H "Content-Type: application/json" \
    -d "{\"folder_ids\":[\"$FOLDER2_ID\"]}" \
    "$API/repos/$REPO_ID/folders" > /dev/null

# Try to delete non-empty folder
RESPONSE=$(auth_curl -X DELETE "$API/folders/$FOLDER2_ID")
expect_contains "$RESPONSE" "not empty" "non-empty folder rejected"

# Delete with force
auth_curl -X DELETE "$API/folders/$FOLDER2_ID?force=true" > /dev/null
pass "force delete works"

# Remove from tracking since we deleted it
CREATED_FOLDERS=$(echo "$CREATED_FOLDERS" | sed "s/$FOLDER2_ID//g")

###############################################################################
section "Auth"
###############################################################################

# Create a namespace for read-only user
RESPONSE=$(admin_curl -X POST -H "Content-Type: application/json" \
    -d '{"name":"folder-readonly-ns"}' \
    "$ADMIN_API/namespaces")

RO_NS_ID=$(get_id "$RESPONSE")
if [ -n "$RO_NS_ID" ]; then
    track_namespace "$RO_NS_ID"
fi

# Create a user
RESPONSE=$(admin_curl -X POST -H "Content-Type: application/json" \
    -d "{\"namespace_id\":\"$RO_NS_ID\"}" \
    "$ADMIN_API/users")

RO_USER_ID=$(get_id "$RESPONSE")

# Give the user read-only access to the test namespace
RESPONSE=$(admin_curl -X POST -H "Content-Type: application/json" \
    -d "{\"namespace_id\":\"$NS_ID\",\"allow\":[\"namespace:read\",\"repo:read\"]}" \
    "$ADMIN_API/users/$RO_USER_ID/namespace-grants")

# Create token for the user
RESPONSE=$(admin_curl -X POST -H "Content-Type: application/json" \
    -d '{}' \
    "$ADMIN_API/users/$RO_USER_ID/tokens")

RO_TOKEN=$(echo "$RESPONSE" | jq -r '.data.token')
RO_TOKEN_ID=$(echo "$RESPONSE" | jq -r '.data.metadata.id // .metadata.id // empty')
if [ -n "$RO_TOKEN_ID" ]; then
    track_token "$RO_TOKEN_ID"
fi

# read-only can list folders
RESPONSE=$(auth_curl_with "$RO_TOKEN" "$API/folders")
expect_contains "$RESPONSE" '"data"' "read-only can list folders"

# read-only cannot create folders in test namespace
RESPONSE=$(auth_curl_with "$RO_TOKEN" -X POST \
    -H "Content-Type: application/json" \
    -d '{"name":"should-fail","namespace":"test"}' \
    "$API/folders")
expect_contains "$RESPONSE" "Insufficient permissions\|Forbidden" "read-only cannot create folders"

# read-only cannot update folders
RESPONSE=$(auth_curl_with "$RO_TOKEN" -X PATCH \
    -H "Content-Type: application/json" \
    -d '{"name":"should-fail-update"}' \
    "$API/folders/$FOLDER1_ID")
expect_contains "$RESPONSE" "Insufficient permissions\|Forbidden" "read-only cannot update folders"

# read-only cannot delete folders
RESPONSE=$(auth_curl_with "$RO_TOKEN" -X DELETE "$API/folders/$FOLDER1_ID")
expect_contains "$RESPONSE" "Insufficient permissions\|Forbidden" "read-only cannot delete folders"

# Clean up read-only user
admin_curl -X DELETE "$ADMIN_API/users/$RO_USER_ID" > /dev/null 2>&1

###############################################################################
summary
