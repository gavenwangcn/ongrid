#!/usr/bin/env bash
# build-edge-offline-package.sh — build edge-agent offline install tarballs.
#
# Does NOT invoke make. Produces everything needed to manually upload and
# install ongrid-edge on hosts that cannot reach the manager's /install.sh
# or the public internet.
#
# Requirements on the build host: go, curl, unzip, tar, sha256sum.
# Network: GitHub / Grafana release URLs (build host only; edge hosts stay offline).
#
# Usage:
#   ./scripts/build-edge-offline-package.sh              # linux-amd64 + arm64
#   ./scripts/build-edge-offline-package.sh --arch amd64 # one arch
#   ./scripts/build-edge-offline-package.sh --arch arm64
#
# Output (under dist/out/edge-offline/):
#   ongrid-edge-offline-<version>-linux-amd64.tar.gz
#   ongrid-edge-offline-<version>-linux-arm64.tar.gz   (unless --arch filters)
#   manager-staging/linux-amd64/  — loose files for nginx /edge/ on the manager
#
# Edge host install (after scp + tar xf):
#   sudo ONGRID_CLOUD_ADDR=<manager>:40012 \
#        EDGE_ACCESS_KEY=<key> EDGE_SECRET_KEY=<secret> \
#        ./install-edge.sh
#
set -euo pipefail

SCRIPT_DIR=$(cd -- "$(dirname "${BASH_SOURCE[0]}")" && pwd)
REPO_ROOT=$(cd -- "$SCRIPT_DIR/.." && pwd)

# --- versions (keep aligned with Makefile) -----------------------------------
PROMTAIL_VERSION=3.4.0
OTELCOL_VERSION=0.118.0
NODE_EXPORTER_VERSION=1.8.2
PROCESS_EXPORTER_VERSION=0.8.4

FETCH_CURL_FLAGS=(-fL --retry 3 --retry-all-errors --retry-delay 3 \
  --connect-timeout 15 --speed-time 60 --speed-limit 1024 --show-error)

VERSION=$(cat "$REPO_ROOT/VERSION" 2>/dev/null || true)
if [[ -z "$VERSION" ]]; then
  VERSION=$(git -C "$REPO_ROOT" describe --tags --always --dirty 2>/dev/null || echo v0.0.0-dev)
fi

OUT_DIR="$REPO_ROOT/dist/out/edge-offline"
STAGE_ROOT="$REPO_ROOT/dist/stage/edge-offline-build"
MANAGER_STAGE="$OUT_DIR/manager-staging"

ARCH_FILTER="" # empty = both linux-amd64 and linux-arm64

usage() {
  cat <<EOF
Usage: $(basename "$0") [OPTIONS]

Build offline edge-agent install packages (no make required).

Options:
  --arch amd64|arm64|all   Target architecture (default: all linux arches)
  --out DIR                Output directory (default: dist/out/edge-offline)
  -h, --help               Show this help

After build, upload ongrid-edge-offline-<ver>-linux-<arch>.tar.gz to the edge
host, extract, and run install-edge.sh with manager tunnel credentials.
EOF
}

while [[ $# -gt 0 ]]; do
  case "$1" in
    --arch)
      shift
      case "${1:-}" in
        amd64)  ARCH_FILTER=linux-amd64 ;;
        arm64)  ARCH_FILTER=linux-arm64 ;;
        all|"") ARCH_FILTER="" ;;
        *) echo "unknown --arch: $1" >&2; exit 2 ;;
      esac
      ;;
    --arch=*)
      case "${1#*=}" in
        amd64)  ARCH_FILTER=linux-amd64 ;;
        arm64)  ARCH_FILTER=linux-arm64 ;;
        all)    ARCH_FILTER="" ;;
        *) die "unknown --arch=${1#*=}" ;;
      esac
      ;;
    --out)
      shift
      OUT_DIR=${1:?--out requires directory}
      MANAGER_STAGE="$OUT_DIR/manager-staging"
      ;;
    -h|--help) usage; exit 0 ;;
    *) echo "unknown arg: $1" >&2; usage; exit 2 ;;
  esac
  shift
