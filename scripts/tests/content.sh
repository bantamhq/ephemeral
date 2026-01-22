#!/bin/bash
# Content API Tests
set -e

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
source "$SCRIPT_DIR/lib.sh"

require_token
trap cleanup EXIT

echo ""
echo -e "${BLUE}═══════════════════════════════════════${NC}"
echo -e "${BLUE}  Content API Tests${NC}"
echo -e "${BLUE}═══════════════════════════════════════${NC}"

###############################################################################
section "Setup"
###############################################################################

# Get primary namespace name
NS_JSON=$(auth_curl "$API/namespaces")
NS_NAME=$(echo "$NS_JSON" | jq -r '.data[] | select(.is_primary == true) | .name' 2>/dev/null)
if [ -z "$NS_NAME" ]; then
    NS_NAME=$(echo "$NS_JSON" | jq -r '.data[0].name' 2>/dev/null)
fi
if [ -z "$NS_NAME" ] || [ "$NS_NAME" = "null" ]; then
    echo "Failed to get namespace name"
    exit 1
fi
info "Using namespace: $NS_NAME"

info "Creating test repository..."
RESPONSE=$(auth_curl -X POST -H "Content-Type: application/json" \
    -d '{"name":"test-content-api","public":false}' \
    "$API/repos")

REPO_ID=$(get_id "$RESPONSE")
if [ -z "$REPO_ID" ]; then
    echo "Failed to create repo: $RESPONSE"
    exit 1
fi
track_repo "$REPO_ID"
info "Created repo: $REPO_ID"

info "Creating empty repository..."
RESPONSE=$(auth_curl -X POST -H "Content-Type: application/json" \
    -d '{"name":"test-content-empty","public":false}' \
    "$API/repos")

EMPTY_REPO_ID=$(get_id "$RESPONSE")
if [ -z "$EMPTY_REPO_ID" ]; then
    echo "Failed to create empty repo: $RESPONSE"
    exit 1
fi
track_repo "$EMPTY_REPO_ID"
info "Created empty repo: $EMPTY_REPO_ID"

# Clone and add content
TMPDIR=$(mktemp -d)
cd "$TMPDIR"

git clone -q "http://x-token:$TOKEN@${BASE_URL#http://}/git/$NS_NAME/test-content-api.git" repo 2>/dev/null
cd repo

# Create test files
echo "# Test Repository" > README.md
mkdir -p src
cat > src/main.go << 'EOF'
package main

import "fmt"

func main() {
    fmt.Println("Hello")
}
EOF

mkdir -p docs
echo "Documentation" > docs/index.md

# Binary file
printf '\x00\x01\x02\x03' > binary.dat

git add .
git commit -q -m "Initial commit"
git push -q origin main 2>/dev/null

# Second commit
echo "More content" >> README.md
git add .
git commit -q -m "Update README"
git push -q origin main 2>/dev/null

MAIN_HEAD_SHA=$(git rev-parse HEAD)
MAIN_BASE_SHA=$(git rev-parse HEAD~1)

# Create tag
git tag v1.0.0
git push -q origin v1.0.0 2>/dev/null

# Create feature branch
git checkout -q -b feature/api
echo "Feature change" >> docs/index.md
git add docs/index.md
git commit -q -m "Add feature docs"
git push -q origin feature/api 2>/dev/null

FEATURE_HEAD_SHA=$(git rev-parse HEAD)

cd /
rm -rf "$TMPDIR"

info "Test content pushed"

###############################################################################
section "Empty Repository"
###############################################################################

RESPONSE=$(auth_curl "$API/repos/$EMPTY_REPO_ID/refs")
expect_contains "$RESPONSE" "Repository is empty" "empty repo refs returns error"

RESPONSE=$(auth_curl "$API/repos/$EMPTY_REPO_ID/commits")
expect_contains "$RESPONSE" "Repository is empty" "empty repo commits returns error"

###############################################################################
section "Refs"
###############################################################################

RESPONSE=$(auth_curl "$API/repos/$REPO_ID/refs")

expect_contains "$RESPONSE" '"name":"main"' "contains main branch"
expect_contains "$RESPONSE" '"type":"branch"' "has branch type"
expect_contains "$RESPONSE" '"name":"v1.0.0"' "contains v1.0.0 tag"
expect_contains "$RESPONSE" '"type":"tag"' "has tag type"
expect_json "$RESPONSE" '.data[0].name' "main" "main is first (default)"
expect_json "$RESPONSE" '.data[0].is_default' "true" "main is_default=true"

###############################################################################
section "Ref Management"
###############################################################################

