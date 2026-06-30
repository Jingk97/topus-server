#!/usr/bin/env bash
# 拉取固定版 osquery 到项目内 deploy/osquery/bin/<os-arch>/。
#
# 设计：osqueryd 不装宿主机、不进 git(bin/ 已 gitignore)，靠本脚本现拉到项目目录，
# agent 从项目内路径找它(见 internal/agent/osq)。版本固定在同目录 VERSION 文件。
#
# 平台差异(重要)：
#   - macOS：osqueryd 依赖完整 .app bundle(签名+资源)，单抠二进制跑不了 →
#            提取整个 osquery.app，osqueryd 在 osquery.app/Contents/MacOS/osqueryd。
#   - Linux：usr/bin/osqueryd 是普通静态二进制，单文件即可。
set -euo pipefail
cd "$(dirname "$0")"
VERSION="$(tr -d '[:space:]' < VERSION)"

# 1 识别平台，映射「资产名 / 提取方式 / 输出目录」。
OS="$(uname -s)"
ARCH="$(uname -m)"
KIND=file          # file=单二进制 / bundle=整个 .app
case "$OS-$ARCH" in
  Darwin-arm64)
    ASSET="osquery-${VERSION}_1.macos_arm64.tar.gz"
    KIND=bundle
    SRC="opt/osquery/lib/osquery.app"
    OUT="darwin-arm64" ;;
  Linux-x86_64)
    ASSET="osquery-${VERSION}_1.linux_x86_64.tar.gz"
    SRC="usr/bin/osqueryd"
    OUT="linux-amd64" ;;
  Linux-aarch64)
    ASSET="osquery-${VERSION}_1.linux_aarch64.tar.gz"
    SRC="usr/bin/osqueryd"
    OUT="linux-arm64" ;;
  *)
    echo "unsupported platform: $OS-$ARCH" >&2
    exit 1 ;;
esac

DEST="bin/$OUT"
# osqueryd 最终路径(mac 在 bundle 内, linux 在目录下)。
if [ "$KIND" = bundle ]; then
  OSQD="$DEST/osquery.app/Contents/MacOS/osqueryd"
else
  OSQD="$DEST/osqueryd"
fi

# 2 已存在则跳过(幂等)。
if [ -x "$OSQD" ]; then
  echo "already present: $OSQD"
  exit 0
fi

# 3 下载 -> 解包 -> 放到项目内 bin/<os-arch>/。
URL="https://github.com/osquery/osquery/releases/download/${VERSION}/${ASSET}"
TMP="$(mktemp -d)"
trap 'rm -rf "$TMP"' EXIT
echo "downloading osqueryd ${VERSION} [$OUT]: $URL"
curl -sSL -o "$TMP/osq.tar.gz" "$URL"
tar xzf "$TMP/osq.tar.gz" -C "$TMP" "$SRC"
mkdir -p "$DEST"
if [ "$KIND" = bundle ]; then
  rm -rf "$DEST/osquery.app"
  cp -R "$TMP/$SRC" "$DEST/osquery.app"
else
  cp "$TMP/$SRC" "$DEST/osqueryd"
  chmod +x "$DEST/osqueryd"
fi
echo "placed: $OSQD"
"$OSQD" --version
# TODO(later): SHA256 checksum 校验(每平台一行)防供应链篡改。
