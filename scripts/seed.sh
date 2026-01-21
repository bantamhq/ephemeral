#!/bin/bash
# Seed script for testing TUI with realistic data
# Usage: ./seed.sh <token>

set -e

TOKEN="${1:-$EPHEMERAL_TOKEN}"
BASE_URL="${EPHEMERAL_SERVER:-http://localhost:8080}"

if [ -z "$TOKEN" ]; then
    echo "Usage: ./seed.sh <token>"
    echo "Or set EPHEMERAL_TOKEN environment variable"
    exit 1
fi

API="$BASE_URL/api/v1"
AUTH_HEADER="Authorization: Bearer $TOKEN"

echo "Seeding data at $BASE_URL..."

# Helper function for API calls
api() {
    local method=$1
    local path=$2
    local data=$3

    if [ -n "$data" ]; then
        curl -s -X "$method" -H "$AUTH_HEADER" -H "Content-Type: application/json" -d "$data" "$API$path"
    else
        curl -s -X "$method" -H "$AUTH_HEADER" "$API$path"
    fi
}

get_id() {
    grep -o '"id":"[^"]*"' | head -1 | cut -d'"' -f4
}

###############################################################################
# Folders (flat with colors)
###############################################################################
echo ""
echo "Creating folders..."

# Project categories
WEB=$(api POST /folders '{"name": "web", "color": "#61DAFB"}' | get_id)
echo "  web (blue)"

MOBILE=$(api POST /folders '{"name": "mobile", "color": "#FA7343"}' | get_id)
echo "  mobile (orange)"

BACKEND=$(api POST /folders '{"name": "backend", "color": "#00ADD8"}' | get_id)
echo "  backend (cyan)"

INFRA=$(api POST /folders '{"name": "infra", "color": "#326CE5"}' | get_id)
echo "  infra (blue)"

# Library categories
INTERNAL=$(api POST /folders '{"name": "internal-libs", "color": "#10B981"}' | get_id)
echo "  internal-libs (green)"

OPENSOURCE=$(api POST /folders '{"name": "open-source", "color": "#F59E0B"}' | get_id)
echo "  open-source (amber)"

# Other categories
EXPERIMENTS=$(api POST /folders '{"name": "experiments", "color": "#8B5CF6"}' | get_id)
echo "  experiments (purple)"

ARCHIVE=$(api POST /folders '{"name": "archive", "color": "#6B7280"}' | get_id)
echo "  archive (gray)"

DOCS=$(api POST /folders '{"name": "docs", "color": "#3B82F6"}' | get_id)
echo "  docs (blue)"

# Language/framework specific folders
GO=$(api POST /folders '{"name": "go", "color": "#00ADD8"}' | get_id)
echo "  go (cyan)"

TYPESCRIPT=$(api POST /folders '{"name": "typescript", "color": "#3178C6"}' | get_id)
echo "  typescript (blue)"

RUST=$(api POST /folders '{"name": "rust", "color": "#DEA584"}' | get_id)
echo "  rust (orange)"

PYTHON=$(api POST /folders '{"name": "python", "color": "#3776AB"}' | get_id)
echo "  python (blue)"

###############################################################################
# Repos
###############################################################################
echo ""
echo "Creating repos..."