done

if [[ -n "$ARCH_FILTER" ]]; then
  TARGETS=("$ARCH_FILTER")
else
  TARGETS=(linux-amd64 linux-arm64)
fi

log()  { printf '[edge-offline] %s\n' "$*"; }
warn() { printf '[edge-offline] warn: %s\n' "$*" >&2; }
die()  { printf '[edge-offline] error: %s\n' "$*" >&2; exit 1; }

need_cmd() {
  command -v "$1" >/dev/null 2>&1 || die "missing required command: $1"
}

need_cmd go
need_cmd curl
need_cmd unzip
need_cmd tar
need_cmd sha256sum

log "version=$VERSION targets=${TARGETS[*]}"

rm -rf "$STAGE_ROOT"
mkdir -p "$OUT_DIR" "$STAGE_ROOT" "$MANAGER_STAGE"

# --- compile ongrid-edge -------------------------------------------------------
build_edge_binary() {
  local target=$1
  local os=${target%-*}
  local arch=${target##*-}
  local dest="$STAGE_ROOT/$target/ongrid-edge"
  mkdir -p "$STAGE_ROOT/$target"
  log "go build ongrid-edge $target"
  GOOS=$os GOARCH=$arch CGO_ENABLED=0 \
    go build -trimpath -ldflags "-s -w -X main.version=$VERSION" \
    -o "$dest" "$REPO_ROOT/cmd/ongrid-edge"
  chmod 755 "$dest"
}

# --- fetch plugin binary -------------------------------------------------------
fetch_promtail() {
  local target=$1
  local dest="$STAGE_ROOT/$target/promtail"
  [[ -f "$dest" ]] && return 0
  local os=${target%-*} arch=${target##*-}
  local zip=/tmp/promtail-$$-$target.zip
  local url="https://github.com/grafana/loki/releases/download/v${PROMTAIL_VERSION}/promtail-${os}-${arch}.zip"
  log "fetch promtail $target"
  curl "${FETCH_CURL_FLAGS[@]}" -o "$zip" "$url"
  unzip -p "$zip" > "$dest"
  chmod +x "$dest"
  rm -f "$zip"
}

fetch_otelcol() {
  local target=$1
  local dest="$STAGE_ROOT/$target/otelcol-contrib"
  [[ -f "$dest" ]] && return 0
  local os=${target%-*} arch=${target##*-}
  local tgz=/tmp/otelcol-$$-$target.tar.gz
  local url="https://github.com/open-telemetry/opentelemetry-collector-releases/releases/download/v${OTELCOL_VERSION}/otelcol-contrib_${OTELCOL_VERSION}_${os}_${arch}.tar.gz"
  log "fetch otelcol-contrib $target"
  mkdir -p "$STAGE_ROOT/$target"
  curl "${FETCH_CURL_FLAGS[@]}" -o "$tgz" "$url"
  tar -xzf "$tgz" -C "$STAGE_ROOT/$target" otelcol-contrib
  chmod +x "$dest"
  rm -f "$tgz"
}

fetch_node_exporter() {
  local target=$1
  local dest="$STAGE_ROOT/$target/node_exporter"
  [[ -f "$dest" ]] && return 0
  local os=${target%-*} arch=${target##*-}
  local folder="node_exporter-${NODE_EXPORTER_VERSION}.${os}-${arch}"
  local tgz=/tmp/node_exporter-$$-$target.tar.gz
  local url="https://github.com/prometheus/node_exporter/releases/download/v${NODE_EXPORTER_VERSION}/${folder}.tar.gz"
  log "fetch node_exporter $target"
  mkdir -p "$STAGE_ROOT/$target"
  curl "${FETCH_CURL_FLAGS[@]}" -o "$tgz" "$url"
  tar -xzf "$tgz" --strip-components=1 -C "$STAGE_ROOT/$target" "${folder}/node_exporter"
  chmod +x "$dest"
  rm -f "$tgz"
}

fetch_process_exporter() {
  local target=$1
  local dest="$STAGE_ROOT/$target/process_exporter"
  [[ -f "$dest" ]] && return 0
  local os=${target%-*} arch=${target##*-}
  local folder="process-exporter-${PROCESS_EXPORTER_VERSION}.${os}-${arch}"
  local tgz=/tmp/process_exporter-$$-$target.tar.gz
  local url="https://github.com/ncabatoff/process-exporter/releases/download/v${PROCESS_EXPORTER_VERSION}/${folder}.tar.gz"
  log "fetch process_exporter $target"
  mkdir -p "$STAGE_ROOT/$target"
  curl "${FETCH_CURL_FLAGS[@]}" -o "$tgz" "$url"
  tar -xzf "$tgz" --strip-components=1 -C "$STAGE_ROOT/$target" "${folder}/process-exporter"
  mv "$STAGE_ROOT/$target/process-exporter" "$dest"
  chmod +x "$dest"
  rm -f "$tgz"
}

fetch_plugins() {
  local target=$1
  fetch_promtail "$target"
  fetch_otelcol "$target"
  fetch_node_exporter "$target"
  fetch_process_exporter "$target"
}

# --- ADR-024 upgrade bundle ----------------------------------------------------
build_upgrade_bundle() {
  local target=$1
  local bin_dir="$REPO_ROOT/bin/$target"
  local src="$STAGE_ROOT/$target"
  mkdir -p "$bin_dir"
  for f in ongrid-edge promtail otelcol-contrib node_exporter process_exporter; do
    install -m 755 "$src/$f" "$bin_dir/$f"
  done
  bash "$REPO_ROOT/dist/build-edge-bundle.sh" "$VERSION" "$target" "$STAGE_ROOT/$target"
}

# --- assemble per-arch offline tarball -----------------------------------------
stage_offline_package() {
  local target=$1
  local os=${target%-*}
  local arch=${target##*-}
  local pkg_name="ongrid-edge-offline-${VERSION}-${target}"
  local pkg_dir="$STAGE_ROOT/$pkg_name"
  local src="$STAGE_ROOT/$target"

  rm -rf "$pkg_dir"
  mkdir -p "$pkg_dir"

  # install scripts + systemd unit
  install -m 755 "$REPO_ROOT/deploy/install/edge/install-edge.sh" "$pkg_dir/"
  install -m 755 "$REPO_ROOT/deploy/install/edge/install.sh" "$pkg_dir/"
  install -m 755 "$REPO_ROOT/deploy/install/edge/uninstall.sh" "$pkg_dir/"
  install -m 644 "$REPO_ROOT/deploy/install/edge/ongrid-edge.service" "$pkg_dir/"
  install -m 644 "$REPO_ROOT/deploy/install/edge/ongrid-edge.env.example" "$pkg_dir/"
  install -m 755 "$REPO_ROOT/deploy/install/apply-pending-upgrade.sh" "$pkg_dir/"
  install -m 755 "$REPO_ROOT/deploy/install/edge/build-edge-bundle.sh" "$pkg_dir/"

  # loose binaries (names match install-edge.sh expectations)
  install -m 755 "$src/ongrid-edge"           "$pkg_dir/ongrid-edge-${os}-${arch}"
  install -m 755 "$src/promtail"              "$pkg_dir/promtail-${os}-${arch}"
  install -m 755 "$src/otelcol-contrib"       "$pkg_dir/otelcol-contrib-${os}-${arch}"
  install -m 755 "$src/node_exporter"         "$pkg_dir/node_exporter-${os}-${arch}"
  install -m 755 "$src/process_exporter"      "$pkg_dir/process_exporter-${os}-${arch}"

  # remote upgrade bundle (manager can also host this under /edge/)
  cp "$src/edge-bundle-${arch}-${VERSION}.tar.gz" "$pkg_dir/"
  cp "$src/edge-bundle-${arch}-${VERSION}.tar.gz.sha256" "$pkg_dir/"

  echo "$VERSION" > "$pkg_dir/VERSION"

  cat > "$pkg_dir/README.txt" <<EOF
ongrid edge offline install package
===================================
Version: $VERSION
Arch:    $target

1) Upload this directory (or the .tar.gz) to the edge host.

2) Install (replace credentials from manager UI → Devices):
   tar xzf ongrid-edge-offline-${VERSION}-${target}.tar.gz
   cd ongrid-edge-offline-${VERSION}-${target}
   sudo ONGRID_CLOUD_ADDR=<manager-ip>:40012 \\
        EDGE_ACCESS_KEY=<access-key> \\
        EDGE_SECRET_KEY=<secret-key> \\
        ./install-edge.sh

3) Verify:
   journalctl -u ongrid-edge -f
   # expect: tunnel connected / registered with cloud

4) Uninstall:
   sudo ./install-edge.sh --uninstall

Remote upgrade (ADR-024):
  apply-pending-upgrade.sh is installed to /usr/local/lib/ongrid-edge/
  by install-edge.sh. The manager serves edge-bundle-*.tar.gz from /edge/;
  copy edge-bundle-${arch}-${VERSION}.tar.gz* to the manager's bin/ or
  deploy/edge/ if you use UI-driven whole-bundle upgrades.

Online install alternative (edge can reach manager HTTP):
   curl -sSL http://<manager>/install.sh | bash -s -- \\
     --access-key=... --secret-key=... \\
     --server-edge-addr=<manager>:40012 \\
     --server-http-addr=<manager>
EOF

  local tarball="$OUT_DIR/${pkg_name}.tar.gz"
  tar -C "$STAGE_ROOT" -czf "$tarball" "$pkg_name"
  sha256sum "$tarball" | awk '{print $1}' > "${tarball}.sha256"
  log "wrote $tarball ($(du -sh "$tarball" | awk '{print $1}'))"
  log "sha256: $(cat "${tarball}.sha256")"
}

# --- manager nginx /edge/ staging (optional convenience) -----------------------
stage_manager_flat() {
  local target=$1
  local os=${target%-*}
  local arch=${target##*-}
  local src="$STAGE_ROOT/$target"
  local dst="$MANAGER_STAGE/$target"
  mkdir -p "$dst"

  install -m 755 "$REPO_ROOT/deploy/install/edge/install.sh" "$MANAGER_STAGE/install.sh"
  install -m 755 "$REPO_ROOT/deploy/install/edge/uninstall.sh" "$MANAGER_STAGE/uninstall.sh"
  install -m 755 "$REPO_ROOT/deploy/install/apply-pending-upgrade.sh" "$MANAGER_STAGE/apply-pending-upgrade.sh"

  install -m 755 "$src/ongrid-edge"      "$dst/ongrid-edge-${os}-${arch}"
  install -m 755 "$src/promtail"         "$dst/promtail-${os}-${arch}"
  install -m 755 "$src/otelcol-contrib"  "$dst/otelcol-contrib-${os}-${arch}"
  install -m 755 "$src/node_exporter"    "$dst/node_exporter-${os}-${arch}"
  install -m 755 "$src/process_exporter" "$dst/process_exporter-${os}-${arch}"

  cp "$src/edge-bundle-${arch}-${VERSION}.tar.gz" "$dst/"
  cp "$src/edge-bundle-${arch}-${VERSION}.tar.gz.sha256" "$dst/"

  log "manager staging: $dst (copy flat files to ~/ongrid/bin/ for dev compose)"
}

# --- main ----------------------------------------------------------------------
for target in "${TARGETS[@]}"; do
  build_edge_binary "$target"
  fetch_plugins "$target"
  build_upgrade_bundle "$target"
  stage_offline_package "$target"
  stage_manager_flat "$target"
done

echo ""
log "=== done ==="
log "edge offline packages:"
ls -lh "$OUT_DIR"/ongrid-edge-offline-"${VERSION}"-*.tar.gz 2>/dev/null || true
echo ""
log "manager flat files (for nginx /edge/): $MANAGER_STAGE"
log "  cp -f $MANAGER_STAGE/install.sh ~/ongrid/bin/"
log "  cp -f $MANAGER_STAGE/linux-amd64/* ~/ongrid/bin/   # adjust arch"
