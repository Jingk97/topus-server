// Package osq 封装 osqueryd 子进程的拉起监管与查询客户端（客户端模式）。
//
// agent 当父进程拉起 osqueryd（ephemeral 内存态），经 extension socket 用
// osquery-go 客户端跑写死 SQL 取当前态——见《agent 功能构成与 osquery 边界》§4。
package osq

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/osquery/osquery-go"
)

// osqueryd 路径定位见 resolve.go 的 ResolvePath（覆盖开发/统一包/embed/显式四种来源）。

// Daemon 管理一个被拉起的 osqueryd 子进程及其 extension socket。
type Daemon struct {
	cmd     *exec.Cmd
	socket  string
	tmpDir  string
	pidfile string        // 记录本子进程 PID，供下次启动清理 kill -9 遗留的孤儿
	done    chan struct{} // reaper goroutine 收尸后关闭，表示 cmd 已退出
	waitErr error         // cmd.Wait 的返回（子进程退出原因）
}

// Start 拉起 osqueryd（ephemeral）并等 extension socket 就绪。
//
// 关键点：① ephemeral = 内存态，不写持久 RocksDB/pidfile，避免单机多实例锁冲突；
// ② socket 文件出现 ≠ 可连，必须轮询试连（osquery 官方实证，~200ms 延迟）。
func Start(ctx context.Context, osquerydPath string, log *slog.Logger) (*Daemon, error) {
	// 统一转绝对路径：exec 对不含 "/" 的名字会去 $PATH 查找（并非用当前目录），
	// 绝对路径可避免"误查 PATH / 报 not found in $PATH"，让缺失时给出确定的路径错误。
	// Abs 失败（取不到工作目录）属异常，直接报错——不静默保留相对路径继续。
	abs, err := filepath.Abs(osquerydPath)
	if err != nil {
		return nil, fmt.Errorf("解析 osqueryd 绝对路径失败（%s）：%w", osquerydPath, err)
	}
	osquerydPath = abs
	if _, err := os.Stat(osquerydPath); err != nil {
		return nil, fmt.Errorf("osqueryd 不可用（%s）：%w", osquerydPath, err)
	}
	// 0 每次启动先清理上次 agent 异常退出（如 kill -9）遗留的采集子进程，
	//   再重新拉起——避免孤儿 osqueryd 持续泄漏（不静默：杀了/失败都记日志）。
	killStale(osquerydPath, log)

	// macOS unix socket 路径有 104 字节上限，而 os.TempDir 在 mac 是很长的 /var/folders/...，
	// 故 socket 放短路径 /tmp 下避免超限（S1 仅 linux/mac）。
	tmpDir, err := os.MkdirTemp("/tmp", "tpx-osq-")
	if err != nil {
		return nil, err
	}
	sock := filepath.Join(tmpDir, "osq.em")

	// 写一个空 JSON 配置文件（{}）供 osqueryd 读取。
	// 关键：不能用 --config_path=/dev/null——osqueryd 读配置走"安全读文件"，只收普通文件，
	//   /dev/null 是字符设备会被判成"config file does not exist"而报错退出（Linux 上更严）。
	//   写一个真实空配置，既能被正常读取、又不加载任何外部配置。
	confPath := filepath.Join(tmpDir, "osq.conf")
	if err := os.WriteFile(confPath, []byte("{}"), 0o600); err != nil {
		_ = os.RemoveAll(tmpDir)
		return nil, fmt.Errorf("写 osqueryd 空配置：%w", err)
	}

	// 1 拉起 osqueryd：ephemeral 内存态 + 空本地配置（不拉远程）。
	// TODO(产品化): --disable_watchdog 与 stderr 直透是开发期取值；产品形态应默认开 watchdog
	//   （防 osqueryd 内存/CPU 失控）+ 日志走文件/丢弃。做成 build-tag/配置开关（下一任务设计）。
	cmd := exec.CommandContext(ctx, osquerydPath,
		"--extensions_socket="+sock,
		"--ephemeral",
		"--disable_logging=true",
		"--disable_watchdog=true",
		"--config_path="+confPath,
	)
	// 关键：osquery 是"一个二进制两种人格"，靠 argv[0] 的 basename 判定模式——
	//   basename == "osqueryd" → daemon（守护进程，我们要的：常驻、只服务 extension socket）；
	//   其它任何名字（含我们落盘的 topus-agentd）→ osqueryi 交互 shell（读 stdin 的 REPL）。
	// agent 拉起时无 tty，若进了 shell 模式会立刻 EOF 退出、socket 随之消失 → agent 轮询超时。
	// 故这里必须把 argv[0] 强制成 "osqueryd" 锁定 daemon 模式（文件名仍是 topus-agentd，仅 argv[0] 变）。
	// 代价：ps 里进程名显示 osqueryd 而非 topus-agentd；进程名品牌化需 patch osquery，留待产品化。
	// 副作用：cmdline 不再含落盘路径，故 killStale 不能靠路径匹配——改用 pidfile（见下）。
	cmd.Args[0] = "osqueryd"
	cmd.Stderr = os.Stderr // 开发期透出 osqueryd 日志便于排查（产品化改，见上 TODO）
	if err := cmd.Start(); err != nil {
		_ = os.RemoveAll(tmpDir)
		return nil, fmt.Errorf("启动 osqueryd：%w", err)
	}
	d := &Daemon{cmd: cmd, socket: sock, tmpDir: tmpDir, pidfile: pidfilePath(osquerydPath), done: make(chan struct{})}
	// reaper：唯一调用 cmd.Wait 的地方（Stop 不再自己 Wait，避免重复 Wait 报错）。
	// 它收尸后关 done，waitReady 据此发现"启动即崩溃"，Stop 据此等待彻底退出。
	go func() {
		d.waitErr = cmd.Wait()
		close(d.done)
	}()

	// 2 轮询 socket 就绪（试连成功才算 ready；期间若子进程已崩溃则提前带因返回）。
	if err := d.waitReady(ctx); err != nil {
		d.Stop()
		return nil, err
	}

	// 3 就绪后记录 PID 到 pidfile，供下次启动清理本 agent 遗留的孤儿。
	//   best-effort：写不了（如 osqueryd 所在目录只读）只告警，不影响本次采集。
	if err := os.WriteFile(d.pidfile, []byte(strconv.Itoa(cmd.Process.Pid)), 0o600); err != nil {
		log.Warn("写 osqueryd pidfile 失败（不影响本次；下次清理残留会缺记录）", "pidfile", d.pidfile, "err", err)
	}
	return d, nil
}