create_repo() {
    local name=$1
    local public=$2
    local desc=$3
    shift 3
    local folders=("$@")

    local public_flag="false"
    [ "$public" = "public" ] && public_flag="true"

    local json="{\"name\": \"$name\", \"public\": $public_flag"
    if [ -n "$desc" ]; then
        json="$json, \"description\": \"$desc\""
    fi
    json="$json}"

    local id=$(api POST /repos "$json" | get_id)

    if [ ${#folders[@]} -gt 0 ]; then
        local folder_json=$(printf ',"%s"' "${folders[@]}")
        folder_json="[${folder_json:1}]"
        api PUT "/repos/$id/folders" "{\"folder_ids\": $folder_json}" > /dev/null
    fi

    echo "  $name"
}

# Web projects (6 repos)
create_repo "dashboard" private "Internal analytics and metrics dashboard" "$WEB" "$TYPESCRIPT"
create_repo "marketing-site" public "Company marketing website with CMS integration" "$WEB" "$TYPESCRIPT"
create_repo "admin-panel" private "" "$WEB" "$TYPESCRIPT"
create_repo "docs-site" public "Product documentation built with Docusaurus" "$WEB" "$DOCS" "$TYPESCRIPT"
create_repo "landing-page" public "" "$WEB"
create_repo "component-library" private "Shared React component library with Storybook" "$WEB" "$TYPESCRIPT" "$OPENSOURCE"

# Mobile projects (4 repos)
create_repo "ios-app" private "Native iOS app built with SwiftUI" "$MOBILE"
create_repo "android-app" private "" "$MOBILE"
create_repo "react-native-app" private "Cross-platform mobile app for customers" "$MOBILE" "$TYPESCRIPT"
create_repo "flutter-prototype" private "" "$MOBILE" "$EXPERIMENTS"

# Backend projects (5 repos)
create_repo "api-gateway" private "Central API gateway with rate limiting and auth" "$BACKEND" "$GO"
create_repo "auth-service" private "OAuth2 and JWT authentication service" "$BACKEND" "$GO"
create_repo "notification-service" private "" "$BACKEND" "$GO"
create_repo "analytics-pipeline" private "Real-time event processing with Apache Kafka" "$BACKEND" "$PYTHON"
create_repo "search-service" private "" "$BACKEND" "$RUST"

# Infra projects (4 repos)
create_repo "terraform-modules" private "Reusable Terraform modules for AWS infrastructure" "$INFRA"
create_repo "k8s-manifests" private "" "$INFRA"
create_repo "ci-pipelines" private "GitHub Actions workflows and reusable actions" "$INFRA"
create_repo "monitoring-stack" private "" "$INFRA"

# Internal libraries (3 repos)
create_repo "go-kit" private "Common Go utilities, middleware, and patterns" "$INTERNAL" "$GO"
create_repo "ts-utils" private "" "$INTERNAL" "$TYPESCRIPT"
create_repo "python-common" private "Shared Python utilities and data models" "$INTERNAL" "$PYTHON"

# Open source libraries (3 repos)
create_repo "http-client" public "Lightweight HTTP client with retry and circuit breaker" "$OPENSOURCE" "$GO"
create_repo "react-hooks" public "" "$OPENSOURCE" "$TYPESCRIPT"
create_repo "cli-builder" public "Framework for building beautiful CLI applications" "$OPENSOURCE" "$RUST"

# Experiments (3 repos)
create_repo "raytracer" public "Weekend raytracer project following Peter Shirley's book" "$EXPERIMENTS" "$RUST"
create_repo "ml-sandbox" private "" "$EXPERIMENTS" "$PYTHON"
create_repo "wasm-playground" private "" "$EXPERIMENTS" "$RUST"

# Archive (2 repos)
create_repo "legacy-api" private "Deprecated v1 API (migrated to api-gateway)" "$ARCHIVE" "$GO"
create_repo "old-website" private "" "$ARCHIVE" "$TYPESCRIPT"

# Root level repos (no folder) - 5 repos
create_repo "dotfiles" public "Personal dotfiles for macOS and Linux"
create_repo "notes" private ""
create_repo "blog" public "Personal blog built with Next.js" "$TYPESCRIPT"
create_repo "resume" private "" "$DOCS"
create_repo "scripts" private "Collection of useful shell and Python scripts" "$PYTHON"

###############################################################################
# Push pre-made repos with commit history
###############################################################################
echo ""
echo "Adding commit history to repos..."

# Get primary namespace from user's accessible namespaces
NS_JSON=$(api GET /namespaces)
NS_NAME=$(echo "$NS_JSON" | jq -r '.data[] | select(.is_primary == true) | .name' 2>/dev/null)
if [ -z "$NS_NAME" ]; then
    # Fall back to first namespace if no primary found
    NS_NAME=$(echo "$NS_JSON" | jq -r '.data[0].name' 2>/dev/null)
fi
SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
SEED_REPOS="$SCRIPT_DIR/seed-repos"
GIT_HOST="${BASE_URL#http://}"
GIT_HOST="${GIT_HOST#https://}"
GIT_SCHEME="http"
if [[ "$BASE_URL" == https://* ]]; then
    GIT_SCHEME="https"
fi

push_seed_repo() {
    local name=$1
    local src="$SEED_REPOS/$name.git"
    local temp_dir
    local repo_dir

    if [ ! -d "$src" ]; then
        echo "  $name (seed repo not found, skipping)"
        return
    fi

    temp_dir=$(mktemp -d)
    repo_dir="$temp_dir/$name"
    git clone -q "$src" "$repo_dir"

    git -C "$repo_dir" remote set-url origin "$GIT_SCHEME://x-token:$TOKEN@$GIT_HOST/git/$NS_NAME/$name.git"
    git -C "$repo_dir" push -q origin --all
    git -C "$repo_dir" push -q origin --tags
    rm -rf "$temp_dir"
    echo "  $name"
}

push_seed_repo "admin-panel"
push_seed_repo "analytics-pipeline"

echo ""
echo "Seed complete!"
echo ""
echo "Summary:"
echo "  Folders: 13"
echo "  Repos: 35"
echo "  Repos with history: 2"
echo ""
echo "Run the TUI to see the data: eph"
