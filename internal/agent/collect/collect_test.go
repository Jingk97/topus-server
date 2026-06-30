package collect

import "testing"

// mockQuerier 按 SQL 返回预置行，模拟 osquery 而不起真 osqueryd。
type mockQuerier struct {
	rows map[string][]map[string]string
	err  error
}

func (m *mockQuerier) QueryRows(sql string) ([]map[string]string, error) {
	if m.err != nil {
		return nil, m.err
	}
	return m.rows[sql], nil
}

// TestCollect 验证组装：host 字段映射（含 product_uuid=uuid）+ 进程字段映射 + 字符串转数值。
func TestCollect(t *testing.T) {
	q := &mockQuerier{rows: map[string][]map[string]string{
		sqlSystemInfo: {{
			"hostname":          "node-1",
			"uuid":              "ABC-123",
			"cpu_logical_cores": "8",
			"physical_memory":   "17179869184",
		}},
		sqlOSVersion: {{"name": "Ubuntu", "version": "22.04"}},
		sqlProcesses: {
			{"pid": "1", "path": "/sbin/init", "cmdline": "/sbin/init", "start_time": "1700000000"},
			{"pid": "42", "path": "/usr/bin/sshd", "cmdline": "sshd -D", "start_time": "1700000100"},
		},
	}}

	snap, err := Collect(q)
	if err != nil {
		t.Fatalf("Collect: %v", err)
	}
	// host 断言
	if snap.Host.Hostname != "node-1" {
		t.Errorf("hostname = %q, 期望 node-1", snap.Host.Hostname)
	}
	if snap.Host.ProductUUID != "ABC-123" {
		t.Errorf("product_uuid = %q, 期望 ABC-123（= system_info.uuid）", snap.Host.ProductUUID)
	}
	if snap.Host.CPUCores != 8 {
		t.Errorf("cpu = %d, 期望 8", snap.Host.CPUCores)
	}
	if snap.Host.MemoryBytes != 17179869184 {
		t.Errorf("mem = %d, 期望 17179869184", snap.Host.MemoryBytes)
	}
	if snap.Host.OSName != "Ubuntu" || snap.Host.OSVersion != "22.04" {
		t.Errorf("os = %q %q, 期望 Ubuntu 22.04", snap.Host.OSName, snap.Host.OSVersion)
	}
	// 进程断言
	if len(snap.Processes) != 2 {
		t.Fatalf("进程数 = %d, 期望 2", len(snap.Processes))
	}
	if snap.Processes[1].PID != 42 || snap.Processes[1].ExePath != "/usr/bin/sshd" {
		t.Errorf("进程[1] = %+v, 期望 pid=42 path=/usr/bin/sshd", snap.Processes[1])
	}
	if snap.CollectedAt.IsZero() {
		t.Errorf("CollectedAt 不应为零值")
	}
}
