#!/usr/bin/env bash
# build-product-deploy-bundle.sh — 打包 deploy/product 生产部署所需的全部配置文件与静态资源。
#
# 在构建服务器（有完整仓库源码）上执行；产物手动上传到生产机后解压，再
# docker compose 启动。应用镜像（ongrid / ongrid-web）仍由构建机 build +
# tag + push，本脚本打包 compose 挂载所需的全部宿主机侧文件。
#
# 打包范围（缺一不可，缺则脚本失败退出）：
#   - nginx / frontier / prometheus(+告警规则) / loki / tempo / grafana 看板
#   - searxng、edge 安装脚本与二进制、edge 升级 bundle
#   - 离线嵌入模型（知识库 RAG）
#   - 内置知识库 builtin_vault（全部 .md）
#   - 内置 skills/、agents/ 助手模板
#
# Usage（仓库根目录）:
#   ./scripts/build-product-deploy-bundle.sh
#   ./scripts/build-product-deploy-bundle.sh --arch arm64
#
# 构建机必须先准备：
#   make fetch-promtail fetch-otelcol fetch-node-exporter fetch-process-exporter
#   make build-edge-linux-amd64 build-edge-linux-arm64
#   make fetch-embedding-model
#
# 生产机解压示例（ONGRID_DEPLOY_ROOT=/opt/ongrid/deploy）:
#   sudo mkdir -p /opt/ongrid
#   sudo tar -xzvf ongrid-product-deploy-<ver>-linux-amd64.tar.gz -C /opt/ongrid
#   cd /opt/ongrid/deploy/product
#   cp .env.example .env
#   ./bootstrap-data-dirs.sh
#   docker compose -f docker-compose-deploy.yml --env-file .env up -d
#
set -euo pipefail

SCRIPT_DIR=$(cd -- "$(dirname "${BASH_SOURCE[0]}")" && pwd)
REPO_ROOT=$(cd -- "$SCRIPT_DIR/.." && pwd)

VERSION=""
TARGET_ARCH="${TARGET_ARCH:-amd64}"
SKIP_EDGE=0
SKIP_EMBEDDING=0
OUT_DIR="${OUT_DIR:-$REPO_ROOT/dist/out/product-deploy}"

usage() {
  cat <<'EOF'
Usage: scripts/build-product-deploy-bundle.sh [OPTIONS]

Options:
  --version VER       版本号（默认读仓库 VERSION 文件）
  --arch amd64|arm64  主打 edge 架构（默认 amd64）
  --skip-edge         跳过 edge 二进制（仅调试用，生产勿用）
  --skip-embedding    跳过嵌入模型（仅调试用，生产勿用）
  --out DIR           输出目录（默认 dist/out/product-deploy）
  -h, --help          显示帮助
EOF
}

while [[ $# -gt 0 ]]; do
  case "$1" in
    --version) shift; VERSION="${1:-}" ;;
    --version=*) VERSION="${1#*=}" ;;
    --arch) shift; TARGET_ARCH="${1:-amd64}" ;;
    --arch=*) TARGET_ARCH="${1#*=}" ;;
    --skip-edge) SKIP_EDGE=1 ;;
    --skip-embedding) SKIP_EMBEDDING=1 ;;
    --out) shift; OUT_DIR="${1:-}" ;;
    --out=*) OUT_DIR="${1#*=}" ;;
    -h|--help) usage; exit 0 ;;
    *) echo "unknown option: $1" >&2; usage; exit 2 ;;
  esac
  shift
done

case "$TARGET_ARCH" in
  amd64|linux-amd64) TARGET_ARCH=amd64; PACKAGE_TARGET=linux-amd64 ;;
  arm64|linux-arm64) TARGET_ARCH=arm64; PACKAGE_TARGET=linux-arm64 ;;
  *) echo "unsupported --arch: $TARGET_ARCH" >&2; exit 2 ;;
esac

if [[ -z "$VERSION" ]]; then
  VERSION=$(tr -d '[:space:]' < "$REPO_ROOT/VERSION" 2>/dev/null || true)
fi
if [[ -z "$VERSION" ]]; then
  VERSION=$(git -C "$REPO_ROOT" describe --tags --always --dirty 2>/dev/null || echo v0.0.0-dev)
fi

