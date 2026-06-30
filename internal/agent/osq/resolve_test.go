package osq

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

// TestCandidatePaths_Priority 验证无显式指定时的优先级顺序：env → 同目录 → 开发期路径。
func TestCandidatePaths_Priority(t *testing.T) {
	got := candidatePaths("", "/y/env-osqueryd", "/exe", "/cwd", "linux", "amd64")
	want := []string{
		"/y/env-osqueryd",   // 2 环境变量
		"/exe/topus-agentd", // 3 同目录·产品化命名
		"/exe/osqueryd",     // 3 同目录·原名
		"/cwd/deploy/osquery/bin/linux-amd64/osqueryd", // 4 开发期路径
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("优先级顺序不符\n got=%v\nwant=%v", got, want)
	}
}

// TestCandidatePaths_ExplicitWins 显式指定时只认它，不追加其它候选。
func TestCandidatePaths_ExplicitWins(t *testing.T) {
	got := candidatePaths("/x/explicit", "/y/env", "/exe", "/cwd", "linux", "amd64")
	if len(got) != 1 || got[0] != "/x/explicit" {
		t.Fatalf("显式指定应独占，got=%v", got)
	}
}

// TestCandidatePaths_Darwin mac 同目录候选含 .app bundle，开发期路径也走 bundle。
func TestCandidatePaths_Darwin(t *testing.T) {
	got := candidatePaths("", "", "/exe", "/cwd", "darwin", "arm64")
	want := []string{
		"/exe/topus-agentd",
		"/exe/osquery.app/Contents/MacOS/osqueryd",
		"/exe/osqueryd",
		"/cwd/deploy/osquery/bin/darwin-arm64/osquery.app/Contents/MacOS/osqueryd",
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("mac 候选不符\n got=%v\nwant=%v", got, want)
	}
}

// TestResolvePath_EnvHit 环境变量指向的真实文件应被命中。
func TestResolvePath_EnvHit(t *testing.T) {
	dir := t.TempDir()
	fake := filepath.Join(dir, "osqueryd")
	if err := os.WriteFile(fake, []byte("#!/bin/true\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("TOPUS_OSQUERYD", fake)
	got, err := ResolvePath("")
	if err != nil || got != fake {
		t.Fatalf("期望命中 env 文件 %s，got=%q err=%v", fake, got, err)
	}
}

// TestResolvePath_ExplicitMissing 显式指定但文件不存在 → 报错（不静默回退）。
func TestResolvePath_ExplicitMissing(t *testing.T) {
	if _, err := ResolvePath("/definitely/not/here/osqueryd"); err == nil {
		t.Fatal("显式指定不存在的路径应报错")
	}
}