// waitReady 轮询试连 extension socket，直到成功、子进程崩溃或超时。
func (d *Daemon) waitReady(ctx context.Context) error {
	deadline := time.Now().Add(15 * time.Second)
	for time.Now().Before(deadline) {
		// 子进程启动即退出（崩溃/参数错）时 socket 永不出现，不必空等满 15s，
		// 带上真实退出原因立即返回，便于定位。
		if d.exited() {
			return fmt.Errorf("osqueryd 启动后即退出（%v），extension socket 未建立（socket=%s）", d.waitErr, d.socket)
		}
		c, err := osquery.NewClient(d.socket, 3*time.Second)
		if err == nil {
			c.Close()
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(200 * time.Millisecond):
		}
	}
	return fmt.Errorf("osqueryd extension socket 就绪超时（%s）", d.socket)
}

// exited 非阻塞判断子进程是否已退出（reaper 已关 done）。
func (d *Daemon) exited() bool {
	select {
	case <-d.done:
		return true
	default:
		return false
	}
}

// Client 新建一个连到该 osqueryd 的查询客户端。调用方负责 Close。
func (d *Daemon) Client() (*osquery.ExtensionManagerClient, error) {
	return osquery.NewClient(d.socket, 5*time.Second)
}

// Stop 停止 osqueryd 并清理临时目录与 pidfile。
func (d *Daemon) Stop() {
	if d.cmd != nil && d.cmd.Process != nil {
		_ = d.cmd.Process.Kill()
		<-d.done // 等 reaper 收尸，避免留下僵尸进程
	}
	if d.pidfile != "" {
		_ = os.Remove(d.pidfile) // 正常退出清掉记录，下次不再误当残留
	}
	if d.tmpDir != "" {
		_ = os.RemoveAll(d.tmpDir)
	}
}