PKG_NAME="ongrid-product-deploy-${VERSION}-${PACKAGE_TARGET}"
STAGE_PARENT="$REPO_ROOT/dist/stage"
STAGE_DIR="$STAGE_PARENT/$PKG_NAME"
DEPLOY_ROOT="$STAGE_DIR/deploy"
PRODUCT_DIR="$DEPLOY_ROOT/product"
TARBALL="$OUT_DIR/${PKG_NAME}.tar.gz"
SHAFILE="${TARBALL}.sha256"
PRIMARY_ARCH="linux-${TARGET_ARCH}"

log()  { printf '[product-bundle] %s\n' "$*"; }
warn() { printf '[product-bundle] warn: %s\n' "$*" >&2; }
die()  { printf '[product-bundle] error: %s\n' "$*" >&2; exit 1; }

must_file() {
  local src="$1" dst="$2" mode="${3:-}"
  [[ -f "$src" ]] || die "required file missing: $src"
  mkdir -p "$(dirname "$dst")"
  cp -f "$src" "$dst"
  [[ -n "$mode" ]] && chmod "$mode" "$dst"
  log "  + ${dst#$STAGE_DIR/}"
}

must_dir() {
  local src="$1" dst="$2"
  [[ -d "$src" ]] || die "required directory missing: $src"
  mkdir -p "$dst"
  cp -rf "$src/." "$dst/"
  log "  + ${dst#$STAGE_DIR/}/"
}

count_md() {
  find "$1" -type f -name '*.md' 2>/dev/null | wc -l | tr -d ' '
}

log "packaging ${PKG_NAME}"
rm -rf "$STAGE_DIR"
mkdir -p "$PRODUCT_DIR" "$PRODUCT_DIR/edge" "$OUT_DIR"

printf '%s\n' "$VERSION" > "$STAGE_DIR/VERSION"

# --- product：compose + env 模板 + 引导脚本 --------------------------------
must_file "$REPO_ROOT/deploy/product/docker-compose-deploy.yml" \
  "$PRODUCT_DIR/docker-compose-deploy.yml"
must_file "$REPO_ROOT/deploy/product/.env.example" \
  "$PRODUCT_DIR/.env.example"
must_file "$REPO_ROOT/deploy/product/bootstrap-data-dirs.sh" \
  "$PRODUCT_DIR/bootstrap-data-dirs.sh" 755

# --- nginx -------------------------------------------------------------------
must_file "$REPO_ROOT/deploy/nginx/nginx.conf" \
  "$DEPLOY_ROOT/nginx/nginx.conf"

# --- install 栈配置 ----------------------------------------------------------
must_file "$REPO_ROOT/deploy/install/frontier.yaml" \
  "$DEPLOY_ROOT/install/frontier.yaml"
must_file "$REPO_ROOT/deploy/install/prometheus.yml" \
  "$DEPLOY_ROOT/install/prometheus.yml"
must_file "$REPO_ROOT/deploy/install/prometheus-rules.yml" \
  "$DEPLOY_ROOT/install/prometheus-rules.yml"
must_file "$REPO_ROOT/deploy/install/loki-config.yaml" \
  "$DEPLOY_ROOT/install/loki-config.yaml"
must_file "$REPO_ROOT/deploy/install/tempo-config.yaml" \
  "$DEPLOY_ROOT/install/tempo-config.yaml"
must_dir "$REPO_ROOT/deploy/install/grafana" \
  "$DEPLOY_ROOT/install/grafana"
if [[ -d "$REPO_ROOT/deploy/install/prometheus" ]]; then
  must_dir "$REPO_ROOT/deploy/install/prometheus" \
    "$DEPLOY_ROOT/install/prometheus"
fi

# --- searxng -----------------------------------------------------------------
if [[ -d "$REPO_ROOT/deploy/install/searxng" ]]; then
  must_dir "$REPO_ROOT/deploy/install/searxng" "$DEPLOY_ROOT/searxng"
else
  must_dir "$REPO_ROOT/deploy/searxng" "$DEPLOY_ROOT/searxng"
fi

# --- 内置 skills / agents（助手模板，挂载覆盖镜像内 /skills /agents）-------
must_dir "$REPO_ROOT/skills" "$PRODUCT_DIR/runtime/skills"
must_dir "$REPO_ROOT/agents" "$PRODUCT_DIR/runtime/agents"

