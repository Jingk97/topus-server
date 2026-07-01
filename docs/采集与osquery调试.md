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

host 画像（丰富档，写死 SQL，见 `internal/agent/collect/collect.go`）：

| osquery 表 | 采的字段 | 组装到 |
|-----------|---------|--------|
| `system_info` | hostname, uuid(=product_uuid), cpu_brand, cpu_logical/physical_cores, physical_memory, hardware_vendor/model/serial | host 基础+CPU+硬件盘点 |
| `os_version` | name, version | host.os |
| `interface_addresses` JOIN `interface_details` | 承载 IP 的网卡 name/ip/mac（过滤环回/链路本地/全0 mac） | host.interfaces |
| `block_devices` | 物理盘 name/model/size（正则过滤分区/合成盘） | host.disks |
| `uptime` | total_seconds | host.uptime_seconds |
| `logged_in_users` | user（type=user 去重） | host.logged_in_users |
| `processes` | pid, path, cmdline, start_time（最小集） | processes |

> 遗留：磁盘 `size_raw` 单位跨平台未归一；mac 上有 APFS 合成盘混入（Linux 干净）。

## 4. 可扩采集的表（参考）

| 表 | 能采什么 | 切片 |
|----|---------|------|
| `interface_addresses` / `interface_details` | 网卡 IP / MAC | 可入 S1 host 画像 |
| `uptime` | 开机时长 | host 画像 |
| `logged_in_users` | 登录用户 | host 画像 |
| `listening_ports` / `process_open_sockets` | 监听端口 / 连接 | **S2**（拓扑） |
| `deb_packages` / `rpm_packages` | 软件包清单 | 后期 |

> 连接/拓扑相关（`listening_ports` 等）是 S2 范围，S1 克制不采。要扩 S1 host 画像字段，改 `collect.go` 的写死 SQL 并补单测即可。

## 5. 在 Linux 真机测采集（交叉编译 + 部署）

agent 开发在 mac，但**部署目标是 Linux**。在 Linux 真机验证采集（磁盘/网卡比 mac 干净、进程 cmdline 能拿全）：

**① mac 上交叉编译 linux agent**（静态二进制，不依赖目标机装 Go）：

```bash
GOOS=linux GOARCH=amd64 go build -o bin/topus-agent-linux-amd64 ./cmd/agent
# arm64 机器：GOOS=linux GOARCH=arm64 go build -o bin/topus-agent-linux-arm64 ./cmd/agent
```

**② scp 到 Linux 真机**：

```bash
scp bin/topus-agent-linux-amd64 user@<linux-host>:~/
```

**③ Linux 真机上拉 osqueryd + 跑采集**：

```bash
# 拉 linux osqueryd（联网一次；arm64 机器把 x86_64 换成 aarch64）
curl -sSL -o osq.tgz https://github.com/osquery/osquery/releases/download/5.23.0/osquery-5.23.0_1.linux_x86_64.tar.gz
tar xzf osq.tgz usr/bin/osqueryd
mv usr/bin/osqueryd ./osqueryd          # 放 agent 同目录 → agent 自动找（免 --osqueryd）

chmod +x topus-agent-linux-amd64 osqueryd
sudo ./topus-agent-linux-amd64 collect  # 自动找同目录 osqueryd；sudo 拿全进程 cmdline
```

> **agent 怎么找 osqueryd**（一处定义，见 `internal/agent/osq/resolve.go`）：按优先级
> ① `--osqueryd` 显式 → ② `TOPUS_OSQUERYD` 环境变量 → ③ **agent 同目录**（`topus-agentd`/`osqueryd`/mac bundle）→ ④ 开发期项目路径 `deploy/osquery/bin/<os-arch>` → ⑤ 系统 `PATH`。
> 所以"osqueryd 放 agent 旁边"就自动命中——这也是产品化**统一包/embed**（osqueryd 落盘为 `topus-agentd`）的查找路径。

**Linux 预期（验证点）**：
- **磁盘**：干净物理盘（`/dev/sda`、`/dev/nvme0n1`），无 mac 那种 APFS 合成盘噪音。
- **网卡**：`eth0`/`ens*`/`enp*` + 真实 IP/MAC。
- **进程 cmdline**：sudo 下应拿全（mac 非 root 会有空）。

> osqueryd 是 Linux 普通静态二进制（不像 mac 要完整 .app bundle），单文件即可跑。

## 6. 产品化形态：单文件 embed（osqueryd 内嵌进 agent）

§5 是**开发期**形态：agent 与 osqueryd 两个文件、osqueryd 靠 fetch 脚本单独拉。
**产品化/部署**形态是**一个二进制搞定**：osqueryd 用 `//go:embed` 打进 agent，运行时解压
落盘为 `topus-agentd` 再拉起。用户只需下载、跑一个文件，无需联网拉 osqueryd。

**① 构建单文件 embed 版**（mac 上交叉编译，产物 ~102MB = agent 18MB + 内嵌 osqueryd 84MB）：

```bash
deploy/build-agent-embed.sh amd64        # arm64 机器：deploy/build-agent-embed.sh arm64
# 产物：bin/topus-agent-linux-amd64-embed（-tags embedosq）
```

**② scp 到 Linux 真机、核对 md5、直接跑**（不需要再单独放 osqueryd）：

```bash
scp bin/topus-agent-linux-amd64-embed user@<linux-host>:~/
md5sum topus-agent-linux-amd64-embed     # 与本机 `md5 bin/...` 比对，确认传的是新版
chmod +x topus-agent-linux-amd64-embed
sudo ./topus-agent-linux-amd64-embed collect
```

**预期**：`拉起 osqueryd` → `osqueryd 就绪，extension socket 已连通` → `采集完成`（host/
cpu/interfaces/disks/process_count 概览）→ 完整 JSON 快照。

**运行时机制**（见 `internal/agent/osq/embed_osqueryd.go` + `daemon.go`）：
- 首次运行把内嵌的 osqueryd 解压到 `~/.cache/topus/topus-agentd`（sudo 下即 `/root/.cache/topus/`），
  按 sha256 判重复用；空盘/损坏/无权限都**硬报错不静默**（无 `/tmp` 兜底）。
- 拉起时 socket 放 `/tmp/tpx-osq-*/osq.em`（mac 104 字节路径上限），配 `--ephemeral` 内存态。

**两个真机踩坑（已修，留档避免复犯）**：

| 现象 | 根因 | 修法 |
|------|------|------|
| `Error reading config: config file does not exist: /dev/null` 后 socket 超时 | osqueryd 读配置走"安全读文件"，只收普通文件；`/dev/null` 是字符设备被拒 → 报错退出 | 写真实空 JSON 配置 `{}`，`--config_path` 指它 |
| 日志出现 `Using a virtual database. Need help, type '.help'`（osqueryi shell 横幅）后 socket 超时 | osquery 按 `argv[0]` basename 判模式：非 `osqueryd` → 进交互 shell；agent 无 tty→EOF 退出→socket 消失 | 拉起时强制 `argv[0]="osqueryd"` 锁 daemon 模式 |

> 排错口诀：**socket 就绪超时 ≠ osqueryd 没打进来**——能走到"等 socket"说明 osqueryd 已解压+exec，
> 问题在它起来后为何不建/不留 socket。手动 `sudo <落盘的 topus-agentd> ... --verbose` 看它自己的日志最快。

---
← 返回 [README](../README.md) · 技术内幕见 docs 仓《系统技术手册》§4.6
