#!/usr/bin/env bash
# 端到端冒烟：验证 agent 与 server 两个二进制能经【单向 TLS】gRPC 真实通信。
# 主验收信号——覆盖"进程能起 + TLS 握手 + agent 用 CA 验 server + Ping 通"。
set -euo pipefail

ADDR="127.0.0.1:19090"   # 用非默认端口，避免撞占用
CERTS="certs"

# 1 生成开发证书（ca.pem / server.pem / server-key.pem）。
go run ./cmd/gen-certs -dir="$CERTS" -hosts=127.0.0.1,localhost >/dev/null

# 2 后台起真 server 进程（单向 TLS）；脚本退出时务必回收，避免残留占端口。
./bin/topus-server -addr="$ADDR" -tls-cert="$CERTS/server.pem" -tls-key="$CERTS/server-key.pem" &
SRV=$!
trap 'kill "$SRV" 2>/dev/null || true' EXIT

# 3 轮询重试 agent 预检（带 --ca 验 server）：监听就绪有时间差，必须轮询而非一次性。
for _ in $(seq 1 25); do
  if ./bin/topus-agent test --server="$ADDR" --ca="$CERTS/ca.pem" >/tmp/topus_smoke.out 2>&1; then
    echo "SMOKE PASS (单向 TLS): $(cat /tmp/topus_smoke.out)"
    exit 0
  fi
  sleep 0.2
done

echo "SMOKE FAIL"
cat /tmp/topus_smoke.out || true
exit 1
