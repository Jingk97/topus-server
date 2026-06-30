package osq

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
)

// osqueryd 查找方式（一处定义，覆盖开发 / 统一包 / embed / 显式四种场景）。
//
// 优先级从高到低：
//  1. explicit  —— `--osqueryd` 显式指定（特殊情况留口子）
//  2. env       —— `TOPUS_OSQUERYD` 环境变量
//  3. 同目录     —— agent 可执行文件旁边（产品化"统一包/embed"的关键：
//     落盘的 `topus-agentd`，或 `osqueryd`，mac 则 `osquery.app/.../osqueryd`）
//  4. 开发期路径  —— `deploy/osquery/bin/<os-arch>/`（相对当前目录，fetch.sh 拉的位置）
//  5. 系统 PATH  —— 兜底找 `osqueryd`
//
// 产品化 embed 后，osqueryd 落盘命名 `topus-agentd`、放在 agent 同目录 → 命中第 3 级，
// 无需 `--osqueryd`、无需项目目录结构。

// ResolvePath 按上述优先级定位 osqueryd，返回可用路径或错误。
func ResolvePath(explicit string) (string, error) {
	exeDir := ""
	if exe, err := os.Executable(); err == nil {
		exeDir = filepath.Dir(exe)
	}
	cwd, _ := os.Getwd()

	cands := candidatePaths(explicit, os.Getenv("TOPUS_OSQUERYD"), exeDir, cwd, runtime.GOOS, runtime.GOARCH)
	for _, p := range cands {
		if fileExists(p) {
			return p, nil
		}
	}
	// 5 系统 PATH 兜底（显式指定时不走兜底，避免静默命中别的 osqueryd）。
	if explicit == "" {
		if p, err := exec.LookPath("osqueryd"); err == nil {
			return p, nil
		}
	}
	return "", fmt.Errorf("找不到 osqueryd（试过 %v）；跑 deploy/osquery/fetch.sh 拉取，或用 --osqueryd 指定", cands)
}

// candidatePaths 按优先级返回候选路径（纯函数，注入参数便于测试）。
func candidatePaths(explicit, env, exeDir, cwd, goos, goarch string) []string {
	// 1 显式指定：只认它，不再追加其它候选。
	if explicit != "" {
		return []string{explicit}
	}
	var c []string
	// 2 环境变量
	if env != "" {
		c = append(c, env)
	}
	// 3 agent 同目录（产品化/统一包/embed）
	if exeDir != "" {
		c = append(c, siblingCandidates(exeDir, goos)...)
	}
	// 4 开发期项目路径（相对 cwd）
	c = append(c, devCandidates(cwd, goos, goarch)...)
	return c
}

// siblingCandidates 返回 agent 可执行文件同目录下的候选（产品化命名优先）。
func siblingCandidates(dir, goos string) []string {
	if goos == "darwin" {
		return []string{
			filepath.Join(dir, "topus-agentd"), // 产品化命名（mac 一般不 embed，留作兜底）
			filepath.Join(dir, "osquery.app", "Contents", "MacOS", "osqueryd"),
			filepath.Join(dir, "osqueryd"),
		}
	}
	return []string{
		filepath.Join(dir, "topus-agentd"), // 产品化 embed 落盘命名
		filepath.Join(dir, "osqueryd"),
	}
}

// devCandidates 返回开发期项目路径（fetch.sh 拉到的位置，相对 cwd）。
func devCandidates(cwd, goos, goarch string) []string {
	base := filepath.Join(cwd, "deploy", "osquery", "bin", goos+"-"+goarch)
	if goos == "darwin" {
		// mac 是完整 .app bundle
		return []string{filepath.Join(base, "osquery.app", "Contents", "MacOS", "osqueryd")}
	}
	return []string{filepath.Join(base, "osqueryd")}
}

// fileExists 判断路径存在且为普通文件（可执行性由后续 exec 实际校验）。
func fileExists(p string) bool {
	if p == "" {
		return false
	}
	fi, err := os.Stat(p)
	return err == nil && !fi.IsDir()
}
