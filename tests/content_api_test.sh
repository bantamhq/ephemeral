#!/bin/bash
set -e

# Content API Test Suite
# Usage: ./content_api_test.sh [token]
#        TOKEN=xxx ./content_api_test.sh
#        ./content_api_test.sh  (will prompt)

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

# Helper to make authenticated requests
auth_curl() {
    curl -s -u "x-token:$TOKEN" "$@"
}

# Helper to make unauthenticated requests
anon_curl() {
    curl -s "$@"
}

# Check if response contains expected string
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

# Check if response has expected JSON field value
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

# Check HTTP status code
expect_status() {
    local status="$1"
    local expected="$2"
    local test_name="$3"

    if [ "$status" = "$expected" ]; then
        pass "$test_name"
    else
        fail "$test_name" "status $expected" "status $status"
    fi
}

###############################################################################
# SETUP
###############################################################################

section "Setup"

# Clean up any leftover test repo from previous runs
echo "Cleaning up previous test repo (if any)..."
OLD_REPO=$(auth_curl "$BASE_URL/api/v1/repos" | jq -r '.data[] | select(.Name=="test-content-api") | .ID' 2>/dev/null || true)
if [ -n "$OLD_REPO" ] && [ "$OLD_REPO" != "null" ]; then
    auth_curl -X DELETE "$BASE_URL/api/v1/repos/$OLD_REPO" > /dev/null
    echo "Deleted old test repo: $OLD_REPO"
fi

# Create test repo
echo "Creating test repository..."
REPO_RESPONSE=$(auth_curl -X POST \
    -H "Content-Type: application/json" \
    -d '{"name":"test-content-api","public":false}' \
    "$BASE_URL/api/v1/repos")

REPO_ID=$(echo "$REPO_RESPONSE" | jq -r '.data.ID')

if [ "$REPO_ID" = "null" ] || [ -z "$REPO_ID" ]; then
    echo "Failed to create repo: $REPO_RESPONSE"
    exit 1
fi
echo "Created repo: $REPO_ID"

# Clone and add content
TMPDIR=$(mktemp -d)
cd "$TMPDIR"

git clone -q "http://x-token:$TOKEN@${BASE_URL#http://}/git/default/test-content-api.git" repo
cd repo

# Create test files
echo "# Test Repository" > README.md
mkdir -p src
echo 'package main

import "fmt"

func main() {
    fmt.Println("Hello")
}' > src/main.go

mkdir -p docs
echo "Documentation" > docs/index.md

# Binary file
printf '\x00\x01\x02\x03' > binary.dat

git add .
git commit -q -m "Initial commit"
git push -q origin main

# Second commit
echo "More content" >> README.md
git add .
git commit -q -m "Update README"
git push -q origin main

# Create tag
git tag v1.0.0
git push -q origin v1.0.0

cd /
rm -rf "$TMPDIR"

echo "Test content pushed"

###############################################################################
# REFS ENDPOINT
###############################################################################

section "GET /api/v1/repos/{id}/refs"

RESPONSE=$(auth_curl "$BASE_URL/api/v1/repos/$REPO_ID/refs")

# Should have main branch
expect_contains "$RESPONSE" '"name":"main"' "refs: contains main branch"
expect_contains "$RESPONSE" '"type":"branch"' "refs: has branch type"

# Should have tag
expect_contains "$RESPONSE" '"name":"v1.0.0"' "refs: contains v1.0.0 tag"
expect_contains "$RESPONSE" '"type":"tag"' "refs: has tag type"

# Main should be default
expect_json "$RESPONSE" '.data[0].name' "main" "refs: main is first (default)"
expect_json "$RESPONSE" '.data[0].is_default' "true" "refs: main is_default=true"

###############################################################################
# COMMITS ENDPOINT
###############################################################################

section "GET /api/v1/repos/{id}/commits"

# Default (no ref)
RESPONSE=$(auth_curl "$BASE_URL/api/v1/repos/$REPO_ID/commits")
expect_contains "$RESPONSE" '"message":"Update README' "commits: default ref returns commits"

# Explicit ref
RESPONSE=$(auth_curl "$BASE_URL/api/v1/repos/$REPO_ID/commits?ref=main")
expect_contains "$RESPONSE" '"message":"Update README' "commits: ref=main works"

# Pagination
RESPONSE=$(auth_curl "$BASE_URL/api/v1/repos/$REPO_ID/commits?limit=1")
expect_json "$RESPONSE" '.has_more' "true" "commits: limit=1 has_more=true"
expect_json "$RESPONSE" '.data | length' "1" "commits: limit=1 returns 1 commit"

# Tag ref
RESPONSE=$(auth_curl "$BASE_URL/api/v1/repos/$REPO_ID/commits?ref=v1.0.0")
expect_contains "$RESPONSE" '"sha"' "commits: tag ref works"

###############################################################################
# TREE ENDPOINT
###############################################################################

section "GET /api/v1/repos/{id}/tree/{ref}/*"

# Root directory
RESPONSE=$(auth_curl "$BASE_URL/api/v1/repos/$REPO_ID/tree/main/")
expect_contains "$RESPONSE" '"name":"README.md"' "tree: root contains README.md"
expect_contains "$RESPONSE" '"name":"src"' "tree: root contains src dir"
expect_contains "$RESPONSE" '"type":"dir"' "tree: has dir type"
expect_contains "$RESPONSE" '"type":"file"' "tree: has file type"

# Subdirectory
RESPONSE=$(auth_curl "$BASE_URL/api/v1/repos/$REPO_ID/tree/main/src")
expect_contains "$RESPONSE" '"name":"main.go"' "tree: src/ contains main.go"

