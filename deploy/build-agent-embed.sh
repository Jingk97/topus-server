#!/usr/bin/env bash
# 构建内嵌 osqueryd 的单文件 linux agent (产品化形态)。
#
# osqueryd 不入 git: 这里构建时临时 fetch + gzip 进 assets/, go build -tags embedosq
# 把它 embed 进二进制; 运行时 agent 解压落盘为 topus-agentd (见 embed_osqueryd.go)。
# 用法: deploy/build-agent-embed.sh [amd64|arm64]   (默认 amd64)
set -euo pipefail
cd "$(dirname "$0")/.."   # → server 根

VERSION="$(tr -d '[:space:]' < deploy/osquery/VERSION)"
ARCH="${1:-amd64}"
case "$ARCH" in
  amd64) OSQ_ASSET="osquery-${VERSION}_1.linux_x86_64.tar.gz" ;;
  arm64) OSQ_ASSET="osquery-${VERSION}_1.linux_aarch64.tar.gz" ;;
  *) echo "用法: $0 [amd64|arm64]" >&2; exit 1 ;;
esac
INNER="opt/osquery/bin/osqueryd"   # usr/bin/osqueryd 是软链, 取真实二进制
GZ="internal/agent/osq/assets/osqueryd.gz"
OUT="bin/topus-agent-linux-${ARCH}-embed"

# 1 fetch linux osqueryd -> gzip 进 assets (不入 git)。
echo "fetch + gzip linux/${ARCH} osqueryd ${VERSION} ..."
TMP="$(mktemp -d)"
trap 'rm -rf "$TMP"' EXIT
curl -sSL -o "$TMP/osq.tgz" "https://github.com/osquery/osquery/releases/download/${VERSION}/${OSQ_ASSET}"
tar xzf "$TMP/osq.tgz" -C "$TMP" "$INNER"
gzip -c "$TMP/$INNER" > "$GZ"
echo "assets/osqueryd.gz size: $(wc -c < "$GZ") bytes"

# 2 交叉编译 embed 版 agent (单文件, 内嵌 osqueryd)。
echo "build ${OUT} (-tags embedosq) ..."
mkdir -p bin
GOOS=linux GOARCH="$ARCH" go build -tags embedosq -o "$OUT" ./cmd/agent
echo "done: ${OUT} size: $(wc -c < "$OUT") bytes (内嵌 osqueryd, 落盘为 topus-agentd)"
