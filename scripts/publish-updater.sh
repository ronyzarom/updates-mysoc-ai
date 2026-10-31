#!/bin/bash
# Publish the updater self-update artifacts (updater-linux-amd64 / -arm64) to a
# single ring on updates.mysoc.ai.
#
# The admin API key is NEVER read, printed, or stored on this workstation. It
# lives only in the server's config/.env; this script uploads by copying the
# artifacts to the server over SSH and running curl inside the SSH session,
# where the key is sourced from that env file and never leaves the host.
#
# Usage:
#   scripts/publish-updater.sh --version 1.18.0.1 --groups alpha --yes
#
# Target groups are MANDATORY: an upload without them reaches every ring,
# including production. Start at alpha and widen later, per ring, with a soak
# in between (see docs/UPDATER-RELEASE.md).
set -euo pipefail

# --- server coordinates (match scripts/deploy.sh) ---------------------------
SERVER_USER="${UPDATES_SERVER_USER:-bitnami}"
SERVER_IP="${UPDATES_SERVER_IP:-18.201.144.150}"
SSH_KEY="${UPDATES_SSH_KEY:-/Users/ronyzaromil/Downloads/LightsailDefaultKey-eu-west-updates-mysoc.ai.pem}"
REMOTE_DIR="${UPDATES_REMOTE_DIR:-/home/bitnami/updates-mysoc-ai}"
LOCAL_API="${UPDATES_LOCAL_API:-http://127.0.0.1:8080}"
PUBLIC_API="${UPDATES_PUBLIC_API:-https://updates.mysoc.ai}"
CHANNEL="stable"

RED='\033[0;31m'; GREEN='\033[0;32m'; YELLOW='\033[1;33m'; CYAN='\033[0;36m'; NC='\033[0m'
die() { echo -e "${RED}Error: $1${NC}" >&2; exit 1; }

PROJECT_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
VERSION=""
TARGET_GROUPS=""
ARTIFACTS_DIR=""
ASSUME_YES=0

while [[ $# -gt 0 ]]; do
    case "$1" in
        --version|-v) VERSION="$2"; shift 2;;
        --groups|-g)  TARGET_GROUPS="$2"; shift 2;;
        --artifacts-dir) ARTIFACTS_DIR="$2"; shift 2;;
        --yes|-y)     ASSUME_YES=1; shift;;
        --help|-h)
            sed -n '2,15p' "$0" | sed 's/^# \{0,1\}//'
            exit 0;;
        *) die "unknown option: $1";;
    esac
done

[[ -z "$VERSION" ]] && VERSION="$(tr -d '[:space:]' < "$PROJECT_ROOT/VERSION")"
[[ -z "$VERSION" ]] && die "--version is required (or a VERSION file)"
[[ -z "$TARGET_GROUPS" ]] && die "--groups is required (start with alpha); an upload without target groups reaches every ring, including production"

IFS=',' read -r -a GROUP_LIST <<< "$TARGET_GROUPS"
for g in "${GROUP_LIST[@]}"; do
    case "$g" in
        alpha|beta|stable|production) ;;
        *) die "invalid target group '${g}' (must be alpha, beta, stable, or production)";;
    esac
done

[[ -z "$ARTIFACTS_DIR" ]] && ARTIFACTS_DIR="$PROJECT_ROOT/dist/updater-kits/updater-artifacts-${VERSION}"
[[ -d "$ARTIFACTS_DIR" ]] || die "artifacts dir not found: $ARTIFACTS_DIR (run scripts/build-kits.sh first)"

ARCHES=(amd64 arm64)
for arch in "${ARCHES[@]}"; do
    [[ -f "$ARTIFACTS_DIR/updater-linux-${arch}" ]] || die "missing artifact: $ARTIFACTS_DIR/updater-linux-${arch}"
done
[[ -f "$ARTIFACTS_DIR/SHA256SUMS" ]] || die "missing $ARTIFACTS_DIR/SHA256SUMS"
[[ -f "$SSH_KEY" ]] || die "SSH key not found: $SSH_KEY"

