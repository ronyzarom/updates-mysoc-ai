#!/usr/bin/env bash
# Build the production updater kits from the tracked templates in kits/,
# stamping the binary (ldflags), config, README, and bundled docs from the
# repo's single VERSION anchor so nothing can drift.
#
# Usage: scripts/build-kits.sh [mysoc|siemcore]...   (default: both)
#
# Output: dist/updater-kits/<kit>-<VERSION>/
#
# Rules enforced here (see .cursor/rules/development-cycle.mdc):
# - a kit for an existing version is never overwritten — bump VERSION first;
# - a build from an uncommitted tree is stamped "-dirty" and is diagnostic
#   only: it must never be shipped or tailored for a customer.
set -euo pipefail

cd "$(dirname "$0")/.."

VERSION=$(tr -d '[:space:]' < VERSION)
COMMIT=$(git rev-parse --short HEAD)
BUILD_TIME=$(date -u +%Y-%m-%dT%H:%M:%SZ)

if [[ -n "$(git status --porcelain)" ]]; then
    VERSION="${VERSION}-dirty"
    echo "WARNING: working tree is dirty — building DIAGNOSTIC kits (${VERSION}); do not ship." >&2
fi

if command -v sha256sum >/dev/null 2>&1; then
    SHA() { sha256sum "$@"; }
else
    SHA() { shasum -a 256 "$@"; }
fi

TIERS=("$@")
[[ ${#TIERS[@]} -eq 0 ]] && TIERS=(mysoc siemcore)

for tier in "${TIERS[@]}"; do
    case "$tier" in
        mysoc)    BIN_NAME=mysoc-updater;            KIT_NAME=mysoc-updater-kit ;;
        siemcore) BIN_NAME=siemcore-cascade-updater; KIT_NAME=siemcore-updater-kit ;;
        *) echo "unknown tier: $tier (expected mysoc or siemcore)" >&2; exit 1 ;;
    esac

    OUT="dist/updater-kits/${KIT_NAME}-${VERSION}"
    if [[ -e "$OUT" ]]; then
        echo "ERROR: $OUT already exists — never rebuild under an existing version; bump VERSION." >&2
        exit 1
    fi

    echo "==> $tier: building binaries (${VERSION}, ${COMMIT})"
    mkdir -p "$OUT/bin" "$OUT/docs"
    for arch in amd64 arm64; do
        GOOS=linux GOARCH="$arch" CGO_ENABLED=0 go build \
            -ldflags "-s -w -X main.Version=${VERSION} -X main.GitCommit=${COMMIT} -X main.BuildTime=${BUILD_TIME}" \
            -o "$OUT/bin/${BIN_NAME}-linux-${arch}" ./cmd/updater-simulator
    done

    echo "==> $tier: rendering templates"
    for f in kits/"$tier"/*; do
        base=$(basename "$f")
        case "$base" in
            config.yaml|README.md)
                sed "s/@VERSION@/${VERSION}/g" "$f" > "$OUT/$base" ;;
            *)
                cp "$f" "$OUT/$base" ;;
        esac
    done
    chmod +x "$OUT/install.sh"

    echo "==> $tier: bundling docs (stamped)"
    for doc in RELAY-DEPLOYMENT.md UPDATER-GUIDELINES.md UPDATE-ENTRYPOINT-CONTRACT.md; do
        {
            echo "<!-- bundled with ${KIT_NAME} ${VERSION} (commit ${COMMIT}) -->"
            cat "docs/$doc"
        } > "$OUT/docs/$doc"
    done

    echo "==> $tier: checksums"
    (cd "$OUT" && SHA bin/* config.yaml install.sh ./*.service > SHA256SUMS)

    echo "    built $OUT"
done

# Self-update release artifacts: the same binary under the per-platform
# product names the fleet watches ("updater-<os>-<arch>"). Upload each file as
# a release for its product on the updates server and every node running the
# self-updatable kit layout upgrades itself on the next heartbeat cycle.
ART="dist/updater-kits/updater-artifacts-${VERSION}"
if [[ -e "$ART" ]]; then
    echo "ERROR: $ART already exists — never rebuild under an existing version; bump VERSION." >&2
    exit 1
fi
echo "==> self-update artifacts"
mkdir -p "$ART"
for arch in amd64 arm64; do
    GOOS=linux GOARCH="$arch" CGO_ENABLED=0 go build \
        -ldflags "-s -w -X main.Version=${VERSION} -X main.GitCommit=${COMMIT} -X main.BuildTime=${BUILD_TIME}" \
        -o "$ART/updater-linux-${arch}" ./cmd/updater-simulator
done
(cd "$ART" && SHA updater-linux-* > SHA256SUMS)
echo "    built $ART (upload as products updater-linux-amd64 / updater-linux-arm64, version ${VERSION})"

echo
echo "DONE — kits ${VERSION} (commit ${COMMIT})"
