# 采集与 osquery 调试指南

> how-to（操作类）：怎么手动跑 agent 采集、怎么用 osquery shell 探索可采字段。
> 技术内幕（osquery 集成的设计/平台坑）见 docs 仓《系统技术手册》§4.6；本文只讲"怎么操作"。

## 0. 前置：把 osqueryd 拉到项目内

osqueryd 不装宿主机、不入 git，用脚本现拉到项目目录（`deploy/osquery/bin/<os-arch>/`）：

```bash
make osquery          # 等价于 bash deploy/osquery/fetch.sh，已存在则跳过
```

- macOS 拉的是完整 `osquery.app` bundle（单抠二进制跑不了）；Linux 是单文件 `osqueryd`。
- 版本固定在 `deploy/osquery/VERSION`（当前 5.23.0）。

## 1. 跑 agent 采集，看实际采到什么

```bash
make collect                                   # 一键：拉osquery + 构建 + 采集

# 细粒度：
./bin/topus-agent collect                      # 日志(stderr) + 快照JSON(stdout)
./bin/topus-agent collect --json=false         # 只看日志概览（采了啥/进程数/耗时）
./bin/topus-agent collect > /tmp/snap.json     # JSON 存文件，日志仍在屏幕
python3 -m json.tool /tmp/snap.json | less     # 美化浏览
grep -c '"pid"' /tmp/snap.json                 # 数采到多少进程
```

**观察点**：
- stderr 结构化日志：`拉起 osqueryd → 就绪 → 采集完成`，含 host / product_uuid / 进程数 / 耗时。
- stdout 快照 JSON：完整 `host`（hostname/product_uuid/OS/CPU/内存）+ `processes`（pid/exe_path/cmdline/start_time）。

> mac 开发机上部分系统进程 `cmdline` 为空（非 root 权限限制）；生产 Linux 下 agent 以特权跑能拿全。

## 2. osquery shell：自己 SQL 探索能采什么（扩采集探路）

osquery 有 186 张表，可用 shell 交互探索，决定要扩采哪些字段：

```bash
OSQD="deploy/osquery/bin/darwin-arm64/osquery.app/Contents/MacOS/osqueryd"   # mac
# Linux: OSQD="deploy/osquery/bin/linux-amd64/osqueryd"

"$OSQD" -S                                     # 进交互 shell（osquery> 提示符）
```

shell 内常用：

```sql
.tables                      -- 列出全部表
.tables process              -- 按关键字筛表名
.schema listening_ports      -- 看某表字段定义
.mode line                   -- 多列竖排（看着清楚）
SELECT pid,name,path FROM processes LIMIT 5;
SELECT * FROM interface_addresses;     -- 网卡 IP
.quit
```

非交互一条命令（脚本/快速看）：

```bash
"$OSQD" -S --json "SELECT name,path FROM processes WHERE name LIKE '%ssh%'"
```

## 3. 当前采集的表与字段

S1 最小集（写死 SQL，见 `internal/agent/collect/collect.go`）：

| 表 | 采的字段 | 组装到 |
|----|---------|--------|
| `system_info` | hostname, uuid, cpu_logical_cores, physical_memory | host（uuid = product_uuid） |
| `os_version` | name, version | host |
| `processes` | pid, path, cmdline, start_time | processes |

## 4. 可扩采集的表（参考）

| 表 | 能采什么 | 切片 |
|----|---------|------|
| `interface_addresses` / `interface_details` | 网卡 IP / MAC | 可入 S1 host 画像 |
| `uptime` | 开机时长 | host 画像 |
| `logged_in_users` | 登录用户 | host 画像 |
| `listening_ports` / `process_open_sockets` | 监听端口 / 连接 | **S2**（拓扑） |
| `deb_packages` / `rpm_packages` | 软件包清单 | 后期 |

> 连接/拓扑相关（`listening_ports` 等）是 S2 范围，S1 克制不采。要扩 S1 host 画像字段，改 `collect.go` 的写死 SQL 并补单测即可。

---
← 返回 [README](../README.md) · 技术内幕见 docs 仓《系统技术手册》§4.6
