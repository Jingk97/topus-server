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
	"time"

	"github.com/osquery/osquery-go"
)

// osqueryd 路径定位见 resolve.go 的 ResolvePath（覆盖开发/统一包/embed/显式四种来源）。

// Daemon 管理一个被拉起的 osqueryd 子进程及其 extension socket。
type Daemon struct {
	cmd    *exec.Cmd
	socket string
	tmpDir string
}

// Start 拉起 osqueryd（ephemeral）并等 extension socket 就绪。
//
// 关键点：① ephemeral = 内存态，不写持久 RocksDB/pidfile，避免单机多实例锁冲突；
// ② socket 文件出现 ≠ 可连，必须轮询试连（osquery 官方实证，~200ms 延迟）。
func Start(ctx context.Context, osquerydPath string, log *slog.Logger) (*Daemon, error) {
	// 统一转绝对路径：exec 对不含 "/" 的名字会去 $PATH 查找（并非用当前目录），
	// 绝对路径可避免"误查 PATH / 报 not found in $PATH"，让缺失时给出确定的路径错误。
	if abs, err := filepath.Abs(osquerydPath); err == nil {
		osquerydPath = abs
	}
	if _, err := os.Stat(osquerydPath); err != nil {
		return nil, fmt.Errorf("osqueryd 不可用（%s）：%w", osquerydPath, err)
	}
	// 0 每次启动先清理上次 agent 异常退出（如 kill -9）遗留的同路径采集子进程，
	//   再重新拉起——避免端口/socket/资源被僵尸进程占用（不静默：杀了记日志）。
	killStale(osquerydPath, log)

	// macOS unix socket 路径有 104 字节上限，而 os.TempDir 在 mac 是很长的 /var/folders/...，
	// 故 socket 放短路径 /tmp 下避免超限（S1 仅 linux/mac）。
	tmpDir, err := os.MkdirTemp("/tmp", "tpx-osq-")
	if err != nil {
		return nil, err
	}
	sock := filepath.Join(tmpDir, "osq.em")

	// 1 拉起 osqueryd：ephemeral 内存态 + 无远程配置。
	// TODO(产品化): --disable_watchdog 与 stderr 直透是开发期取值；产品形态应默认开 watchdog
	//   （防 osqueryd 内存/CPU 失控）+ 日志走文件/丢弃。做成 build-tag/配置开关（下一任务设计）。
	cmd := exec.CommandContext(ctx, osquerydPath,
		"--extensions_socket="+sock,
		"--ephemeral",
		"--disable_logging=true",
		"--disable_watchdog=true",
		"--config_path=/dev/null",
	)
	cmd.Stderr = os.Stderr // 开发期透出 osqueryd 日志便于排查（产品化改，见上 TODO）
	if err := cmd.Start(); err != nil {
		_ = os.RemoveAll(tmpDir)
		return nil, fmt.Errorf("启动 osqueryd：%w", err)
	}
	d := &Daemon{cmd: cmd, socket: sock, tmpDir: tmpDir}

	// 2 轮询 socket 就绪（试连成功才算 ready）。
	if err := d.waitReady(ctx); err != nil {
		d.Stop()
		return nil, err
	}
	return d, nil
}

// waitReady 轮询试连 extension socket，直到成功或超时。
func (d *Daemon) waitReady(ctx context.Context) error {
	deadline := time.Now().Add(15 * time.Second)
	for time.Now().Before(deadline) {
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

// Client 新建一个连到该 osqueryd 的查询客户端。调用方负责 Close。
func (d *Daemon) Client() (*osquery.ExtensionManagerClient, error) {
	return osquery.NewClient(d.socket, 5*time.Second)
}

// Stop 停止 osqueryd 并清理临时目录。
func (d *Daemon) Stop() {
	if d.cmd != nil && d.cmd.Process != nil {
		_ = d.cmd.Process.Kill()
		_ = d.cmd.Wait()
	}
	if d.tmpDir != "" {
		_ = os.RemoveAll(d.tmpDir)
	}
}

// killStale 杀掉上次 agent 异常退出遗留的、同一可执行路径的采集子进程。
//
// 用 pkill -f 匹配命令行含该绝对路径的进程（落盘路径唯一，匹配精确）。S1 仅 linux/mac。
// pkill 退出码：0=杀掉了（记日志）；1=无匹配（正常，无残留）；其它/不存在=忽略继续（不致命）。
func killStale(osquerydPath string, log *slog.Logger) {
	err := exec.Command("pkill", "-f", osquerydPath).Run()
	if err == nil {
		log.Info("已清理残留采集进程", "path", osquerydPath)
		return
	}
	// 区分"无匹配(exit 1)"与真异常：无匹配是常态，不打扰；其它降级为 Warn。
	var ee *exec.ExitError
	if ok := asExitError(err, &ee); ok && ee.ExitCode() == 1 {
		return
	}
	log.Warn("清理残留采集进程未完成（忽略继续）", "path", osquerydPath, "err", err)
}

// asExitError 是 errors.As 的小包装（避免在调用处引入 errors 包）。
func asExitError(err error, target **exec.ExitError) bool {
	if ee, ok := err.(*exec.ExitError); ok {
		*target = ee
		return true
	}
	return false
}
