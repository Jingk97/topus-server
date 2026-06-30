# topus-server

Topus（全自动动态 CMDB）的 server 端（含 agent）。当前 **S1 行走骨架**已落：
① health 链路预检（明文 + 单向 TLS）；② agent 本地采集闭环（监管 osqueryd 采 host/进程）。

## 目录约定

```
api/topus/<模块>/v<n>/   # proto 契约 + buf 生成的 *.pb.go（proto 优先，buf 管理）
cmd/server/              # server 可执行入口（一应用一二进制）
cmd/agent/               # agent 入口（test 链路预检 / collect 本地采集 子命令）
internal/service/        # 服务实现（不依赖框架，handler 为适配器）
internal/agent/          # agent 端：osq(监管 osqueryd) / collect(采集组装)
internal/biz/<上下文>/   # 各限界上下文领域内核（后续切片填充）
internal/data/           # repo 适配器（PostgreSQL / NATS，后续）
deploy/osquery/          # osqueryd 拉取(fetch.sh)；bin/ gitignore 不提交
bin/                     # 构建产物（gitignore，不提交）
scripts/                 # 冒烟等脚本
```

## 工具链

- Go 1.22+
- buf（proto 生成）、protoc-gen-go、protoc-gen-go-grpc：
  ```bash
  go install github.com/bufbuild/buf/cmd/buf@latest
  go install google.golang.org/protobuf/cmd/protoc-gen-go@latest
  go install google.golang.org/grpc/cmd/protoc-gen-go-grpc@latest
  ```
  确保 `$(go env GOPATH)/bin` 在 `PATH` 中。

## 如何验证（怎样算通过）

> 验证信号分两层：**单元/契约测试**（快、纯逻辑）+ **端到端冒烟**（强、双进程真连）。
> 行走骨架的"主验收信号"是端到端冒烟——它才覆盖"进程能起 + 端口可达 + Ping 通"。

| 命令 | 验证什么 | 通过判据 |
|------|---------|---------|
| `make gen` | 由 proto 生成 Go 代码 | `api/.../health.pb.go`、`*_grpc.pb.go` 生成 |
| `make test` | 单元/契约：bufconn 内存管道调 Ping | `go test` 全绿（`TestHealthPing` PASS） |
| `make smoke` | **端到端**：起真 server 进程 → `topus-agent test` 真连 | 打印 `SMOKE PASS: ok server=... time=...`，退出码 0 |
| `make collect` | agent 本地采集 host+进程（监管 osqueryd） | 日志显示采集概览 + 输出快照 JSON，进程数与 `ps` 接近 |

> 采集的**手动测试方法**与 **osquery shell 探索**（怎么看采到什么、怎么探可扩字段），见
> [采集与 osquery 调试指南](docs/采集与osquery调试.md)。

手动复现端到端（等价于 `make smoke`）：

```bash
make build
./bin/topus-server -addr=127.0.0.1:9090 &      # 起 server
./bin/topus-agent test --server=127.0.0.1:9090  # 预检；通则打印 ok、退出 0
```

链路不通时 `topus-agent test` 打印明确失败并以非 0 退出（退 1 = 链路不通，退 2 = 用法错）。

## 验收映射（S1）

- 本刀对应方案《纳管批次与接入安全》§3.8「测试命令（链路预检）」：批量纳管前在样本机
  先跑 `topus-agent test` 测绿，再批量铺。
- 完整 S1 验收（采集 / 注册发证 / 上报落库 / 新鲜度 / 撞值检出）见
  `../docs/changes/2026-06-s1-行走骨架/切片方案与契约.md` §4，按后续切片逐条补失败测试。