API_BRANCH="api/feature"
API_BRANCH_RENAMED="api/feature-renamed"
API_TAG="api-tag"

RESPONSE=$(auth_curl -X POST -H "Content-Type: application/json" \
    -d "{\"name\":\"$API_BRANCH\",\"type\":\"branch\",\"target\":\"main\"}" \
    "$API/repos/$REPO_ID/refs")
expect_contains "$RESPONSE" '"name":"api/feature"' "create branch via API"
expect_json "$RESPONSE" '.data.type' "branch" "branch type returned"

RESPONSE=$(auth_curl -X POST -H "Content-Type: application/json" \
    -d "{\"name\":\"$API_TAG\",\"type\":\"tag\",\"target\":\"main\"}" \
    "$API/repos/$REPO_ID/refs")
expect_contains "$RESPONSE" '"name":"api-tag"' "create tag via API"
expect_json "$RESPONSE" '.data.type' "tag" "tag type returned"

RESPONSE=$(auth_curl -X PATCH -H "Content-Type: application/json" \
    -d "{\"target\":\"$MAIN_BASE_SHA\"}" \
    "$API/repos/$REPO_ID/refs/branch/$API_BRANCH")
expect_contains "$RESPONSE" "$MAIN_BASE_SHA" "update branch target"

RESPONSE=$(auth_curl -X PATCH -H "Content-Type: application/json" \
    -d "{\"new_name\":\"$API_BRANCH_RENAMED\"}" \
    "$API/repos/$REPO_ID/refs/branch/$API_BRANCH")
expect_contains "$RESPONSE" '"name":"api/feature-renamed"' "rename branch"

RESPONSE=$(auth_curl -X PUT -H "Content-Type: application/json" \
    -d "{\"name\":\"$API_BRANCH_RENAMED\"}" \
    "$API/repos/$REPO_ID/default-branch")
expect_contains "$RESPONSE" '"name":"api/feature-renamed"' "default branch set"

RESPONSE=$(auth_curl "$API/repos/$REPO_ID/refs")
expect_json "$RESPONSE" '.data[0].name' "$API_BRANCH_RENAMED" "default branch moved"

auth_curl -X PUT -H "Content-Type: application/json" \
    -d '{"name":"main"}' \
    "$API/repos/$REPO_ID/default-branch" > /dev/null

auth_curl -X DELETE "$API/repos/$REPO_ID/refs/branch/$API_BRANCH_RENAMED" > /dev/null
RESPONSE=$(auth_curl "$API/repos/$REPO_ID/refs")
expect_not_contains "$RESPONSE" '"name":"api/feature-renamed"' "branch deleted"

auth_curl -X DELETE "$API/repos/$REPO_ID/refs/tag/$API_TAG" > /dev/null
RESPONSE=$(auth_curl "$API/repos/$REPO_ID/refs")
expect_not_contains "$RESPONSE" '"name":"api-tag"' "tag deleted"

###############################################################################
section "Commits"
###############################################################################

# Default ref
RESPONSE=$(auth_curl "$API/repos/$REPO_ID/commits")
expect_contains "$RESPONSE" '"message":"Update README' "default ref returns commits"

# Explicit ref
RESPONSE=$(auth_curl "$API/repos/$REPO_ID/commits?ref=main")
expect_contains "$RESPONSE" '"message":"Update README' "ref=main works"

# Commit stats
expect_contains "$RESPONSE" '"stats"' "commits include stats field"
expect_contains "$RESPONSE" '"files_changed"' "stats has files_changed"
expect_contains "$RESPONSE" '"additions"' "stats has additions"
expect_contains "$RESPONSE" '"deletions"' "stats has deletions"

# Verify stats values are integers
FILES_CHANGED=$(echo "$RESPONSE" | jq '.data[0].stats.files_changed')
if [[ "$FILES_CHANGED" =~ ^[0-9]+$ ]]; then
    pass "stats.files_changed is integer"
else
    fail "stats.files_changed is integer" "integer" "$FILES_CHANGED"
fi

LATEST_SHA=$(echo "$RESPONSE" | jq -r '.data[0].sha')
if [ -n "$LATEST_SHA" ] && [ "$LATEST_SHA" != "null" ]; then
    pass "captured latest commit sha"
else
    fail "captured latest commit sha" "non-empty sha" "$LATEST_SHA"
fi

# Path filter
RESPONSE=$(auth_curl "$API/repos/$REPO_ID/commits?ref=main&path=README.md")
expect_contains "$RESPONSE" '"message":"Update README' "path filter returns commits"

