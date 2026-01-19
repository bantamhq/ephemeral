#!/bin/bash
# Labels API Tests
set -e

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
source "$SCRIPT_DIR/lib.sh"

require_token
trap cleanup EXIT

echo ""
echo -e "${BLUE}═══════════════════════════════════════${NC}"
echo -e "${BLUE}  Labels API Tests${NC}"
echo -e "${BLUE}═══════════════════════════════════════${NC}"

###############################################################################
section "Create"
###############################################################################

# Create a label
RESPONSE=$(auth_curl -X POST -H "Content-Type: application/json" \
    -d '{"name":"go"}' \
    "$API/labels")

LABEL1_ID=$(get_id "$RESPONSE")
if [ -n "$LABEL1_ID" ]; then
    track_label "$LABEL1_ID"
    pass "create label"
else
    fail "create label" "valid ID" "$RESPONSE"
fi

expect_json "$RESPONSE" '.data.Name' "go" "name matches"

# Create label with color
RESPONSE=$(auth_curl -X POST -H "Content-Type: application/json" \
    -d '{"name":"typescript","color":"#3178C6"}' \
    "$API/labels")

LABEL2_ID=$(get_id "$RESPONSE")
if [ -n "$LABEL2_ID" ]; then
    track_label "$LABEL2_ID"
fi
expect_json "$RESPONSE" '.data.Name' "typescript" "name matches"
expect_json "$RESPONSE" '.data.Color' "#3178C6" "color set"

# Create more labels for testing
RESPONSE=$(auth_curl -X POST -H "Content-Type: application/json" \
    -d '{"name":"rust","color":"#DEA584"}' \
    "$API/labels")
LABEL3_ID=$(get_id "$RESPONSE")
track_label "$LABEL3_ID"

# Duplicate name should fail
RESPONSE=$(auth_curl -X POST -H "Content-Type: application/json" \
    -d '{"name":"go"}' \
    "$API/labels")

expect_contains "$RESPONSE" "already exists" "duplicate name rejected"

# Empty name should fail
RESPONSE=$(auth_curl -X POST -H "Content-Type: application/json" \
    -d '{"name":""}' \
    "$API/labels")

expect_contains "$RESPONSE" "required" "empty name rejected"

###############################################################################
section "List"
###############################################################################

RESPONSE=$(auth_curl "$API/labels")
expect_contains "$RESPONSE" '"go"' "contains go label"
expect_contains "$RESPONSE" '"typescript"' "contains typescript label"
expect_contains "$RESPONSE" '"rust"' "contains rust label"

###############################################################################
section "Get"
###############################################################################

# Get label by ID
RESPONSE=$(auth_curl "$API/labels/$LABEL1_ID")
expect_json "$RESPONSE" '.data.ID' "$LABEL1_ID" "returns correct label"
expect_json "$RESPONSE" '.data.Name' "go" "name matches"

# Get non-existent label
RESPONSE=$(auth_curl "$API/labels/nonexistent-id")
expect_contains "$RESPONSE" "not found" "non-existent returns 404"

###############################################################################
section "Update"
###############################################################################

# Update name
RESPONSE=$(auth_curl -X PATCH -H "Content-Type: application/json" \
    -d '{"name":"golang"}' \
    "$API/labels/$LABEL1_ID")

expect_json "$RESPONSE" '.data.Name' "golang" "name changed"

# Update color
RESPONSE=$(auth_curl -X PATCH -H "Content-Type: application/json" \
    -d '{"color":"#00ADD8"}' \
    "$API/labels/$LABEL1_ID")

expect_json "$RESPONSE" '.data.Color' "#00ADD8" "color changed"

# Verify persisted
RESPONSE=$(auth_curl "$API/labels/$LABEL1_ID")
expect_json "$RESPONSE" '.data.Name' "golang" "name persisted"
expect_json "$RESPONSE" '.data.Color' "#00ADD8" "color persisted"

###############################################################################
section "Repo Labels"
###############################################################################

# Create a repo to test with
RESPONSE=$(auth_curl -X POST -H "Content-Type: application/json" \
    -d '{"name":"test-repo-labels","public":false}' \
    "$API/repos")

REPO_ID=$(get_id "$RESPONSE")
track_repo "$REPO_ID"

# Add labels to repo
RESPONSE=$(auth_curl -X POST -H "Content-Type: application/json" \
    -d "{\"label_ids\":[\"$LABEL1_ID\",\"$LABEL2_ID\"]}" \
    "$API/repos/$REPO_ID/labels")

expect_contains "$RESPONSE" '"golang"' "repo has golang label"
expect_contains "$RESPONSE" '"typescript"' "repo has typescript label"

# List repo labels
RESPONSE=$(auth_curl "$API/repos/$REPO_ID/labels")
expect_json_length "$RESPONSE" '.data' "2" "repo has 2 labels"

# Remove a label
auth_curl -X DELETE "$API/repos/$REPO_ID/labels/$LABEL2_ID" > /dev/null
pass "label removed from repo"

# Verify removal
RESPONSE=$(auth_curl "$API/repos/$REPO_ID/labels")
expect_json_length "$RESPONSE" '.data' "1" "repo has 1 label after removal"
expect_not_contains "$RESPONSE" '"typescript"' "typescript label removed"

# Add invalid label should fail
RESPONSE=$(auth_curl -X POST -H "Content-Type: application/json" \
    -d '{"label_ids":["nonexistent"]}' \
    "$API/repos/$REPO_ID/labels")

expect_contains "$RESPONSE" "not found" "invalid label rejected"

###############################################################################
section "Delete"
###############################################################################

# Create label to delete
RESPONSE=$(auth_curl -X POST -H "Content-Type: application/json" \
    -d '{"name":"to-delete"}' \
    "$API/labels")

DELETE_LABEL_ID=$(get_id "$RESPONSE")

# Delete it
auth_curl -X DELETE "$API/labels/$DELETE_LABEL_ID" > /dev/null
pass "label deleted"

# Verify it's gone
RESPONSE=$(auth_curl "$API/labels/$DELETE_LABEL_ID")
expect_contains "$RESPONSE" "not found" "label no longer exists"

###############################################################################
section "Auth"
###############################################################################

# Create read-only token
RESPONSE=$(auth_curl -X POST -H "Content-Type: application/json" \
    -d '{"name":"test-ro-labels","scope":"read-only"}' \
    "$API/tokens")

RO_TOKEN=$(echo "$RESPONSE" | jq -r '.data.token')
RO_TOKEN_ID=$(get_id "$RESPONSE")
track_token "$RO_TOKEN_ID"

# read-only can list labels
RESPONSE=$(curl -s -u "x-token:$RO_TOKEN" "$API/labels")
expect_contains "$RESPONSE" '"data"' "read-only can list labels"

# read-only cannot create labels
RESPONSE=$(curl -s -u "x-token:$RO_TOKEN" -X POST \
    -H "Content-Type: application/json" \
    -d '{"name":"should-fail"}' \
    "$API/labels")
expect_contains "$RESPONSE" "Insufficient permissions\|Forbidden" "read-only cannot create labels"

###############################################################################
summary
