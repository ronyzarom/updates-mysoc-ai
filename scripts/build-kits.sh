#!/usr/bin/env bash
# Build the production updater kits from the tracked templates in kits/,
# stamping the binary (ldflags), config, README, and bundled docs from the
# repo's single VERSION anchor so nothing can drift.
#
# Usage: scripts/build-kits.sh [mysoc|siemcore|widecore]...   (default: all three)
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
[[ ${#TIERS[@]} -eq 0 ]] && TIERS=(mysoc siemcore widecore)

for tier in "${TIERS[@]}"; do
    case "$tier" in
        mysoc)    BIN_NAME=mysoc-updater;            KIT_NAME=mysoc-updater-kit ;;
        siemcore) BIN_NAME=siemcore-cascade-updater; KIT_NAME=siemcore-updater-kit ;;
        widecore) BIN_NAME=widecore-updater;         KIT_NAME=widecore-updater-kit ;;
        *) echo "unknown tier: $tier (expected mysoc, siemcore, or widecore)" >&2; exit 1 ;;
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
    # The root-owned self-update restore backstop (survivability invariant 4).
    [[ -f "$OUT/${BIN_NAME}-self-restore.sh" ]] && chmod +x "$OUT/${BIN_NAME}-self-restore.sh"

    # Each kit bundles its root apply shim so `install.sh --executor filesystem`
    # can install it on a fresh host with no extra files staged.
    if [[ "$tier" == mysoc ]]; then
        echo "==> $tier: bundling apply wrapper (mysoc-apply-update.sh)"
        cp deploy/mysoc-apply-update.sh "$OUT/mysoc-apply-update.sh"
        chmod +x "$OUT/mysoc-apply-update.sh"
    elif [[ "$tier" == siemcore ]]; then
        # The four siemcore host files (apply shim, sudoers entry, override.conf
        # unit drop-in, executor-block.yaml) are SiemCore-owned; we bundle them
        # with a pinned siemcore-host-files.sha256 and verify the bytes match the
        # pin before shipping, so a drifted or tampered file fails the build (and
        # later the install).
        echo "==> $tier: bundling SiemCore host files + verifying pins"
        for hf in siemcore-apply-update.sh siemcore-cascade-updater.sudoers override.conf executor-block.yaml siemcore-host-files.sha256; do
            cp "deploy/siemcore-cascade/$hf" "$OUT/$hf"
        done
        chmod +x "$OUT/siemcore-apply-update.sh"
        for hf in siemcore-apply-update.sh siemcore-cascade-updater.sudoers override.conf executor-block.yaml; do
            want="$(awk -v b="$hf" '$2==b || $2=="*"b {print $1; exit}' "$OUT/siemcore-host-files.sha256")"
            have="$(SHA "$OUT/$hf" | awk '{print $1}')"
            if [[ -z "$want" || "$want" != "$have" ]]; then
                echo "ERROR: siemcore host file $hf does not match its pin (pin ${want:-none}, got $have)." >&2
                echo "       Regenerate deploy/siemcore-cascade/siemcore-host-files.sha256 after a deliberate change." >&2
                exit 1
            fi
        done
    elif [[ "$tier" == widecore ]]; then
        # The four widecore host files (apply shim, sudoers entry, unit drop-in,
        # executor block) are WideCore-owned; verify pinned SHA-256s before shipping.
        echo "==> $tier: bundling WideCore host files + verifying pins"
        for hf in widecore-apply-update sudoers-widecore-updater updater-override.conf updater-executor.yaml widecore-host-files.sha256; do
            cp "deploy/widecore-cascade/$hf" "$OUT/$hf"
        done
        chmod +x "$OUT/widecore-apply-update"
        for hf in widecore-apply-update sudoers-widecore-updater updater-override.conf updater-executor.yaml; do
            want="$(awk -v b="$hf" '$2==b || $2=="*"b {print $1; exit}' "$OUT/widecore-host-files.sha256")"
            have="$(SHA "$OUT/$hf" | awk '{print $1}')"
            if [[ -z "$want" || "$want" != "$have" ]]; then
                echo "ERROR: widecore host file $hf does not match its pin (pin ${want:-none}, got $have)." >&2
                echo "       Regenerate deploy/widecore-cascade/widecore-host-files.sha256 after a deliberate change." >&2
                exit 1
            fi
        done
    fi

    echo "==> $tier: bundling docs (stamped)"
    for doc in RELAY-DEPLOYMENT.md UPDATER-GUIDELINES.md UPDATE-ENTRYPOINT-CONTRACT.md; do
        {
            echo "<!-- bundled with ${KIT_NAME} ${VERSION} (commit ${COMMIT}) -->"
            cat "docs/$doc"
        } > "$OUT/docs/$doc"
    done

    echo "==> $tier: checksums"
    (
        cd "$OUT"
        files=(bin/* config.yaml install.sh ./*.service)
        [[ -f mysoc-apply-update.sh ]] && files+=(mysoc-apply-update.sh)
        [[ -f siemcore-apply-update.sh ]] && files+=(siemcore-apply-update.sh siemcore-cascade-updater.sudoers override.conf executor-block.yaml siemcore-host-files.sha256)
        [[ -f widecore-apply-update ]] && files+=(widecore-apply-update sudoers-widecore-updater updater-override.conf updater-executor.yaml widecore-host-files.sha256)
        for restore in ./*-self-restore.sh; do
            [[ -f "$restore" ]] && files+=("$restore")
        done
        SHA "${files[@]}" > SHA256SUMS
    )

    # A self-describing tarball (plus its own checksum) so a fresh host can
    # fetch and verify the whole kit over plain HTTPS, with no license header.
    # The publish step copies/points <kit>-latest.tar.gz at the newest build.
    echo "==> $tier: tarball"
    base="$(basename "$OUT")"
    tar -C "dist/updater-kits" -czf "$OUT.tar.gz" "$base"
    (cd "dist/updater-kits" && SHA "$base.tar.gz" > "$base.tar.gz.sha256")

    echo "    built $OUT (+ $base.tar.gz)"
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