# Pagination
RESPONSE=$(auth_curl "$API/repos/$REPO_ID/commits?limit=1")
expect_json "$RESPONSE" '.has_more' "true" "limit=1 has_more=true"
expect_json "$RESPONSE" '.data | length' "1" "limit=1 returns 1 commit"

# Invalid cursor
RESPONSE=$(auth_curl "$API/repos/$REPO_ID/commits?cursor=0000000000000000000000000000000000000000")
expect_contains "$RESPONSE" "Invalid cursor" "invalid cursor rejected"

# Tag ref
RESPONSE=$(auth_curl "$API/repos/$REPO_ID/commits?ref=v1.0.0")
expect_contains "$RESPONSE" '"sha"' "tag ref works"

###############################################################################
section "Commit Detail"
###############################################################################

RESPONSE=$(auth_curl "$API/repos/$REPO_ID/commits/$MAIN_HEAD_SHA")
expect_contains "$RESPONSE" "$MAIN_HEAD_SHA" "commit detail returns sha"
expect_contains "$RESPONSE" '"message":"Update README' "commit detail returns message"
expect_json "$RESPONSE" '.data.parent_shas[0]' "$MAIN_BASE_SHA" "commit detail parent sha"

RESPONSE=$(auth_curl "$API/repos/$REPO_ID/commits/$MAIN_HEAD_SHA/diff")
expect_contains "$RESPONSE" 'README.md' "commit diff includes README"
expect_json "$RESPONSE" '.data.stats.files_changed' "1" "commit diff stats"

###############################################################################
section "Compare"
###############################################################################

RESPONSE=$(auth_curl "$API/repos/$REPO_ID/compare/main...feature%2Fapi")
expect_json "$RESPONSE" '.data.ahead_by' "1" "compare ahead_by=1"
expect_json "$RESPONSE" '.data.behind_by' "0" "compare behind_by=0"
expect_json "$RESPONSE" '.data.merge_base_sha' "$MAIN_HEAD_SHA" "compare merge base"
expect_json "$RESPONSE" '.data.base_sha' "$MAIN_HEAD_SHA" "compare base sha"
expect_json "$RESPONSE" '.data.head_sha' "$FEATURE_HEAD_SHA" "compare head sha"
expect_json "$RESPONSE" '.data.commits | length' "1" "compare returns 1 commit"
expect_contains "$RESPONSE" 'Feature change' "compare diff includes change"

###############################################################################
section "Tree"
###############################################################################

# Root directory
RESPONSE=$(auth_curl "$API/repos/$REPO_ID/tree/main/")
expect_contains "$RESPONSE" '"name":"README.md"' "root contains README.md"
expect_contains "$RESPONSE" '"name":"src"' "root contains src dir"
expect_contains "$RESPONSE" '"type":"dir"' "has dir type"
expect_contains "$RESPONSE" '"type":"file"' "has file type"

# Subdirectory
RESPONSE=$(auth_curl "$API/repos/$REPO_ID/tree/main/src")
expect_contains "$RESPONSE" '"name":"main.go"' "src/ contains main.go"

# Tag ref
RESPONSE=$(auth_curl "$API/repos/$REPO_ID/tree/v1.0.0/")
expect_contains "$RESPONSE" '"name":"README.md"' "tag ref works"

###############################################################################
section "Tree Depth"
###############################################################################

# Default depth=1 (no children expanded)
RESPONSE=$(auth_curl "$API/repos/$REPO_ID/tree/main/")
HAS_CHILDREN=$(echo "$RESPONSE" | jq '[.data[] | select(.type == "dir") | .children // [] | length] | add // 0')
if [ "$HAS_CHILDREN" = "0" ]; then
    pass "default depth=1: directories not expanded"
else
    fail "default depth=1: directories not expanded" "0 children" "$HAS_CHILDREN"
fi

# Depth=2 expands one level
RESPONSE=$(auth_curl "$API/repos/$REPO_ID/tree/main/?depth=2")
expect_contains "$RESPONSE" '"children"' "depth=2: has children array"
expect_contains "$RESPONSE" '"has_children"' "depth=2: has has_children field"

# Check src directory has children expanded
SRC_CHILDREN=$(echo "$RESPONSE" | jq '[.data[] | select(.name == "src") | .children | length] | .[0] // 0')
if [ "$SRC_CHILDREN" -ge "1" ]; then
    pass "depth=2: src directory has children"
else
    fail "depth=2: src directory has children" ">=1" "$SRC_CHILDREN"
fi

