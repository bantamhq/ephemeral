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

git clone -q "http://x-token:$TOKEN@${BASE_URL#http://}/git/default/test-content-api.git" repo 2>/dev/null
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

# Create tag
git tag v1.0.0
git push -q origin v1.0.0 2>/dev/null

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
section "Error Cases"
###############################################################################

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

RESPONSE=$(anon_curl "$API/repos/$REPO_ID/tree/main/")
expect_contains "$RESPONSE" '"name":"README.md"' "public: anonymous tree access"

RESPONSE=$(anon_curl "$API/repos/$REPO_ID/blob/main/README.md")
expect_contains "$RESPONSE" 'Test Repository' "public: anonymous blob access"

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