# --- 内置知识库 builtin_vault（离线同步 / 代码浏览基线）--------------------
VAULT_SRC="$REPO_ROOT/internal/manager/biz/knowledge/builtin_vault"
must_dir "$VAULT_SRC" "$PRODUCT_DIR/knowledge/builtin_vault"
VAULT_MD_COUNT=$(count_md "$PRODUCT_DIR/knowledge/builtin_vault")
[[ "$VAULT_MD_COUNT" -ge 10 ]] || die "builtin_vault too few markdown files ($VAULT_MD_COUNT) — knowledge base incomplete"

# --- edge：脚本 + 二进制 + 升级 bundle ---------------------------------------
EDGE_DIR="$PRODUCT_DIR/edge"
for script in install.sh uninstall.sh install-edge.sh build-edge-bundle.sh \
              ongrid-edge.env.example ongrid-edge.service; do
  must_file "$REPO_ROOT/deploy/install/edge/$script" \
    "$EDGE_DIR/$script" 755
done
must_file "$REPO_ROOT/deploy/install/apply-pending-upgrade.sh" \
  "$EDGE_DIR/apply-pending-upgrade.sh" 755
if [[ -f "$REPO_ROOT/deploy/edge/bash-policy.example.yaml" ]]; then
  must_file "$REPO_ROOT/deploy/edge/bash-policy.example.yaml" \
    "$EDGE_DIR/bash-policy.example.yaml"
fi

if [[ "$SKIP_EDGE" -eq 0 ]]; then
  for target in linux-amd64 linux-arm64 darwin-amd64 darwin-arm64; do
    for bin in ongrid-edge promtail otelcol-contrib node_exporter process_exporter; do
      src="$REPO_ROOT/bin/${target}/${bin}"
      dst="$EDGE_DIR/${bin}-${target}"
      if [[ -f "$src" ]]; then
        cp -f "$src" "$dst"
        chmod 755 "$dst"
        log "  + edge/${bin}-${target}"
      fi
    done
  done
  for bin in ongrid-edge promtail otelcol-contrib node_exporter process_exporter; do
    f="$EDGE_DIR/${bin}-${PRIMARY_ARCH}"
    [[ -f "$f" ]] || die "required edge binary missing: $f (run make build-edge-linux-${TARGET_ARCH} etc.)"
  done
  # ADR-024 一键升级 bundle
  if [[ -x "$EDGE_DIR/build-edge-bundle.sh" ]]; then
    for arch in linux-amd64 linux-arm64; do
      if [[ -f "$EDGE_DIR/ongrid-edge-${arch}" ]]; then
        "$EDGE_DIR/build-edge-bundle.sh" "$EDGE_DIR" "$VERSION" "$arch" \
          || die "build-edge-bundle failed for ${arch}"
        log "  + edge/edge-bundle-${arch}-${VERSION}.tar.gz (generated)"
      fi
    done
  fi
else
  warn "--skip-edge: edge binaries not packaged (not for production)"
fi

# --- 离线嵌入模型 ------------------------------------------------------------
if [[ "$SKIP_EMBEDDING" -eq 0 ]]; then
  EMB_SRC="$REPO_ROOT/.cache/embedding-models/fast-bge-small-zh-v1.5"
  EMB_DST="$PRODUCT_DIR/embeddings/fast-bge-small-zh-v1.5"
  [[ -f "$EMB_SRC/model_optimized.onnx" ]] || die "embedding model missing at $EMB_SRC — run: make fetch-embedding-model"
  mkdir -p "$EMB_DST"
  cp -rf "$EMB_SRC/." "$EMB_DST/"
  log "  + product/embeddings/fast-bge-small-zh-v1.5/"
else
  warn "--skip-embedding: RAG local embedder will not work offline"
fi