# Tag ref
RESPONSE=$(auth_curl "$BASE_URL/api/v1/repos/$REPO_ID/tree/v1.0.0/")
expect_contains "$RESPONSE" '"name":"README.md"' "tree: tag ref works"

###############################################################################
# BLOB ENDPOINT
###############################################################################

section "GET /api/v1/repos/{id}/blob/{ref}/*"

# Text file
RESPONSE=$(auth_curl "$BASE_URL/api/v1/repos/$REPO_ID/blob/main/README.md")
expect_json "$RESPONSE" '.data.encoding' "utf-8" "blob: README.md encoding=utf-8"
expect_json "$RESPONSE" '.data.is_binary' "false" "blob: README.md is_binary=false"
expect_contains "$RESPONSE" 'Test Repository' "blob: README.md has content"

# Go file
RESPONSE=$(auth_curl "$BASE_URL/api/v1/repos/$REPO_ID/blob/main/src/main.go")
expect_contains "$RESPONSE" 'package main' "blob: main.go has content"

# Binary file
RESPONSE=$(auth_curl "$BASE_URL/api/v1/repos/$REPO_ID/blob/main/binary.dat")
expect_json "$RESPONSE" '.data.encoding' "base64" "blob: binary.dat encoding=base64"
expect_json "$RESPONSE" '.data.is_binary' "true" "blob: binary.dat is_binary=true"

# Raw mode
RESPONSE=$(auth_curl "$BASE_URL/api/v1/repos/$REPO_ID/blob/main/README.md?raw=true")
expect_contains "$RESPONSE" "Test Repository" "blob: raw mode returns content"

# Raw content-type (use -i for headers with GET, not -I which sends HEAD)
CONTENT_TYPE=$(curl -s -i -u "x-token:$TOKEN" "$BASE_URL/api/v1/repos/$REPO_ID/blob/main/README.md?raw=true" | grep -i "^content-type:" | tr -d '\r')
expect_contains "$CONTENT_TYPE" "text/markdown" "blob: raw README.md content-type"

###############################################################################
# ERROR CASES
###############################################################################

section "Error Cases"

# Non-existent repo
RESPONSE=$(auth_curl "$BASE_URL/api/v1/repos/nonexistent/refs")
expect_contains "$RESPONSE" '"error"' "error: non-existent repo returns error"

# Invalid ref
RESPONSE=$(auth_curl "$BASE_URL/api/v1/repos/$REPO_ID/tree/invalid-branch/")
expect_contains "$RESPONSE" 'not found' "error: invalid ref returns not found"

# Path not found
RESPONSE=$(auth_curl "$BASE_URL/api/v1/repos/$REPO_ID/blob/main/nonexistent.txt")
expect_contains "$RESPONSE" 'not found' "error: missing path returns not found"

# Directory for blob endpoint
RESPONSE=$(auth_curl "$BASE_URL/api/v1/repos/$REPO_ID/blob/main/src")
expect_contains "$RESPONSE" 'directory' "error: dir path for blob returns error"

# File for tree endpoint
RESPONSE=$(auth_curl "$BASE_URL/api/v1/repos/$REPO_ID/tree/main/README.md")
expect_contains "$RESPONSE" 'file' "error: file path for tree returns error"

###############################################################################
# PUBLIC REPO ACCESS
###############################################################################

section "Public Repo Anonymous Access"

# Make repo public
auth_curl -X PATCH \
    -H "Content-Type: application/json" \
    -d '{"public":true}' \
    "$BASE_URL/api/v1/repos/$REPO_ID" > /dev/null

# Anonymous access should work
RESPONSE=$(anon_curl "$BASE_URL/api/v1/repos/$REPO_ID/refs")
expect_contains "$RESPONSE" '"name":"main"' "public: anonymous refs access"

RESPONSE=$(anon_curl "$BASE_URL/api/v1/repos/$REPO_ID/commits")
expect_contains "$RESPONSE" '"sha"' "public: anonymous commits access"

RESPONSE=$(anon_curl "$BASE_URL/api/v1/repos/$REPO_ID/tree/main/")
expect_contains "$RESPONSE" '"name":"README.md"' "public: anonymous tree access"

RESPONSE=$(anon_curl "$BASE_URL/api/v1/repos/$REPO_ID/blob/main/README.md")
expect_contains "$RESPONSE" 'Test Repository' "public: anonymous blob access"

###############################################################################
# PRIVATE REPO ACCESS
###############################################################################

section "Private Repo Access Control"

# Make repo private
auth_curl -X PATCH \
    -H "Content-Type: application/json" \
    -d '{"public":false}' \
    "$BASE_URL/api/v1/repos/$REPO_ID" > /dev/null

# Anonymous access should fail
RESPONSE=$(anon_curl "$BASE_URL/api/v1/repos/$REPO_ID/refs")
expect_contains "$RESPONSE" 'Authentication required' "private: anonymous refs denied"

RESPONSE=$(anon_curl "$BASE_URL/api/v1/repos/$REPO_ID/tree/main/")
expect_contains "$RESPONSE" 'Authentication required' "private: anonymous tree denied"

# Authenticated access should work
RESPONSE=$(auth_curl "$BASE_URL/api/v1/repos/$REPO_ID/refs")
expect_contains "$RESPONSE" '"name":"main"' "private: authenticated access works"

###############################################################################
# CLEANUP
###############################################################################

section "Cleanup"

auth_curl -X DELETE "$BASE_URL/api/v1/repos/$REPO_ID" > /dev/null
echo "Deleted test repo"

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
