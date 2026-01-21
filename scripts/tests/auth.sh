#!/bin/bash
# Auth API Tests
set -e

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
source "$SCRIPT_DIR/lib.sh"

echo ""
echo -e "${BLUE}═══════════════════════════════════════${NC}"
echo -e "${BLUE}  Auth API Tests${NC}"
echo -e "${BLUE}═══════════════════════════════════════${NC}"

###############################################################################
section "Auth Config"
###############################################################################

RESPONSE=$(anon_curl "$API/auth/config")
expect_contains "$RESPONSE" '"auth_methods"' "auth/config returns auth_methods"
expect_contains "$RESPONSE" '"token"' "auth/config includes token method"
expect_not_contains "$RESPONSE" '"web_auth"' "auth/config excludes web_auth when not configured"
expect_not_contains "$RESPONSE" '"web_auth_url"' "auth/config excludes web_auth_url when not configured"

###############################################################################
section "Auth Exchange (unconfigured)"
###############################################################################

RESPONSE=$(anon_curl -X POST \
    -H "Content-Type: application/json" \
    -d '{"code":"TEST-CODE","code_verifier":"test-verifier"}' \
    "$API/auth/exchange")
expect_contains "$RESPONSE" '"code":"not_configured"' "exchange returns not_configured error code"
expect_contains "$RESPONSE" '"message"' "exchange returns error message"

RESPONSE=$(anon_curl -X POST \
    -H "Content-Type: application/json" \
    -d '{"code":"","code_verifier":"test-verifier"}' \
    "$API/auth/exchange")
expect_contains "$RESPONSE" '"code":"invalid_request"' "exchange validates code is required"

RESPONSE=$(anon_curl -X POST \
    -H "Content-Type: application/json" \
    -d '{"code":"TEST-CODE","code_verifier":""}' \
    "$API/auth/exchange")
expect_contains "$RESPONSE" '"code":"invalid_request"' "exchange validates code_verifier is required"

###############################################################################
summary
