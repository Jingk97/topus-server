//go:build embedosq

package osq

import (
	"bytes"
	"compress/gzip"
	"crypto/sha256"
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

// extractEmbedded 把内嵌的 osqueryd 解压落盘成 topus-agentd，返回其路径。
//
// 原则：**不静默**——空资产 / 落盘目录不可用 / 解压失败 一律报错，绝不悄悄回退到外部 osqueryd。
// 版本控制：按内嵌字节的 sha256 复用——落盘的 topus-agentd 与当前内嵌版本一致才复用，
// 不一致（agent 升级了）则重新解压覆盖，避免跑上一版 osqueryd。
func extractEmbedded() (string, bool, error) {
	// 1 embed 构建却没有内嵌字节 = 构建异常，硬报错（不降级）。
	if len(embeddedOsquerydGz) == 0 {
		return "", false, fmt.Errorf("embed 构建但内嵌 osqueryd 为空（构建脚本异常）")
	}

	// 2 落盘目录：TOPUS_RUNDIR > 用户缓存目录；都没有则报错（不兜底 /tmp，避免多用户目录被预置恶意文件）。
	dir, err := embedRunDir()
	if err != nil {
		return "", false, err
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", false, fmt.Errorf("创建落盘目录 %s 失败: %w", dir, err)
	}

	dst := filepath.Join(dir, "topus-agentd")
	sumFile := dst + ".sha256"
	want := fmt.Sprintf("%x", sha256.Sum256(embeddedOsquerydGz))

	// 3 版本一致才复用（hash 用压缩字节，便宜，不必解压算）。
	if fileExists(dst) {
		if got, _ := os.ReadFile(sumFile); string(got) == want {
			return dst, true, nil
		}
		// 版本不一致：落盘的是旧版 osqueryd，下面重新解压覆盖。
	}

	// 4 gunzip 内嵌字节 → 同目录临时文件（保证 rename 原子）。
	gr, err := gzip.NewReader(bytes.NewReader(embeddedOsquerydGz))
	if err != nil {
		return "", false, fmt.Errorf("内嵌 osqueryd 解压失败（资产损坏）: %w", err)
	}
	defer gr.Close()
	tmp, err := os.CreateTemp(dir, "topus-agentd-*.tmp")
	if err != nil {
		return "", false, fmt.Errorf("创建临时文件失败: %w", err)
	}
	tmpName := tmp.Name()
	if _, err := io.Copy(tmp, gr); err != nil { //nolint:gosec // 源是内嵌可信字节
		tmp.Close()
		os.Remove(tmpName)
		return "", false, fmt.Errorf("解压写盘失败: %w", err)
	}
	tmp.Close()

	// 5 先 chmod 再原子 rename（顺序不能反：避免别的进程看到尚不可执行的 topus-agentd）。
	if err := os.Chmod(tmpName, 0o755); err != nil {
		os.Remove(tmpName)
		return "", false, err
	}
	if err := os.Rename(tmpName, dst); err != nil {
		os.Remove(tmpName)
		return "", false, err
	}
	// 6 落版本戳（复用时比对）。失败不致命，仅导致下次必重解压。
	_ = os.WriteFile(sumFile, []byte(want), 0o600)
	return dst, true, nil
}

// embedRunDir 选落盘目录：TOPUS_RUNDIR 优先，否则用户缓存目录。
// **不兜底 /tmp**——多用户可写目录可能被预置恶意 topus-agentd（TOCTOU）；取不到就报错。
func embedRunDir() (string, error) {
	if d := os.Getenv("TOPUS_RUNDIR"); d != "" {
		return d, nil
	}
	if d, err := os.UserCacheDir(); err == nil {
		return filepath.Join(d, "topus"), nil
	}
	return "", fmt.Errorf("无法确定落盘目录：设 TOPUS_RUNDIR 指定一个 agent 专属可写目录")
}
