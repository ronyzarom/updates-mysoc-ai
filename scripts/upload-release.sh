#!/bin/bash
# Upload Release to Updates Server
# Usage: ./upload-release.sh --product <name> --version <ver> --file <path> [options]
#
# Examples:
#   ./upload-release.sh --product siemcore --version 3.3.152.57 --file ./dist/siemcore-universal-3.3.152.57.tar.gz --groups alpha
#   ./upload-release.sh --product mysoc --version 1.5.0.3 --file ./mysoc-1.5.0.3.tar.gz --groups alpha

set -e

# Colors
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
CYAN='\033[0;36m'
NC='\033[0m'

# Default values
UPDATE_SERVER="${UPDATE_SERVER:-https://updates.mysoc.ai}"
CHANNEL="stable"
# Not GROUPS: that is a Bash builtin array whose assignments are ignored.
TARGET_GROUPS=""
NOTES=""

# Parse arguments
while [[ $# -gt 0 ]]; do
    case $1 in
        --product|-p)
            PRODUCT="$2"
            shift 2
            ;;
        --version|-v)
            VERSION="$2"
            shift 2
            ;;
        --file|-f)
            FILE="$2"
            shift 2
            ;;
        --channel|-c)
            CHANNEL="$2"
            shift 2
            ;;
        --groups|-g)
            TARGET_GROUPS="$2"
            shift 2
            ;;
        --notes|-n)
            NOTES="$2"
            shift 2
            ;;
        --server|-s)
            UPDATE_SERVER="$2"
            shift 2
            ;;
        --api-key|-k)
            API_KEY="$2"
            shift 2
            ;;
        --help|-h)
            echo "Upload Release to Updates Server"
            echo ""
            echo "Usage: $0 --product <name> --version <ver> --file <path> [options]"
            echo ""
            echo "Required:"
            echo "  --product, -p    Product name (e.g., siemcore, mysoc, swf)"
            echo "  --version, -v    Version, MAJOR.MINOR.PATCH.BUILD (e.g., 3.3.152.57)"
            echo "  --file, -f       Path to the release artifact"
            echo "  --groups, -g     Target groups, comma-separated: alpha, beta, stable, production"
            echo "                   (start with alpha; the server treats no groups as all four)"
            echo ""
            echo "Optional:"
            echo "  --channel, -c    Release channel: stable, beta, nightly (default: stable)"
            echo "  --notes, -n      Release notes"
            echo "  --server, -s     Updates server URL (default: https://updates.mysoc.ai)"
            echo "  --api-key, -k    Releases-scoped API key (or set UPDATES_API_KEY env var)"
            echo ""
            echo "Examples:"
            echo "  # First upload: alpha only"
            echo "  $0 --product siemcore --version 3.3.152.57 --file ./dist/siemcore-universal-3.3.152.57.tar.gz --groups alpha"
            echo ""
            echo "  # With release notes"
            echo "  $0 --product mysoc --version 1.5.0.3 --file ./mysoc-1.5.0.3.tar.gz --groups alpha --notes \"Bug fixes\""
            exit 0
            ;;
        *)
            echo -e "${RED}Unknown option: $1${NC}"
            exit 1
            ;;
    esac
done

# Get API key from env if not provided
if [ -z "$API_KEY" ]; then
    API_KEY="${UPDATES_API_KEY:-}"
fi

# Validate required arguments
if [ -z "$PRODUCT" ]; then
    echo -e "${RED}Error: --product is required${NC}"
    exit 1
fi

if [ -z "$VERSION" ]; then
    echo -e "${RED}Error: --version is required${NC}"
    exit 1
fi

if [ -z "$FILE" ]; then
    echo -e "${RED}Error: --file is required${NC}"
    exit 1
fi

if [ ! -f "$FILE" ]; then
    echo -e "${RED}Error: File not found: $FILE${NC}"
    exit 1
fi

if [ -z "$TARGET_GROUPS" ]; then
    echo -e "${RED}Error: --groups is required (start with alpha); an upload without target groups reaches every ring, including production${NC}"
    exit 1
