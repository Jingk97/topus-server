package osq

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
)

// osqueryd 查找方式（一处定义，覆盖开发 / 统一包 / embed / 显式四种来源）。
//
// 优先级从高到低：
//  1. explicit  —— `--osqueryd` 显式指定（特殊情况留口子）
//  2. env       —— `TOPUS_OSQUERYD` 环境变量
//  3. 同目录     —— agent 可执行文件旁边（统一包：`topus-agentd`/`osqueryd`/mac bundle）
//  4. embed     —— **agent 内嵌的 osqueryd 解压落盘**（产品化单文件，见 embed_osqueryd.go）
//  5. 开发期路径  —— `deploy/osquery/bin/<os-arch>/`（相对当前目录，fetch.sh 拉的位置）
//  6. 系统 PATH  —— 兜底找 `osqueryd`
//
// embed 落盘命名 `topus-agentd`；普通构建不内嵌（extractEmbedded 返回 false），
// 走 fetch / 同目录。只有 `-tags embedosq` 的发布构建才内嵌。

// ResolvePath 按上述优先级定位 osqueryd，返回可用路径或错误。
func ResolvePath(explicit string) (string, error) {
	exeDir := ""
	if exe, err := os.Executable(); err == nil {
		exeDir = filepath.Dir(exe)
	}
	cwd, _ := os.Getwd()
	return resolveWith(explicit, os.Getenv("TOPUS_OSQUERYD"), exeDir, cwd,
		runtime.GOOS, runtime.GOARCH, extractEmbedded, fileExists, exec.LookPath)
}

// resolveWith 是 ResolvePath 的纯逻辑内核，依赖以参数注入便于测试
// （extract = embed 解压；exists = 文件存在判断；lookPath = PATH 查找）。
func resolveWith(
	explicit, env, exeDir, cwd, goos, goarch string,
	extract func() (string, bool, error),
	exists func(string) bool,
	lookPath func(string) (string, error),
) (string, error) {
	// 1 显式指定独占：命中即用，不存在则报错（不静默回退到别的 osqueryd）。
	if explicit != "" {
		if exists(explicit) {
			return explicit, nil
		}
		return "", fmt.Errorf("--osqueryd 指定的 %s 不存在", explicit)
	}

	var tried []string
	firstExisting := func(ps []string) (string, bool) {
		for _, p := range ps {
			if p == "" {
				continue
			}
			tried = append(tried, p)
			if exists(p) {
				return p, true
			}
		}
		return "", false
	}

	// 2 环境变量
	if p, ok := firstExisting([]string{env}); ok {
		return p, nil
	}
	// 3 agent 同目录（统一包）
	if exeDir != "" {
		if p, ok := firstExisting(siblingCandidates(exeDir, goos)); ok {
			return p, nil
		}
	}
	// 4 embed 自带（解压落盘；普通构建直接 false）
	if p, ok, err := extract(); err != nil {
		return "", fmt.Errorf("解压内嵌 osqueryd: %w", err)
	} else if ok {
		return p, nil
	}
	// 5 开发期项目路径
	if p, ok := firstExisting(devCandidates(cwd, goos, goarch)); ok {
		return p, nil
	}
	// 6 系统 PATH 兜底
	if p, err := lookPath("osqueryd"); err == nil {
		return p, nil
	}
	return "", fmt.Errorf("找不到 osqueryd（试过 %v）；跑 deploy/osquery/fetch.sh 拉取，或用 --osqueryd 指定", tried)
}

// siblingCandidates 返回 agent 可执行文件同目录下的候选（产品化命名优先）。
func siblingCandidates(dir, goos string) []string {
	if goos == "darwin" {
		return []string{
			filepath.Join(dir, "topus-agentd"),
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