// pidfilePath 返回与 osqueryd 同目录的 pidfile 路径。
// 同目录 = 每个 osqueryd 落盘位置各自独立（embed 的 ~/.cache/topus、开发期 deploy 目录互不干扰）。
// 注意（S1 约定）：以"单机单 agent 实例"为前提；同路径并发多实例会共用此 pidfile 而相互清理，S1 不支持。
func pidfilePath(osquerydPath string) string {
	return filepath.Join(filepath.Dir(osquerydPath), "osqueryd.pid")
}

// killStale 清理上次 agent 异常退出（kill -9，没走 Stop）遗留的、由本 agent 拉起的 osqueryd 孤儿。
//
// 为何不再用 pkill -f <路径>：我们把子进程 argv[0] 锁成了 "osqueryd"（见 Start），其
// /proc/pid/cmdline 已不含 osqueryd 落盘路径，pkill -f <路径> 恒匹配不到、形同虚设；且
// 显式绝对路径运行时还会误杀 agent 自身。改用 pidfile：只清我们上次写下的那个 PID，且
// 下手前校验它确实还是我们那个 osqueryd（防 PID 被系统回收后误杀无辜进程）。
func killStale(osquerydPath string, log *slog.Logger) {
	pf := pidfilePath(osquerydPath)
	data, err := os.ReadFile(pf)
	if err != nil {
		return // 无 pidfile = 无上次残留记录（首次运行或已正常清理），正常
	}
	pid, perr := strconv.Atoi(strings.TrimSpace(string(data)))
	if perr != nil || pid <= 0 {
		_ = os.Remove(pf)
		log.Warn("osqueryd pidfile 内容非法，已清除", "pidfile", pf)
		return
	}
	// 身份校验：确认该 PID 仍存活且确是我们那个 osqueryd，避免 PID 回收误杀。
	if !isOurOsqueryd(pid, osquerydPath) {
		_ = os.Remove(pf) // 陈旧记录：进程早退了，或 PID 已被别的程序占用
		return
	}
	p, _ := os.FindProcess(pid) // Linux 上 FindProcess 必不报错
	if kerr := p.Kill(); kerr != nil {
		log.Warn("清理残留 osqueryd 失败（忽略继续）", "pid", pid, "path", osquerydPath, "err", kerr)
	} else {
		log.Info("已清理残留采集进程", "pid", pid, "path", osquerydPath)
	}
	_ = os.Remove(pf)
}

// isOurOsqueryd 判断 pid 是否仍是本 agent 拉起的那个 osqueryd。
//
// Linux：读 /proc/<pid>/exe 软链，精确比对是否等于我们落盘的二进制路径——
//
//	这不受 argv[0] 改名影响（exe 指向真实文件），零误判。
//
// 非 Linux（mac 开发期）：无 /proc，退化为"存活即认"。pidfile 由本 agent 独占写入，
//
//	仅当 PID 被系统回收给别的进程才会误判，概率极低；精确匹配留待需要时按平台补。
func isOurOsqueryd(pid int, osquerydPath string) bool {
	if runtime.GOOS == "linux" {
		exe, err := os.Readlink(fmt.Sprintf("/proc/%d/exe", pid))
		if err != nil {
			return false // 进程已不在，或无权读——不是"确定是我们的"，就不动它
		}
		return exe == osquerydPath
	}
	return processAlive(pid)
}

// processAlive 用 signal 0 探测进程是否存活（不真正发信号）。
func processAlive(pid int) bool {
	p, err := os.FindProcess(pid)
	if err != nil {
		return false
	}
	return p.Signal(syscall.Signal(0)) == nil
}
