// Package collect 用 osquery 写死 SQL 采 host + 进程，组装成上报快照。
//
// host 画像（丰富档，见数据模型 §2.2 / ADR-0010 对账属性 / 业界 CMDB 对标）：
// 基础(system_info+os_version) + 网卡(IP/MAC) + 磁盘 + 开机时长 + 登录用户。
// product_uuid = system_info.uuid（SMBIOS 硬件 UUID，host 对账匹配键）。
// process = processes(pid/path/cmdline/start_time) 最小集（不加字段）。
package collect

import (
	"fmt"
	"regexp"
	"strconv"
	"time"
)

// physicalDiskRe 匹配物理盘设备名，过滤掉分区/合成盘噪音：
// mac diskN（剔除 diskNsM 分区）、linux sd[a-z]/nvmeNnN/vd[a-z]/xvd[a-z]/mmcblkN（剔除其分区）。
var physicalDiskRe = regexp.MustCompile(`^/dev/(disk\d+|sd[a-z]+|nvme\d+n\d+|vd[a-z]+|xvd[a-z]+|mmcblk\d+)$`)

// Querier 是 osquery 客户端的最小依赖面（*osquery.ExtensionManagerClient 满足）。
// 抽成接口便于用 mock 行做单元测试，不必起真 osqueryd。
type Querier interface {
	QueryRows(sql string) ([]map[string]string, error)
}

// NetIface 是一张承载 IP 的网卡（name/ip/mac）。
type NetIface struct {
	Name string `json:"name"`
	IP   string `json:"ip"`
	MAC  string `json:"mac"`
}

// Disk 是一个块设备。SizeRaw 是 osquery 原始值——单位跨平台未归一
// （mac 是 APFS 合成盘、linux 是扇区），物理盘归一/总容量计算后续做。
type Disk struct {
	Name    string `json:"name"`
	Model   string `json:"model"`
	SizeRaw int64  `json:"size_raw"`
}

// HostInfo 是被纳管主机的画像（丰富档）。
type HostInfo struct {
	Hostname    string `json:"hostname"`
	ProductUUID string `json:"product_uuid"` // SMBIOS 硬件 UUID，对账匹配键
	OSName      string `json:"os_name"`
	OSVersion   string `json:"os_version"`
	// CPU / 内存
	CPUBrand         string `json:"cpu_brand"`
	CPULogicalCores  int    `json:"cpu_logical_cores"`
	CPUPhysicalCores int    `json:"cpu_physical_cores"`
	MemoryBytes      int64  `json:"memory_bytes"`
	// 硬件盘点（对账辅助 + 资产）
	HardwareVendor string `json:"hardware_vendor"`
	HardwareModel  string `json:"hardware_model"`
	HardwareSerial string `json:"hardware_serial"`
	// 网络 / 磁盘 / 运行态
	Interfaces    []NetIface `json:"interfaces"`      // 承载 IP 的网卡
	Disks         []Disk     `json:"disks"`           // 块设备列表
	UptimeSeconds int64      `json:"uptime_seconds"`  // 开机时长
	LoggedInUsers []string   `json:"logged_in_users"` // 活跃登录用户（去重）
}

// Process 是单个进程的最小信息。
type Process struct {
	PID       int64  `json:"pid"`
	ExePath   string `json:"exe_path"`
	Cmdline   string `json:"cmdline"`
	StartTime int64  `json:"start_time"`
}

// Snapshot 是一次采集的完整快照（上报单元）。
type Snapshot struct {
	Host        HostInfo  `json:"host"`
	Processes   []Process `json:"processes"`
	CollectedAt time.Time `json:"collected_at"`
}

// SQL 常量：写死、便于审计与稳定（不动态拼 SQL）。
const (
	sqlSystemInfo = "SELECT hostname, uuid, cpu_brand, cpu_logical_cores, cpu_physical_cores, " +
		"physical_memory, hardware_vendor, hardware_model, hardware_serial FROM system_info"
	sqlOSVersion = "SELECT name, version FROM os_version"
	// 网卡：只取承载 IP 的接口（join 自然过滤掉无 IP 的虚拟接口），并去掉环回/链路本地。
	sqlInterfaces = "SELECT ia.interface AS name, ia.address AS ip, id.mac AS mac " +
		"FROM interface_addresses ia JOIN interface_details id ON ia.interface = id.interface " +
		"WHERE ia.address NOT LIKE 'fe80%' AND ia.address NOT LIKE '127.%' AND ia.address != '::1' " +
		"AND id.mac != '00:00:00:00:00:00'"
	sqlDisks     = "SELECT name, model, size FROM block_devices"
	sqlUptime    = "SELECT total_seconds FROM uptime"
	sqlUsers     = "SELECT DISTINCT user FROM logged_in_users WHERE type = 'user'"
	sqlProcesses = "SELECT pid, path, cmdline, start_time FROM processes"
)

