#!/bin/bash
#
# Upload binary releases to updates.mysoc.ai
#
# Usage:
#   ./upload-release.sh <product> <version> <binary-path> [arch]
#
# Examples:
#   ./upload-release.sh siemcore v1.5.0 ./siemcore-linux-amd64
#   ./upload-release.sh siemcore v1.5.0 ./siemcore-linux-arm64
#   ./upload-release.sh mysoc v2.0.0 ./mysoc-linux-amd64
#

set -e

# Colors
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
CYAN='\033[0;36m'
NC='\033[0m'

# Configuration
UPDATE_SERVER="${UPDATE_SERVER:-https://updates.mysoc.ai}"
ADMIN_API_KEY="${ADMIN_API_KEY:-}"

# Functions
log_info() { echo -e "${CYAN}→${NC} $1"; }
log_success() { echo -e "${GREEN}✓${NC} $1"; }
log_error() { echo -e "${RED}✗${NC} $1"; }
log_warn() { echo -e "${YELLOW}⚠${NC} $1"; }

print_usage() {
    echo "Usage: $0 <product> <version> <binary-path>"
    echo ""
    echo "Arguments:"
    echo "  product      Product name (e.g., siemcore, mysoc)"
    echo "  version      Version string (e.g., v1.5.0)"
    echo "  binary-path  Path to the binary file to upload"
    echo ""
    echo "Environment variables:"
    echo "  UPDATE_SERVER   Update server URL (default: https://updates.mysoc.ai)"
    echo "  ADMIN_API_KEY   Admin API key for authentication (required)"
    echo ""
    echo "Examples:"
    echo "  ADMIN_API_KEY=xxx ./upload-release.sh siemcore v1.5.0 ./bin/siemcore-linux-amd64"
    echo "  ADMIN_API_KEY=xxx ./upload-release.sh siemcore v1.5.0 ./bin/siemcore-linux-arm64"
}

# Validate arguments
if [ $# -lt 3 ]; then
    print_usage
    exit 1
fi

PRODUCT="$1"
VERSION="$2"
BINARY_PATH="$3"

# Validate API key
if [ -z "$ADMIN_API_KEY" ]; then
    log_error "ADMIN_API_KEY environment variable is required"
    echo ""
    echo "Set it with: export ADMIN_API_KEY=your-admin-key"
    exit 1
fi

# Validate binary exists
if [ ! -f "$BINARY_PATH" ]; then
    log_error "Binary not found: $BINARY_PATH"
    exit 1
fi

# Get filename from path
FILENAME=$(basename "$BINARY_PATH")
FILESIZE=$(stat -f%z "$BINARY_PATH" 2>/dev/null || stat -c%s "$BINARY_PATH" 2>/dev/null)

# Calculate checksum
log_info "Calculating SHA256 checksum..."
if command -v sha256sum &> /dev/null; then
    CHECKSUM=$(sha256sum "$BINARY_PATH" | awk '{print $1}')
elif command -v shasum &> /dev/null; then
    CHECKSUM=$(shasum -a 256 "$BINARY_PATH" | awk '{print $1}')
else
    log_warn "Neither sha256sum nor shasum found, skipping checksum"
    CHECKSUM=""
fi

echo ""
echo "┌─────────────────────────────────────────────────────────────┐"
echo "│                    Upload Summary                           │"
echo "├─────────────────────────────────────────────────────────────┤"
printf "│ %-15s %-43s │\n" "Product:" "$PRODUCT"
printf "│ %-15s %-43s │\n" "Version:" "$VERSION"
printf "│ %-15s %-43s │\n" "Filename:" "$FILENAME"
printf "│ %-15s %-43s │\n" "Size:" "$(numfmt --to=iec-i --suffix=B $FILESIZE 2>/dev/null || echo "$FILESIZE bytes")"
printf "│ %-15s %-43s │\n" "Checksum:" "${CHECKSUM:0:32}..."
printf "│ %-15s %-43s │\n" "Server:" "$UPDATE_SERVER"
echo "└─────────────────────────────────────────────────────────────┘"
echo ""

# Upload binary
log_info "Uploading $FILENAME to $UPDATE_SERVER..."

UPLOAD_URL="${UPDATE_SERVER}/api/v1/releases/${PRODUCT}/${VERSION}/${FILENAME}"

HTTP_RESPONSE=$(curl -s -w "\n%{http_code}" \
    -X PUT \
    -H "X-API-Key: $ADMIN_API_KEY" \
    -H "Content-Type: application/octet-stream" \
    --data-binary "@$BINARY_PATH" \
    "$UPLOAD_URL")

HTTP_BODY=$(echo "$HTTP_RESPONSE" | sed '$d')
HTTP_CODE=$(echo "$HTTP_RESPONSE" | tail -1)

if [ "$HTTP_CODE" -eq 200 ]; then
    log_success "Binary uploaded successfully!"
    echo ""
    echo "Download URL:"
    echo -e "  ${CYAN}${UPDATE_SERVER}/${PRODUCT}/${VERSION}/${FILENAME}${NC}"
    echo ""
    
    # Upload checksum file if we have one
    if [ -n "$CHECKSUM" ]; then
        log_info "Uploading checksum file..."
        echo "$CHECKSUM  $FILENAME" > "/tmp/${FILENAME}.sha256"
        
        CHECKSUM_RESPONSE=$(curl -s -w "\n%{http_code}" \
            -X PUT \
            -H "X-API-Key: $ADMIN_API_KEY" \
            -H "Content-Type: text/plain" \
            --data-binary "@/tmp/${FILENAME}.sha256" \
            "${UPDATE_SERVER}/api/v1/releases/${PRODUCT}/${VERSION}/${FILENAME}.sha256")
        
        CHECKSUM_CODE=$(echo "$CHECKSUM_RESPONSE" | tail -1)
        if [ "$CHECKSUM_CODE" -eq 200 ]; then
            log_success "Checksum file uploaded"
            echo "  ${UPDATE_SERVER}/${PRODUCT}/${VERSION}/${FILENAME}.sha256"
        else
            log_warn "Failed to upload checksum file"
        fi
        rm -f "/tmp/${FILENAME}.sha256"
    fi
else
    log_error "Upload failed with HTTP $HTTP_CODE"
    echo "$HTTP_BODY"
    exit 1
fi

echo ""
log_success "Release upload complete!"