ssh_cmd() { ssh -i "$SSH_KEY" -o StrictHostKeyChecking=no "$SERVER_USER@$SERVER_IP" "$@"; }
scp_cmd() { scp -i "$SSH_KEY" -o StrictHostKeyChecking=no "$@"; }

echo -e "${CYAN}Publish updater self-update artifacts${NC}"
echo -e "  ${GREEN}Version:${NC} $VERSION"
echo -e "  ${GREEN}Groups:${NC}  $TARGET_GROUPS"
echo -e "  ${GREEN}Server:${NC}  $SERVER_USER@$SERVER_IP (upload via local API $LOCAL_API)"
echo -e "  ${GREEN}Dir:${NC}     $ARTIFACTS_DIR"
echo "  Local SHA256SUMS:"
sed 's/^/    /' "$ARTIFACTS_DIR/SHA256SUMS"
echo

if [[ "$ASSUME_YES" -ne 1 ]]; then
    echo -e "${YELLOW}Upload both arches to [${TARGET_GROUPS}]? [y/N]${NC} "
    read -r CONFIRM
    [[ "$CONFIRM" =~ ^[Yy]$ ]] || { echo "Aborted."; exit 0; }
fi

REMOTE_TMP="/tmp/publish-updater-${VERSION}"
ssh_cmd "mkdir -p '$REMOTE_TMP'"
for arch in "${ARCHES[@]}"; do
    scp_cmd "$ARTIFACTS_DIR/updater-linux-${arch}" "$SERVER_USER@$SERVER_IP:$REMOTE_TMP/updater-linux-${arch}"
done

for arch in "${ARCHES[@]}"; do
    product="updater-linux-${arch}"
    echo -e "${YELLOW}Uploading ${product} ${VERSION}...${NC}"
    # The key is sourced on the server and used in the same shell; it is never
    # echoed and never crosses the SSH boundary to this workstation.
    code=$(ssh_cmd "
        set -euo pipefail
        set -a; . '$REMOTE_DIR/config/.env'; set +a
        : \"\${ADMIN_API_KEY:?ADMIN_API_KEY missing from $REMOTE_DIR/config/.env}\"
        curl -s -o /tmp/publish-resp.json -w '%{http_code}' \
            -X POST '$LOCAL_API/api/v1/releases' \
            -H \"X-API-Key: \$ADMIN_API_KEY\" \
            -F 'product=${product}' \
            -F 'version=${VERSION}' \
            -F 'channel=${CHANNEL}' \
            -F 'target_groups=${TARGET_GROUPS}' \
            -F 'artifact=@${REMOTE_TMP}/${product}'
    ")
    if [[ "$code" != "201" && "$code" != "200" ]]; then
        echo -e "${RED}✗ ${product}: HTTP ${code}${NC}" >&2
        ssh_cmd "cat /tmp/publish-resp.json" >&2 || true
        die "upload failed for ${product}"
    fi
    echo -e "${GREEN}✓ ${product} uploaded (HTTP ${code})${NC}"
done

ssh_cmd "rm -rf '$REMOTE_TMP' /tmp/publish-resp.json" || true

echo
echo -e "${CYAN}Verifying from the public API (no credentials)...${NC}"
VERIFY=$(curl -s "$PUBLIC_API/api/v1/releases" | python3 - "$VERSION" <<'PY'
import sys, json
want = sys.argv[1]
try:
    data = json.load(sys.stdin)
except Exception as e:
    print("PARSE_ERROR", e); sys.exit(0)
rows = []
for r in data:
    name = r.get("product_name", "")
    if name.startswith("updater-linux") and r.get("version") == want:
        arts = (r.get("manifest") or {}).get("artifacts") or [{}]
        rows.append((name, r.get("version"), r.get("channel"),
                     ",".join(r.get("target_groups") or []), arts[0].get("checksum", "")))
for row in sorted(rows):
    print("  %-22s %-10s %-8s groups=%-20s sha256=%s" % row)
print("COUNT", len(rows))
PY
)
echo "$VERIFY"
echo
echo -e "${CYAN}Compare the sha256 above against local SHA256SUMS. Next: merge+tag, hub-notify, soak before widening (docs/UPDATER-RELEASE.md).${NC}"
