#!/usr/bin/env bash
# 生产机首次部署：创建数据目录、授权、同步包内知识库/嵌入模型到 ONGRID_DATA_DIR。
# 在 deploy/product/ 目录执行，读取同目录 .env。
set -euo pipefail

SCRIPT_DIR=$(cd -- "$(dirname "${BASH_SOURCE[0]}")" && pwd)
ENV_FILE="${ENV_FILE:-$SCRIPT_DIR/.env}"

if [[ ! -f "$ENV_FILE" ]]; then
  echo "missing $ENV_FILE — copy .env.example to .env first" >&2
  exit 1
fi

# shellcheck disable=SC1090
set -a
source "$ENV_FILE"
set +a

ONGRID_DATA_DIR="${ONGRID_DATA_DIR:-/var/lib/ongrid}"
ONGRID_LOG_DIR="${ONGRID_LOG_DIR:-/var/log/ongrid}"

echo "[bootstrap] data dir: $ONGRID_DATA_DIR"
echo "[bootstrap] log dir:  $ONGRID_LOG_DIR"

mkdir -p \
  "$ONGRID_DATA_DIR/mysql" \
  "$ONGRID_DATA_DIR/prometheus" \
  "$ONGRID_DATA_DIR/loki" \
  "$ONGRID_DATA_DIR/tempo" \
  "$ONGRID_DATA_DIR/qdrant" \
  "$ONGRID_DATA_DIR/grafana" \
  "$ONGRID_DATA_DIR/embeddings" \
  "$ONGRID_DATA_DIR/repos" \
  "$ONGRID_DATA_DIR/skills" \
  "$ONGRID_LOG_DIR"

# 嵌入模型：product/embeddings/ → ONGRID_DATA_DIR/embeddings/
BUNDLE_EMB="$SCRIPT_DIR/embeddings/fast-bge-small-zh-v1.5"
TARGET_EMB="$ONGRID_DATA_DIR/embeddings/fast-bge-small-zh-v1.5"
if [[ -f "$BUNDLE_EMB/model_optimized.onnx" ]]; then
  if [[ -f "$TARGET_EMB/model_optimized.onnx" ]]; then
    echo "[bootstrap] embedding model already present at $TARGET_EMB"
  else
    echo "[bootstrap] staging embedding model → $TARGET_EMB"
    mkdir -p "$TARGET_EMB"
    cp -rf "$BUNDLE_EMB/." "$TARGET_EMB/"
  fi
else
  echo "[bootstrap] warn: no bundled embedding model under $BUNDLE_EMB" >&2
fi

# 内置知识库：product/knowledge/builtin_vault → repos/builtin-vault
BUNDLE_VAULT="$SCRIPT_DIR/knowledge/builtin_vault"
TARGET_VAULT="$ONGRID_DATA_DIR/repos/builtin-vault"
if [[ -d "$BUNDLE_VAULT" ]]; then
  VAULT_MD=$(find "$BUNDLE_VAULT" -type f -name '*.md' | wc -l | tr -d ' ')
  if [[ "$VAULT_MD" -eq 0 ]]; then
    echo "[bootstrap] warn: bundled vault has no .md files" >&2
  elif [[ -d "$TARGET_VAULT" ]] && [[ -n "$(find "$TARGET_VAULT" -type f -name '*.md' 2>/dev/null | head -1)" ]]; then
    echo "[bootstrap] builtin vault already present at $TARGET_VAULT ($VAULT_MD md in bundle)"
  else
    echo "[bootstrap] staging builtin vault ($VAULT_MD md) → $TARGET_VAULT"
    mkdir -p "$TARGET_VAULT"
    cp -rf "$BUNDLE_VAULT/." "$TARGET_VAULT/"
  fi
else
  echo "[bootstrap] warn: no bundled vault at $BUNDLE_VAULT" >&2
fi

# 容器镜像 uid（与 deploy/install/install.sh 一致）
chown -R 999:999       "$ONGRID_DATA_DIR/mysql"      2>/dev/null || true
chown -R 65534:65534   "$ONGRID_DATA_DIR/prometheus" 2>/dev/null || true
chown -R 10001:10001   "$ONGRID_DATA_DIR/loki"       2>/dev/null || true
chown -R 10001:10001   "$ONGRID_DATA_DIR/tempo"      2>/dev/null || true
chown -R 472:472       "$ONGRID_DATA_DIR/grafana"    2>/dev/null || true
chown -R 65532:65532   "$ONGRID_DATA_DIR/embeddings" 2>/dev/null || true
chown -R 65532:65532   "$ONGRID_DATA_DIR/repos"      2>/dev/null || true
chown -R 65532:65532   "$ONGRID_DATA_DIR/skills"     2>/dev/null || true
chmod -R 0755          "$ONGRID_DATA_DIR/embeddings" 2>/dev/null || true

echo "[bootstrap] done"