fi

IFS=',' read -r -a GROUP_LIST <<< "$TARGET_GROUPS"
for g in "${GROUP_LIST[@]}"; do
    case "$g" in
        alpha|beta|stable|production) ;;
        *)
            echo -e "${RED}Error: invalid target group '${g}' (must be alpha, beta, stable, or production)${NC}"
            exit 1
            ;;
    esac
done

# Display upload info
echo -e "${CYAN}╔════════════════════════════════════════════╗${NC}"
echo -e "${CYAN}║        Upload Release to Updates Server     ║${NC}"
echo -e "${CYAN}╚════════════════════════════════════════════╝${NC}"
echo ""
echo -e "  ${GREEN}Product:${NC}  $PRODUCT"
echo -e "  ${GREEN}Version:${NC}  $VERSION"
echo -e "  ${GREEN}Channel:${NC}  $CHANNEL"
echo -e "  ${GREEN}Groups:${NC}   ${TARGET_GROUPS}"
echo -e "  ${GREEN}File:${NC}     $FILE ($(du -h "$FILE" | cut -f1))"
echo -e "  ${GREEN}Server:${NC}   $UPDATE_SERVER"
if [ -n "$NOTES" ]; then
    echo -e "  ${GREEN}Notes:${NC}    $NOTES"
fi
echo ""

# Build curl command
CURL_ARGS=(
    -X POST
    "${UPDATE_SERVER}/api/v1/releases"
    -F "product=${PRODUCT}"
    -F "version=${VERSION}"
    -F "channel=${CHANNEL}"
    -F "artifact=@${FILE}"
    -F "target_groups=${TARGET_GROUPS}"
)

if [ -n "$NOTES" ]; then
    CURL_ARGS+=(-F "release_notes=${NOTES}")
fi

if [ -n "$API_KEY" ]; then
    CURL_ARGS+=(-H "X-API-Key: ${API_KEY}")
fi

# Confirm upload
echo -e "${YELLOW}Ready to upload. Continue? [y/N]${NC} "
read -r CONFIRM
if [[ ! "$CONFIRM" =~ ^[Yy]$ ]]; then
    echo "Aborted."
    exit 0
fi

echo ""
echo -e "${YELLOW}Uploading...${NC}"

# Execute upload
RESPONSE=$(curl -s -w "\n%{http_code}" "${CURL_ARGS[@]}")
HTTP_CODE=$(echo "$RESPONSE" | tail -n1)
BODY=$(echo "$RESPONSE" | sed '$d')

if [ "$HTTP_CODE" -eq 201 ] || [ "$HTTP_CODE" -eq 200 ]; then
    echo ""
    echo -e "${GREEN}✓ Release uploaded successfully!${NC}"
    echo ""
    echo "Response:"
    echo "$BODY" | python3 -m json.tool 2>/dev/null || echo "$BODY"
    echo ""
    
    # Extract release ID if available
    RELEASE_ID=$(echo "$BODY" | python3 -c "import sys, json; print(json.load(sys.stdin).get('id', ''))" 2>/dev/null || echo "")
    
    if [ -n "$RELEASE_ID" ]; then
        echo -e "${CYAN}Next steps (each promotion needs approval and a soak):${NC}"
        echo ""
        echo "  # Promote to beta:"
        echo "  curl -X PUT ${UPDATE_SERVER}/api/v1/releases/${PRODUCT}/${VERSION}/target-groups \\"
        echo "    -H \"X-API-Key: \$UPDATES_API_KEY\" -H 'Content-Type: application/json' \\"
        echo "    -d '{\"target_groups\": [\"alpha\", \"beta\"]}'"
        echo ""
        echo "  # Then stable, then production (explicit consent): add each group in turn."
        echo "  # Withdraw: -d '{\"target_groups\": []}'"
    fi
else
    echo ""
    echo -e "${RED}✗ Upload failed (HTTP $HTTP_CODE)${NC}"
    echo ""
    echo "Response:"
    echo "$BODY"
    exit 1
fi
