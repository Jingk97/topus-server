//go:build embedosq

package osq

import (
	"bytes"
	"compress/gzip"
	_ "embed"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

// embeddedOsquerydGz 是发布构建内嵌的 osqueryd（gzip 压缩）。
// 由 deploy/build-agent-embed.sh 在构建前 fetch + gzip 写入 assets/osqueryd.gz（不入 git）。
//
//go:embed assets/osqueryd.gz
var embeddedOsquerydGz []byte

// extractEmbedded 把内嵌的 osqueryd 解压落盘成 topus-agentd（首次落盘、之后复用），返回其路径。
//
// 关键点：osqueryd 是独立进程，必须落到磁盘才能 exec（见手册 §4.6 通信机制）；
// 落盘命名 topus-agentd 以统一品牌（ps/文件系统不暴露 osqueryd）。
func extractEmbedded() (string, bool, error) {
	if len(embeddedOsquerydGz) == 0 {
		return "", false, nil // 容错：assets 为空也不崩
	}
	dir := embedRunDir()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", false, fmt.Errorf("建落盘目录 %s: %w", dir, err)
	}
	dst := filepath.Join(dir, "topus-agentd")
	if fileExists(dst) {
		return dst, true, nil // 已解压，复用（S1 不校验 hash）
	}

	// 1 gunzip 内嵌字节 → 临时文件（同目录，保证 rename 原子）。
	gr, err := gzip.NewReader(bytes.NewReader(embeddedOsquerydGz))
	if err != nil {
		return "", false, fmt.Errorf("gzip reader: %w", err)
	}
	defer gr.Close()
	tmp, err := os.CreateTemp(dir, "topus-agentd-*.tmp")
	if err != nil {
		return "", false, err
	}
	tmpName := tmp.Name()
	if _, err := io.Copy(tmp, gr); err != nil { //nolint:gosec // 源是内嵌可信字节
		tmp.Close()
		os.Remove(tmpName)
		return "", false, fmt.Errorf("解压写盘: %w", err)
	}
	tmp.Close()

	// 2 加执行权限 → 原子 rename 到目标名。顺序不能反：先 chmod 再 rename，
	//   避免别的进程看到一个还不可执行的 topus-agentd。
	if err := os.Chmod(tmpName, 0o755); err != nil {
		os.Remove(tmpName)
		return "", false, err
	}
	if err := os.Rename(tmpName, dst); err != nil {
		os.Remove(tmpName)
		return "", false, err
	}
	return dst, true, nil
}

// embedRunDir 选落盘目录：TOPUS_RUNDIR 优先，否则用户缓存目录，再否则临时目录。
func embedRunDir() string {
	if d := os.Getenv("TOPUS_RUNDIR"); d != "" {
		return d
	}
	if d, err := os.UserCacheDir(); err == nil {
		return filepath.Join(d, "topus")
	}
	return filepath.Join(os.TempDir(), "topus")
}