# Verify main.go is in src's children
RESPONSE=$(auth_curl "$API/repos/$REPO_ID/tree/main/?depth=2")
expect_contains "$RESPONSE" '"name":"main.go"' "depth=2: main.go in src children"

# Depth=0 behaves like depth=1 (default)
RESPONSE=$(auth_curl "$API/repos/$REPO_ID/tree/main/?depth=0")
HAS_CHILDREN=$(echo "$RESPONSE" | jq '[.data[] | select(.type == "dir") | .children // [] | length] | add // 0')
if [ "$HAS_CHILDREN" = "0" ]; then
    pass "depth=0: treated as default (no expansion)"
else
    fail "depth=0: treated as default (no expansion)" "0 children" "$HAS_CHILDREN"
fi

# Large depth is capped at max (10)
RESPONSE=$(auth_curl "$API/repos/$REPO_ID/tree/main/?depth=100")
expect_contains "$RESPONSE" '"name":"README.md"' "depth=100: works (capped at max)"

###############################################################################
section "README"
###############################################################################

# Get README
RESPONSE=$(auth_curl "$API/repos/$REPO_ID/readme")
expect_json "$RESPONSE" '.data.filename' "README.md" "readme returns filename"
expect_contains "$RESPONSE" 'Test Repository' "readme has content"
expect_json "$RESPONSE" '.data.is_binary' "false" "readme is not binary"

# Get README with ref
RESPONSE=$(auth_curl "$API/repos/$REPO_ID/readme?ref=main")
expect_contains "$RESPONSE" 'Test Repository' "readme with ref works"

# Get README with tag ref
RESPONSE=$(auth_curl "$API/repos/$REPO_ID/readme?ref=v1.0.0")
expect_contains "$RESPONSE" 'Test Repository' "readme with tag ref works"

# Empty repo has no readme
RESPONSE=$(auth_curl "$API/repos/$EMPTY_REPO_ID/readme")
expect_contains "$RESPONSE" "Repository is empty" "empty repo readme returns empty error"

###############################################################################
section "Blob"
###############################################################################

# Text file
RESPONSE=$(auth_curl "$API/repos/$REPO_ID/blob/main/README.md")
expect_json "$RESPONSE" '.data.encoding' "utf-8" "README.md encoding=utf-8"
expect_json "$RESPONSE" '.data.is_binary' "false" "README.md is_binary=false"
expect_contains "$RESPONSE" 'Test Repository' "README.md has content"

# Go file
RESPONSE=$(auth_curl "$API/repos/$REPO_ID/blob/main/src/main.go")
expect_contains "$RESPONSE" 'package main' "main.go has content"

# Binary file
RESPONSE=$(auth_curl "$API/repos/$REPO_ID/blob/main/binary.dat")
expect_json "$RESPONSE" '.data.encoding' "base64" "binary.dat encoding=base64"
expect_json "$RESPONSE" '.data.is_binary' "true" "binary.dat is_binary=true"

# Raw mode
RESPONSE=$(auth_curl "$API/repos/$REPO_ID/blob/main/README.md?raw=true")
expect_contains "$RESPONSE" "Test Repository" "raw mode returns content"

CONTENT_TYPE=$(auth_curl -o /dev/null -w "%{content_type}" "$API/repos/$REPO_ID/blob/main/README.md?raw=true")
if [ "$CONTENT_TYPE" = "text/markdown; charset=utf-8" ]; then
    pass "raw mode content-type set"
else
    fail "raw mode content-type set" "text/markdown; charset=utf-8" "$CONTENT_TYPE"
fi

###############################################################################
section "Blame"
###############################################################################

RESPONSE=$(auth_curl "$API/repos/$REPO_ID/blame/main/README.md")
LINE_COUNT=$(echo "$RESPONSE" | jq '.data.lines | length')
if [ "$LINE_COUNT" -ge "1" ]; then
    pass "blame returns lines"
else
    fail "blame returns lines" ">=1" "$LINE_COUNT"
fi
expect_contains "$RESPONSE" "$MAIN_HEAD_SHA" "blame includes latest commit"

###############################################################################
section "Archive"
###############################################################################

CONTENT_TYPE=$(auth_curl -o /dev/null -w "%{content_type}" "$API/repos/$REPO_ID/archive/main?format=zip")
if [ "$CONTENT_TYPE" = "application/zip" ]; then
    pass "archive zip content-type"
else
    fail "archive zip content-type" "application/zip" "$CONTENT_TYPE"
fi

