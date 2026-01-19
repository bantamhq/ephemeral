#!/bin/bash
# Folders API Tests
set -e

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
source "$SCRIPT_DIR/lib.sh"

require_token
trap cleanup EXIT

echo ""
echo -e "${BLUE}═══════════════════════════════════════${NC}"
echo -e "${BLUE}  Folders API Tests${NC}"
echo -e "${BLUE}═══════════════════════════════════════${NC}"

###############################################################################
section "Create"
###############################################################################

# Create a root folder
RESPONSE=$(auth_curl -X POST -H "Content-Type: application/json" \
    -d '{"name":"projects"}' \
    "$API/folders")

FOLDER1_ID=$(get_id "$RESPONSE")
if [ -n "$FOLDER1_ID" ]; then
    track_folder "$FOLDER1_ID"
    pass "create root folder"
else
    fail "create root folder" "valid ID" "$RESPONSE"
fi

expect_json "$RESPONSE" '.data.name' "projects" "name matches"
expect_json "$RESPONSE" '.data.parent_id' "null" "parent_id is null"

# Create a nested folder
RESPONSE=$(auth_curl -X POST -H "Content-Type: application/json" \
    -d "{\"name\":\"web\",\"parent_id\":\"$FOLDER1_ID\"}" \
    "$API/folders")

FOLDER2_ID=$(get_id "$RESPONSE")
if [ -n "$FOLDER2_ID" ]; then
    track_folder "$FOLDER2_ID"
fi
expect_json "$RESPONSE" '.data.name' "web" "nested folder name"
expect_json "$RESPONSE" '.data.parent_id' "$FOLDER1_ID" "parent_id set"

# Create another nested folder
RESPONSE=$(auth_curl -X POST -H "Content-Type: application/json" \
    -d "{\"name\":\"mobile\",\"parent_id\":\"$FOLDER1_ID\"}" \
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

# Invalid parent should fail
RESPONSE=$(auth_curl -X POST -H "Content-Type: application/json" \
    -d '{"name":"orphan","parent_id":"nonexistent"}' \
    "$API/folders")

expect_contains "$RESPONSE" "not found" "invalid parent rejected"

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

# Duplicate folder name at root should fail
RESPONSE=$(auth_curl -X POST -H "Content-Type: application/json" \
    -d '{"name":"projects"}' \
    "$API/folders")

expect_contains "$RESPONSE" "already exists" "duplicate root folder rejected"

# Same name under different parents should succeed
RESPONSE=$(auth_curl -X POST -H "Content-Type: application/json" \
    -d "{\"name\":\"docs\",\"parent_id\":\"$FOLDER1_ID\"}" \
    "$API/folders")

FOLDER_DOCS1_ID=$(get_id "$RESPONSE")
if [ -n "$FOLDER_DOCS1_ID" ]; then
    track_folder "$FOLDER_DOCS1_ID"
    pass "create docs under projects"
fi

# Create another folder at root level to test same name under different parents
RESPONSE=$(auth_curl -X POST -H "Content-Type: application/json" \
    -d '{"name":"archive"}' \
    "$API/folders")

FOLDER_ARCHIVE_ID=$(get_id "$RESPONSE")
if [ -n "$FOLDER_ARCHIVE_ID" ]; then
    track_folder "$FOLDER_ARCHIVE_ID"
fi

# Same name (docs) under different parent should succeed
RESPONSE=$(auth_curl -X POST -H "Content-Type: application/json" \
    -d "{\"name\":\"docs\",\"parent_id\":\"$FOLDER_ARCHIVE_ID\"}" \
    "$API/folders")

FOLDER_DOCS2_ID=$(get_id "$RESPONSE")
if [ -n "$FOLDER_DOCS2_ID" ]; then
    track_folder "$FOLDER_DOCS2_ID"
    pass "same name under different parent allowed"
else
    fail "same name under different parent allowed" "success" "$RESPONSE"
fi

# Duplicate name within same parent should fail
RESPONSE=$(auth_curl -X POST -H "Content-Type: application/json" \
    -d "{\"name\":\"docs\",\"parent_id\":\"$FOLDER1_ID\"}" \
    "$API/folders")

expect_contains "$RESPONSE" "already exists" "duplicate in same parent rejected"

###############################################################################
section "List"
###############################################################################

RESPONSE=$(auth_curl "$API/folders")
expect_contains "$RESPONSE" '"projects"' "contains projects folder"
expect_contains "$RESPONSE" '"web"' "contains web folder"
expect_contains "$RESPONSE" '"mobile"' "contains mobile folder"

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

# Cannot set self as parent
RESPONSE=$(auth_curl -X PATCH -H "Content-Type: application/json" \
    -d "{\"parent_id\":\"$FOLDER1_ID\"}" \
    "$API/folders/$FOLDER1_ID")

expect_contains "$RESPONSE" "cannot be its own parent" "self-parent rejected"

# Update with invalid name should fail
RESPONSE=$(auth_curl -X PATCH -H "Content-Type: application/json" \
    -d '{"name":"invalid name with spaces"}' \
    "$API/folders/$FOLDER1_ID")

expect_contains "$RESPONSE" "alphanumeric" "update: invalid name rejected"

# Rename to existing name at same level should fail
RESPONSE=$(auth_curl -X PATCH -H "Content-Type: application/json" \
    -d '{"name":"archive"}' \
    "$API/folders/$FOLDER1_ID")

expect_contains "$RESPONSE" "already exists" "update: duplicate name rejected"

# Move to location with same name should fail
# FOLDER_DOCS1_ID is "docs" under FOLDER1_ID (all-projects)
# FOLDER_DOCS2_ID is "docs" under FOLDER_ARCHIVE_ID (archive)
# Try to move FOLDER_DOCS1_ID to FOLDER_ARCHIVE_ID (where docs already exists)
RESPONSE=$(auth_curl -X PATCH -H "Content-Type: application/json" \
    -d "{\"parent_id\":\"$FOLDER_ARCHIVE_ID\"}" \
    "$API/folders/$FOLDER_DOCS1_ID")

expect_contains "$RESPONSE" "already exists" "update: move to duplicate rejected"

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
# First create a repo in the nested folder
RESPONSE=$(auth_curl -X POST -H "Content-Type: application/json" \
    -d '{"name":"test-repo-in-folder","public":false}' \
    "$API/repos")

REPO_ID=$(get_id "$RESPONSE")
track_repo "$REPO_ID"

# Assign to folder
auth_curl -X PATCH -H "Content-Type: application/json" \
    -d "{\"folder_id\":\"$FOLDER2_ID\"}" \
    "$API/repos/$REPO_ID" > /dev/null

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

# Create read-only token
RESPONSE=$(auth_curl -X POST -H "Content-Type: application/json" \
    -d '{"name":"test-ro-folders","scope":"read-only"}' \
    "$API/tokens")

RO_TOKEN=$(echo "$RESPONSE" | jq -r '.data.token')
RO_TOKEN_ID=$(get_id "$RESPONSE")
track_token "$RO_TOKEN_ID"

# read-only can list folders
RESPONSE=$(auth_curl_with "$RO_TOKEN" "$API/folders")
expect_contains "$RESPONSE" '"data"' "read-only can list folders"

# read-only cannot create folders
RESPONSE=$(auth_curl_with "$RO_TOKEN" -X POST \
    -H "Content-Type: application/json" \
    -d '{"name":"should-fail"}' \
    "$API/folders")
expect_contains "$RESPONSE" "Insufficient permissions\|Forbidden" "read-only cannot create folders"

###############################################################################
summary