// Collect 采一次 host + 进程，组装快照。
func Collect(q Querier) (*Snapshot, error) {
	host, err := collectHost(q)
	if err != nil {
		return nil, fmt.Errorf("采 host：%w", err)
	}
	procs, err := collectProcesses(q)
	if err != nil {
		return nil, fmt.Errorf("采进程：%w", err)
	}
	return &Snapshot{Host: host, Processes: procs, CollectedAt: time.Now().UTC()}, nil
}

func collectHost(q Querier) (HostInfo, error) {
	var h HostInfo
	// 1 system_info 一行：基础 + CPU + 硬件盘点。
	rows, err := q.QueryRows(sqlSystemInfo)
	if err != nil {
		return h, err
	}
	if len(rows) > 0 {
		r := rows[0]
		h.Hostname = r["hostname"]
		h.ProductUUID = r["uuid"]
		h.CPUBrand = r["cpu_brand"]
		h.CPULogicalCores = atoiSafe(r["cpu_logical_cores"])
		h.CPUPhysicalCores = atoiSafe(r["cpu_physical_cores"])
		h.MemoryBytes = atoi64Safe(r["physical_memory"])
		h.HardwareVendor = r["hardware_vendor"]
		h.HardwareModel = r["hardware_model"]
		h.HardwareSerial = r["hardware_serial"]
	}
	// 2 os_version 一行。
	if osRows, err := q.QueryRows(sqlOSVersion); err != nil {
		return h, err
	} else if len(osRows) > 0 {
		h.OSName = osRows[0]["name"]
		h.OSVersion = osRows[0]["version"]
	}
	// 3 网卡（承载 IP 的接口）。
	ifRows, err := q.QueryRows(sqlInterfaces)
	if err != nil {
		return h, err
	}
	for _, r := range ifRows {
		h.Interfaces = append(h.Interfaces, NetIface{Name: r["name"], IP: r["ip"], MAC: r["mac"]})
	}
	// 4 磁盘。
	diskRows, err := q.QueryRows(sqlDisks)
	if err != nil {
		return h, err
	}
	for _, r := range diskRows {
		// 只留物理盘，过滤分区/合成盘噪音（见 physicalDiskRe）。
		if !physicalDiskRe.MatchString(r["name"]) {
			continue
		}
		h.Disks = append(h.Disks, Disk{Name: r["name"], Model: r["model"], SizeRaw: atoi64Safe(r["size"])})
	}
	// 5 开机时长。
	if upRows, err := q.QueryRows(sqlUptime); err != nil {
		return h, err
	} else if len(upRows) > 0 {
		h.UptimeSeconds = atoi64Safe(upRows[0]["total_seconds"])
	}
	// 6 活跃登录用户（去重）。
	userRows, err := q.QueryRows(sqlUsers)
	if err != nil {
		return h, err
	}
	for _, r := range userRows {
		if u := r["user"]; u != "" {
			h.LoggedInUsers = append(h.LoggedInUsers, u)
		}
	}
	return h, nil
}

func collectProcesses(q Querier) ([]Process, error) {
	rows, err := q.QueryRows(sqlProcesses)
	if err != nil {
		return nil, err
	}
	procs := make([]Process, 0, len(rows))
	for _, r := range rows {
		procs = append(procs, Process{
			PID:       atoi64Safe(r["pid"]),
			ExePath:   r["path"],
			Cmdline:   r["cmdline"],
			StartTime: atoi64Safe(r["start_time"]),
		})
	}
	return procs, nil
}

// atoiSafe / atoi64Safe：osquery 返回值都是字符串，空/非法转 0（采集不因脏值中断）。
func atoiSafe(s string) int {
	n, _ := strconv.Atoi(s)
	return n
}

func atoi64Safe(s string) int64 {
	n, _ := strconv.ParseInt(s, 10, 64)
	return n
}
