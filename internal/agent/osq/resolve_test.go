package osq

import (
	"fmt"
	"reflect"
	"testing"
)

func TestSiblingCandidates(t *testing.T) {
	if got := siblingCandidates("/d", "linux"); !reflect.DeepEqual(got, []string{"/d/topus-agentd", "/d/osqueryd"}) {
		t.Fatalf("linux sibling: %v", got)
	}
	want := []string{"/d/topus-agentd", "/d/osquery.app/Contents/MacOS/osqueryd", "/d/osqueryd"}
	if got := siblingCandidates("/d", "darwin"); !reflect.DeepEqual(got, want) {
		t.Fatalf("darwin sibling: %v", got)
	}
}

func TestDevCandidates(t *testing.T) {
	if got := devCandidates("/c", "linux", "amd64"); !reflect.DeepEqual(got, []string{"/c/deploy/osquery/bin/linux-amd64/osqueryd"}) {
		t.Fatalf("linux dev: %v", got)
	}
	want := []string{"/c/deploy/osquery/bin/darwin-arm64/osquery.app/Contents/MacOS/osqueryd"}
	if got := devCandidates("/c", "darwin", "arm64"); !reflect.DeepEqual(got, want) {
		t.Fatalf("darwin dev: %v", got)
	}
}

// existsSet 构造一个"哪些路径算存在"的判断函数。
func existsSet(paths ...string) func(string) bool {
	m := map[string]bool{}
	for _, p := range paths {
		m[p] = true
	}
	return func(p string) bool { return m[p] }
}

// TestResolveWith_Priority 验证六级查找优先级（含 embed）。
func TestResolveWith_Priority(t *testing.T) {
	noExtract := func() (string, bool, error) { return "", false, nil }
	yesExtract := func() (string, bool, error) { return "/embed/topus-agentd", true, nil }
	noLook := func(string) (string, error) { return "", fmt.Errorf("not in PATH") }
	yesLook := func(string) (string, error) { return "/usr/bin/osqueryd", nil }

	// 1 显式指定命中
	if p, err := resolveWith("/x", "", "/e", "/c", "linux", "amd64", noExtract, existsSet("/x"), noLook); err != nil || p != "/x" {
		t.Fatalf("explicit: %q %v", p, err)
	}
	// 1' 显式指定但不存在 → 报错
	if _, err := resolveWith("/x", "", "/e", "/c", "linux", "amd64", noExtract, existsSet(), noLook); err == nil {
		t.Fatal("explicit 缺失应报错")
	}
	// 2 环境变量
	if p, _ := resolveWith("", "/env", "/e", "/c", "linux", "amd64", noExtract, existsSet("/env"), noLook); p != "/env" {
		t.Fatalf("env: %q", p)
	}
	// 3 同目录优先于 embed（sibling 存在时即便有 embed 也用 sibling）
	if p, _ := resolveWith("", "", "/e", "/c", "linux", "amd64", yesExtract, existsSet("/e/osqueryd"), noLook); p != "/e/osqueryd" {
		t.Fatalf("sibling 应优先于 embed: %q", p)
	}
	// 4 embed 命中（无 sibling）
	if p, _ := resolveWith("", "", "/e", "/c", "linux", "amd64", yesExtract, existsSet(), noLook); p != "/embed/topus-agentd" {
		t.Fatalf("embed: %q", p)
	}
	// 5 开发期路径（无 embed）
	dev := "/c/deploy/osquery/bin/linux-amd64/osqueryd"
	if p, _ := resolveWith("", "", "/e", "/c", "linux", "amd64", noExtract, existsSet(dev), noLook); p != dev {
		t.Fatalf("dev: %q", p)
	}
	// 6 PATH 兜底
	if p, _ := resolveWith("", "", "/e", "/c", "linux", "amd64", noExtract, existsSet(), yesLook); p != "/usr/bin/osqueryd" {
		t.Fatalf("PATH: %q", p)
	}
}
