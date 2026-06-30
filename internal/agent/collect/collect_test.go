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

// TestCollect 验证组装：host 基础+画像（CPU/硬件/网卡/磁盘/uptime/用户）+ 进程字段映射。
func TestCollect(t *testing.T) {
	q := &mockQuerier{rows: map[string][]map[string]string{
		sqlSystemInfo: {{
			"hostname":           "node-1",
			"uuid":               "ABC-123",
			"cpu_brand":          "Apple M1 Pro",
			"cpu_logical_cores":  "8",
			"cpu_physical_cores": "8",
			"physical_memory":    "17179869184",
			"hardware_vendor":    "Apple Inc.",
			"hardware_model":     "MacBookPro18,3",
			"hardware_serial":    "SN12345",
		}},
		sqlOSVersion:  {{"name": "Ubuntu", "version": "22.04"}},
		sqlInterfaces: {{"name": "en0", "ip": "10.0.0.5", "mac": "aa:bb:cc:dd:ee:ff"}},
		sqlDisks:      {{"name": "/dev/disk0", "model": "APPLE SSD", "size": "1000000"}},
		sqlUptime:     {{"total_seconds": "321735"}},
		sqlUsers:      {{"user": "jingpc"}, {"user": "root"}},
		sqlProcesses: {
			{"pid": "1", "path": "/sbin/init", "cmdline": "/sbin/init", "start_time": "1700000000"},
			{"pid": "42", "path": "/usr/bin/sshd", "cmdline": "sshd -D", "start_time": "1700000100"},
		},
	}}

	snap, err := Collect(q)
	if err != nil {
		t.Fatalf("Collect: %v", err)
	}
	h := snap.Host
	// 基础 + 对账键
	if h.Hostname != "node-1" || h.ProductUUID != "ABC-123" {
		t.Errorf("基础: hostname=%q product_uuid=%q", h.Hostname, h.ProductUUID)
	}
	if h.OSName != "Ubuntu" || h.OSVersion != "22.04" {
		t.Errorf("os: %q %q", h.OSName, h.OSVersion)
	}
	// CPU / 内存
	if h.CPUBrand != "Apple M1 Pro" || h.CPULogicalCores != 8 || h.CPUPhysicalCores != 8 {
		t.Errorf("cpu: brand=%q logical=%d physical=%d", h.CPUBrand, h.CPULogicalCores, h.CPUPhysicalCores)
	}
	if h.MemoryBytes != 17179869184 {
		t.Errorf("mem=%d", h.MemoryBytes)
	}
	// 硬件盘点
	if h.HardwareVendor != "Apple Inc." || h.HardwareModel != "MacBookPro18,3" || h.HardwareSerial != "SN12345" {
		t.Errorf("硬件: vendor=%q model=%q serial=%q", h.HardwareVendor, h.HardwareModel, h.HardwareSerial)
	}
	// 网卡
	if len(h.Interfaces) != 1 || h.Interfaces[0].IP != "10.0.0.5" || h.Interfaces[0].MAC != "aa:bb:cc:dd:ee:ff" {
		t.Errorf("网卡: %+v", h.Interfaces)
	}
	// 磁盘
	if len(h.Disks) != 1 || h.Disks[0].SizeRaw != 1000000 || h.Disks[0].Model != "APPLE SSD" {
		t.Errorf("磁盘: %+v", h.Disks)
	}
	// uptime / 用户
	if h.UptimeSeconds != 321735 {
		t.Errorf("uptime=%d", h.UptimeSeconds)
	}
	if len(h.LoggedInUsers) != 2 {
		t.Errorf("用户: %v", h.LoggedInUsers)
	}
	// 进程
	if len(snap.Processes) != 2 || snap.Processes[1].PID != 42 || snap.Processes[1].ExePath != "/usr/bin/sshd" {
		t.Errorf("进程: %+v", snap.Processes)
	}
	if snap.CollectedAt.IsZero() {
		t.Errorf("CollectedAt 不应为零值")
	}
}
