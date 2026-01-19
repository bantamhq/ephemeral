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
AUTH="x-token:$TOKEN"

echo "Seeding data at $BASE_URL..."

# Helper function for API calls
api() {
    local method=$1
    local path=$2
    local data=$3

    if [ -n "$data" ]; then
        curl -s -X "$method" -u "$AUTH" -H "Content-Type: application/json" -d "$data" "$API$path"
    else
        curl -s -X "$method" -u "$AUTH" "$API$path"
    fi
}

get_id() {
    grep -o '"id":"[^"]*"' | head -1 | cut -d'"' -f4
}

###############################################################################
# Folders
###############################################################################
echo ""
echo "Creating folders..."

# Projects hierarchy
PROJECTS=$(api POST /folders '{"name": "projects"}' | get_id)
echo "  projects/"

WEB=$(api POST /folders "{\"name\": \"web\", \"parent_id\": \"$PROJECTS\"}" | get_id)
echo "    web/"

MOBILE=$(api POST /folders "{\"name\": \"mobile\", \"parent_id\": \"$PROJECTS\"}" | get_id)
echo "    mobile/"

BACKEND=$(api POST /folders "{\"name\": \"backend\", \"parent_id\": \"$PROJECTS\"}" | get_id)
echo "    backend/"

INFRA=$(api POST /folders "{\"name\": \"infra\", \"parent_id\": \"$PROJECTS\"}" | get_id)
echo "    infra/"

# Libraries hierarchy
LIBS=$(api POST /folders '{"name": "libraries"}' | get_id)
echo "  libraries/"

INTERNAL=$(api POST /folders "{\"name\": \"internal\", \"parent_id\": \"$LIBS\"}" | get_id)
echo "    internal/"

OPENSOURCE=$(api POST /folders "{\"name\": \"open-source\", \"parent_id\": \"$LIBS\"}" | get_id)
echo "    open-source/"

# Other top-level folders
EXPERIMENTS=$(api POST /folders '{"name": "experiments"}' | get_id)
echo "  experiments/"

ARCHIVE=$(api POST /folders '{"name": "archive"}' | get_id)
echo "  archive/"

DOCS=$(api POST /folders '{"name": "docs"}' | get_id)
echo "  docs/"

###############################################################################
# Labels
###############################################################################
echo ""
echo "Creating labels..."

# Languages
GO=$(api POST /labels '{"name": "go", "color": "#00ADD8"}' | get_id)
echo "  go"

TS=$(api POST /labels '{"name": "typescript", "color": "#3178C6"}' | get_id)
echo "  typescript"

RUST=$(api POST /labels '{"name": "rust", "color": "#DEA584"}' | get_id)
echo "  rust"

PYTHON=$(api POST /labels '{"name": "python", "color": "#3776AB"}' | get_id)
echo "  python"

SWIFT=$(api POST /labels '{"name": "swift", "color": "#FA7343"}' | get_id)
echo "  swift"

KOTLIN=$(api POST /labels '{"name": "kotlin", "color": "#7F52FF"}' | get_id)
echo "  kotlin"

# Frameworks
REACT=$(api POST /labels '{"name": "react", "color": "#61DAFB"}' | get_id)
echo "  react"

VUE=$(api POST /labels '{"name": "vue", "color": "#4FC08D"}' | get_id)
echo "  vue"

SVELTE=$(api POST /labels '{"name": "svelte", "color": "#FF3E00"}' | get_id)
echo "  svelte"

# Categories
API_LABEL=$(api POST /labels '{"name": "api", "color": "#6366F1"}' | get_id)
echo "  api"

CLI=$(api POST /labels '{"name": "cli", "color": "#10B981"}' | get_id)
echo "  cli"

DATABASE=$(api POST /labels '{"name": "database", "color": "#F59E0B"}' | get_id)
echo "  database"

DOCKER=$(api POST /labels '{"name": "docker", "color": "#2496ED"}' | get_id)
echo "  docker"

K8S=$(api POST /labels '{"name": "k8s", "color": "#326CE5"}' | get_id)
echo "  k8s"