# --- 完整性校验 --------------------------------------------------------------
REQUIRED=(
  "$PRODUCT_DIR/docker-compose-deploy.yml"
  "$PRODUCT_DIR/.env.example"
  "$DEPLOY_ROOT/nginx/nginx.conf"
  "$DEPLOY_ROOT/install/frontier.yaml"
  "$DEPLOY_ROOT/install/prometheus.yml"
  "$DEPLOY_ROOT/install/prometheus-rules.yml"
  "$DEPLOY_ROOT/install/loki-config.yaml"
  "$DEPLOY_ROOT/install/tempo-config.yaml"
  "$DEPLOY_ROOT/install/grafana/provisioning/dashboards/default.yml"
  "$DEPLOY_ROOT/install/grafana/provisioning/dashboards/json/cluster-overview.json"
  "$DEPLOY_ROOT/install/grafana/provisioning/dashboards/json/server-detail.json"
  "$DEPLOY_ROOT/install/grafana/provisioning/dashboards/json/manager-internals.json"
  "$DEPLOY_ROOT/install/grafana/provisioning/datasources/prometheus.yml"
  "$DEPLOY_ROOT/install/grafana/provisioning/datasources/loki.yml"
  "$DEPLOY_ROOT/install/grafana/provisioning/datasources/tempo.yml"
  "$DEPLOY_ROOT/searxng/settings.yml"
  "$EDGE_DIR/install.sh"
  "$PRODUCT_DIR/runtime/skills/bash/SKILL.md"
  "$PRODUCT_DIR/runtime/agents/specialist-sre.md"
  "$PRODUCT_DIR/knowledge/builtin_vault/alerts/host-metrics.md"
)
for f in "${REQUIRED[@]}"; do
  [[ -f "$f" ]] || die "integrity check failed — missing: $f"
done

SKILL_COUNT=$(find "$PRODUCT_DIR/runtime/skills" -name 'SKILL.md' | wc -l | tr -d ' ')
AGENT_COUNT=$(find "$PRODUCT_DIR/runtime/agents" -name '*.md' | wc -l | tr -d ' ')
[[ "$SKILL_COUNT" -ge 1 ]] || die "no SKILL.md under runtime/skills"
[[ "$AGENT_COUNT" -ge 1 ]] || die "no agent .md under runtime/agents"

log "integrity ok: vault_md=${VAULT_MD_COUNT} skills=${SKILL_COUNT} agents=${AGENT_COUNT}"

# --- 清单 -------------------------------------------------------------------
MANIFEST="$STAGE_DIR/MANIFEST.txt"
{
  echo "ongrid product deploy bundle"
  echo "version: $VERSION"
  echo "target:  $PACKAGE_TARGET"
  echo "built:   $(date -u +%Y-%m-%dT%H:%M:%SZ 2>/dev/null || date)"
  echo "vault_md: ${VAULT_MD_COUNT}  skills: ${SKILL_COUNT}  agents: ${AGENT_COUNT}"
  echo ""
  echo "extract:  sudo tar -xzvf ${PKG_NAME}.tar.gz -C /opt/ongrid"
  echo "deploy:   cd /opt/ongrid/deploy/product"
  echo "env:      cp .env.example .env"
  echo "data:     ./bootstrap-data-dirs.sh"
  echo "start:    docker compose -f docker-compose-deploy.yml --env-file .env up -d"
  echo ""
  find "$STAGE_DIR" -type f ! -path "$MANIFEST" | sort | sed "s|^$STAGE_DIR/||"
} > "$MANIFEST"
log "  + MANIFEST.txt"

# --- 打 tar.gz ---------------------------------------------------------------
log "creating $TARBALL"
if [[ "${OSTYPE:-}" == "darwin"* ]]; then
  xattr -rc "$STAGE_DIR" 2>/dev/null || true
  find "$STAGE_DIR" -name '.DS_Store' -delete 2>/dev/null || true
fi
COPYFILE_DISABLE=1 tar --no-xattrs -czf "$TARBALL" -C "$STAGE_PARENT" "$(basename "$STAGE_DIR")" 2>/dev/null \
  || tar -czf "$TARBALL" -C "$STAGE_PARENT" "$(basename "$STAGE_DIR")"

if command -v sha256sum >/dev/null 2>&1; then
  (cd "$OUT_DIR" && sha256sum "$(basename "$TARBALL")" > "$(basename "$SHAFILE")")
elif command -v shasum >/dev/null 2>&1; then
  (cd "$OUT_DIR" && shasum -a 256 "$(basename "$TARBALL")" > "$(basename "$SHAFILE")")
fi

SIZE=$(du -h "$TARBALL" 2>/dev/null | awk '{print $1}' || wc -c < "$TARBALL")
log "done: $TARBALL ($SIZE)"
[[ -f "$SHAFILE" ]] && log "sha256: $SHAFILE"
