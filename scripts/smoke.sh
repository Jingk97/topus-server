#!/usr/bin/env bash
# 端到端冒烟：验证 agent 与 server 两个二进制能经 gRPC 真实通信。
# 主验收信号（行走骨架第一刀）——比单元测试更强：覆盖"进程能起 + 端口可达 + Ping 通"。
set -euo pipefail

ADDR="127.0.0.1:19090"   # 用非默认端口，避免撞占用

# 1 后台起真 server 进程；脚本退出时务必回收，避免残留进程占端口。
./bin/topus-server -addr="$ADDR" &
SRV=$!
trap 'kill "$SRV" 2>/dev/null || true' EXIT

# 2 轮询重试 agent 预检：server 启动有极短窗口，重试至通或超时。
#   不能起完 server 立刻连——监听就绪有时间差，故必须轮询而非一次性。
for _ in $(seq 1 25); do
  if ./bin/topus-agent test --server="$ADDR" >/tmp/topus_smoke.out 2>&1; then
    echo "SMOKE PASS: $(cat /tmp/topus_smoke.out)"
    exit 0
  fi
  sleep 0.2
done

echo "SMOKE FAIL"
cat /tmp/topus_smoke.out || true
exit 1