DEPRECATED=$(api POST /labels '{"name": "deprecated", "color": "#6B7280"}' | get_id)
echo "  deprecated"

###############################################################################
# Repos
###############################################################################
echo ""
echo "Creating repos..."

create_repo() {
    local name=$1
    local public=$2
    local folder=$3
    shift 3
    local labels=("$@")

    local public_flag="false"
    [ "$public" = "public" ] && public_flag="true"

    local id=$(api POST /repos "{\"name\": \"$name\", \"public\": $public_flag}" | get_id)

    if [ -n "$folder" ]; then
        api PATCH "/repos/$id" "{\"folder_id\": \"$folder\"}" > /dev/null
    fi

    if [ ${#labels[@]} -gt 0 ]; then
        local label_json=$(printf ',"%s"' "${labels[@]}")
        label_json="[${label_json:1}]"
        api POST "/repos/$id/labels" "{\"label_ids\": $label_json}" > /dev/null
    fi

    echo "  $name"
}

# Web projects (6 repos)
create_repo "dashboard" private "$WEB" "$TS" "$REACT"
create_repo "marketing-site" public "$WEB" "$TS" "$SVELTE"
create_repo "admin-panel" private "$WEB" "$TS" "$VUE"
create_repo "docs-site" public "$WEB" "$TS" "$REACT"
create_repo "landing-page" public "$WEB" "$TS"
create_repo "component-library" private "$WEB" "$TS" "$REACT"

# Mobile projects (4 repos)
create_repo "ios-app" private "$MOBILE" "$SWIFT"
create_repo "android-app" private "$MOBILE" "$KOTLIN"
create_repo "react-native-app" private "$MOBILE" "$TS" "$REACT"
create_repo "flutter-prototype" private "$MOBILE"

# Backend projects (5 repos)
create_repo "api-gateway" private "$BACKEND" "$GO" "$API_LABEL" "$DOCKER"
create_repo "auth-service" private "$BACKEND" "$GO" "$API_LABEL" "$DATABASE"
create_repo "notification-service" private "$BACKEND" "$GO" "$API_LABEL"
create_repo "analytics-pipeline" private "$BACKEND" "$PYTHON" "$DATABASE"
create_repo "search-service" private "$BACKEND" "$RUST" "$API_LABEL"

# Infra projects (4 repos)
create_repo "terraform-modules" private "$INFRA" "$DOCKER" "$K8S"
create_repo "k8s-manifests" private "$INFRA" "$K8S"
create_repo "ci-pipelines" private "$INFRA" "$DOCKER"
create_repo "monitoring-stack" private "$INFRA" "$DOCKER" "$K8S"

# Internal libraries (3 repos)
create_repo "go-kit" private "$INTERNAL" "$GO"
create_repo "ts-utils" private "$INTERNAL" "$TS"
create_repo "python-common" private "$INTERNAL" "$PYTHON"

# Open source libraries (3 repos)
create_repo "http-client" public "$OPENSOURCE" "$GO" "$CLI"
create_repo "react-hooks" public "$OPENSOURCE" "$TS" "$REACT"
create_repo "cli-builder" public "$OPENSOURCE" "$RUST" "$CLI"

# Experiments (3 repos)
create_repo "raytracer" public "$EXPERIMENTS" "$RUST"
create_repo "ml-sandbox" private "$EXPERIMENTS" "$PYTHON"
create_repo "wasm-playground" private "$EXPERIMENTS" "$RUST"

# Archive (2 repos)
create_repo "legacy-api" private "$ARCHIVE" "$GO" "$DEPRECATED"
create_repo "old-website" private "$ARCHIVE" "$TS" "$DEPRECATED"

# Root level repos (no folder) - 5 repos
create_repo "dotfiles" public "" "$CLI"
create_repo "notes" private ""
create_repo "blog" public "" "$TS" "$SVELTE"
create_repo "resume" private ""
create_repo "scripts" private "" "$PYTHON" "$CLI"

echo ""
echo "Seed complete!"
echo ""
echo "Summary:"
echo "  Folders: 11"
echo "  Labels: 15"
echo "  Repos: 35"
echo ""
echo "Run the TUI to see the data: ./ephemeral"
