#!/bin/bash
# System API Tests
set -e

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
source "$SCRIPT_DIR/lib.sh"

require_token

echo ""
echo -e "${BLUE}═══════════════════════════════════════${NC}"
echo -e "${BLUE}  System API Tests${NC}"
echo -e "${BLUE}═══════════════════════════════════════${NC}"

###############################################################################
section "Health"
###############################################################################

RESPONSE=$(anon_curl "$BASE_URL/health")
expect_contains "$RESPONSE" "OK" "health returns OK"

###############################################################################
section "Auth Errors"
###############################################################################

RESPONSE=$(curl -s -H "Authorization: Token $TOKEN" "$API/repos")
expect_contains "$RESPONSE" "Invalid authorization scheme" "invalid scheme rejected"

RESPONSE=$(curl -s -H "Authorization: Bearer not-a-token" "$API/repos")
expect_contains "$RESPONSE" "Invalid token format" "invalid token format rejected"

###############################################################################
summary