CONTENT_TYPE=$(auth_curl -o /dev/null -w "%{content_type}" "$API/repos/$REPO_ID/archive/main?format=tar.gz")
if [ "$CONTENT_TYPE" = "application/gzip" ]; then
    pass "archive tar.gz content-type"
else
    fail "archive tar.gz content-type" "application/gzip" "$CONTENT_TYPE"
fi

###############################################################################
section "Error Cases"
###############################################################################

# Invalid ref type
RESPONSE=$(auth_curl -X POST -H "Content-Type: application/json" \
    -d '{"name":"bad-ref","type":"unknown"}' \
    "$API/repos/$REPO_ID/refs")
expect_contains "$RESPONSE" "Invalid ref type" "invalid ref type rejected"

# Missing path history
RESPONSE=$(auth_curl "$API/repos/$REPO_ID/commits?ref=main&path=missing.txt")
expect_contains "$RESPONSE" 'not found' "missing history path returns error"

# Invalid archive format
RESPONSE=$(auth_curl "$API/repos/$REPO_ID/archive/main?format=rar")
expect_contains "$RESPONSE" "Invalid archive format" "invalid archive format rejected"

# Non-existent repo
RESPONSE=$(auth_curl "$API/repos/nonexistent/refs")
expect_contains "$RESPONSE" '"error"' "non-existent repo returns error"

# Invalid ref
RESPONSE=$(auth_curl "$API/repos/$REPO_ID/tree/invalid-branch/")
expect_contains "$RESPONSE" 'not found' "invalid ref returns not found"

# Path not found
RESPONSE=$(auth_curl "$API/repos/$REPO_ID/blob/main/nonexistent.txt")
expect_contains "$RESPONSE" 'not found' "missing path returns not found"

# Directory for blob endpoint
RESPONSE=$(auth_curl "$API/repos/$REPO_ID/blob/main/src")
expect_contains "$RESPONSE" 'directory' "dir path for blob returns error"

# File for tree endpoint
RESPONSE=$(auth_curl "$API/repos/$REPO_ID/tree/main/README.md")
expect_contains "$RESPONSE" 'file' "file path for tree returns error"

###############################################################################
section "Public Repo Access"
###############################################################################

# Make repo public
auth_curl -X PATCH -H "Content-Type: application/json" \
    -d '{"public":true}' \
    "$API/repos/$REPO_ID" > /dev/null

# Anonymous access should work
RESPONSE=$(anon_curl "$API/repos/$REPO_ID/refs")
expect_contains "$RESPONSE" '"name":"main"' "public: anonymous refs access"

RESPONSE=$(anon_curl "$API/repos/$REPO_ID/commits")
expect_contains "$RESPONSE" '"sha"' "public: anonymous commits access"

RESPONSE=$(anon_curl "$API/repos/$REPO_ID/commits/$MAIN_HEAD_SHA")
expect_contains "$RESPONSE" "$MAIN_HEAD_SHA" "public: anonymous commit detail"

RESPONSE=$(anon_curl "$API/repos/$REPO_ID/tree/main/")
expect_contains "$RESPONSE" '"name":"README.md"' "public: anonymous tree access"

RESPONSE=$(anon_curl "$API/repos/$REPO_ID/blob/main/README.md")
expect_contains "$RESPONSE" 'Test Repository' "public: anonymous blob access"

RESPONSE=$(anon_curl "$API/repos/$REPO_ID/readme")
expect_contains "$RESPONSE" 'Test Repository' "public: anonymous readme access"

CONTENT_TYPE=$(anon_curl -o /dev/null -w "%{content_type}" "$API/repos/$REPO_ID/archive/main?format=zip")
if [ "$CONTENT_TYPE" = "application/zip" ]; then
    pass "public: anonymous archive access"
else
    fail "public: anonymous archive access" "application/zip" "$CONTENT_TYPE"
fi

###############################################################################
section "Private Repo Access"
###############################################################################

# Make repo private
auth_curl -X PATCH -H "Content-Type: application/json" \
    -d '{"public":false}' \
    "$API/repos/$REPO_ID" > /dev/null

# Anonymous access should fail
RESPONSE=$(anon_curl "$API/repos/$REPO_ID/refs")
expect_contains "$RESPONSE" 'Authentication required' "private: anonymous refs denied"

RESPONSE=$(anon_curl "$API/repos/$REPO_ID/tree/main/")
expect_contains "$RESPONSE" 'Authentication required' "private: anonymous tree denied"

# Authenticated access should work
RESPONSE=$(auth_curl "$API/repos/$REPO_ID/refs")
expect_contains "$RESPONSE" '"name":"main"' "private: authenticated access works"

###############################################################################
summary
