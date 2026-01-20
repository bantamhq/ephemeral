#!/bin/bash
# Shared test functions for API tests

BASE_URL="${BASE_URL:-http://localhost:8080}"
API="$BASE_URL/api/v1"

# Colors
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m'

# Counters
PASS=0
FAIL=0

# Track created resources for cleanup
CREATED_REPOS=""
CREATED_TOKENS=""
CREATED_FOLDERS=""
CREATED_LABELS=""
CREATED_NAMESPACES=""

pass() {
    echo -e "  ${GREEN}✓${NC} $1"
    PASS=$((PASS + 1))
}

fail() {
    echo -e "  ${RED}✗${NC} $1"
    echo "    Expected: $2"
    echo "    Got: $3"
    FAIL=$((FAIL + 1))
}

section() {
    echo ""
    echo -e "${YELLOW}$1${NC}"
}

info() {
    echo -e "${BLUE}→${NC} $1"
}

auth_curl() {
    curl -s -H "Authorization: Bearer $TOKEN" "$@"
}

# auth_curl_with takes a token as first arg, rest are passed to curl
auth_curl_with() {
    local token="$1"
    shift
    curl -s -H "Authorization: Bearer $token" "$@"
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

expect_json_length() {
    local response="$1"
    local jq_query="$2"
    local expected="$3"
    local test_name="$4"

    local actual=$(echo "$response" | jq "$jq_query | length" 2>/dev/null)
    if [ "$actual" -ge "$expected" ]; then
        pass "$test_name"
    else
        fail "$test_name" ">=$expected" "$actual"
    fi
}

# Extract ID from response (.data.ID or .data.id)
get_id() {
    local response="$1"
    local id=$(echo "$response" | jq -r '.data.ID // .data.id' 2>/dev/null)
    if [ "$id" = "null" ] || [ -z "$id" ]; then
        echo ""
    else
        echo "$id"
    fi
}

# Track resource for cleanup
track_repo() {
    CREATED_REPOS="$CREATED_REPOS $1"
}

track_token() {
    CREATED_TOKENS="$CREATED_TOKENS $1"
}

track_folder() {
    CREATED_FOLDERS="$CREATED_FOLDERS $1"
}

track_label() {
    CREATED_LABELS="$CREATED_LABELS $1"
}

track_namespace() {
    CREATED_NAMESPACES="$CREATED_NAMESPACES $1"
}

# Cleanup all tracked resources
cleanup() {
    echo ""
    section "Cleanup"

    for id in $CREATED_REPOS; do
        auth_curl -X DELETE "$API/repos/$id" > /dev/null 2>&1 || true
        info "Deleted repo: $id"
    done

    for id in $CREATED_TOKENS; do
        auth_curl -X DELETE "$API/tokens/$id" > /dev/null 2>&1 || true
        info "Deleted token: $id"
    done

    for id in $CREATED_FOLDERS; do
        auth_curl -X DELETE "$API/folders/$id?force=true" > /dev/null 2>&1 || true
        info "Deleted folder: $id"
    done

    for id in $CREATED_LABELS; do
        auth_curl -X DELETE "$API/labels/$id" > /dev/null 2>&1 || true
        info "Deleted label: $id"
    done

    for id in $CREATED_NAMESPACES; do
        auth_curl -X DELETE "$API/namespaces/$id" > /dev/null 2>&1 || true
        info "Deleted namespace: $id"
    done
}

# Print summary
summary() {
    echo ""
    echo "========================================"
    echo -e "Results: ${GREEN}$PASS passed${NC}, ${RED}$FAIL failed${NC}"
    echo "========================================"

    if [ $FAIL -gt 0 ]; then
        return 1
    fi
    return 0
}

# Check token is set
require_token() {
    if [ -z "$TOKEN" ]; then
        echo "Error: TOKEN environment variable is required"
        echo "Usage: TOKEN=eph_xxx $0"
        exit 1
    fi
}
