// Package collect 用 osquery 写死 SQL 采 host + 进程，组装成上报快照。
//
// S1 最小集（不加字段，见《agent 功能构成与 osquery 边界》§3）：
// host = system_info + os_version；process = processes(pid/path/cmdline/start_time)。
// product_uuid = system_info.uuid（SMBIOS 硬件 UUID，host 对账匹配键）。
package collect

import (
	"fmt"
	"strconv"
	"time"
)

// Querier 是 osquery 客户端的最小依赖面（*osquery.ExtensionManagerClient 满足）。
// 抽成接口便于用 mock 行做单元测试，不必起真 osqueryd。
type Querier interface {
	QueryRows(sql string) ([]map[string]string, error)
}

// HostInfo 是被纳管主机的基础信息。
type HostInfo struct {
	Hostname    string `json:"hostname"`
	ProductUUID string `json:"product_uuid"` // SMBIOS 硬件 UUID，对账匹配键
	OSName      string `json:"os_name"`
	OSVersion   string `json:"os_version"`
	CPUCores    int    `json:"cpu_logical_cores"`
	MemoryBytes int64  `json:"memory_bytes"`
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

// SQL 常量：写死、最小集，便于审计与稳定性（不动态拼 SQL）。
const (
	sqlSystemInfo = "SELECT hostname, uuid, cpu_logical_cores, physical_memory FROM system_info"
	sqlOSVersion  = "SELECT name, version FROM os_version"
	sqlProcesses  = "SELECT pid, path, cmdline, start_time FROM processes"
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
	// 1 system_info 一行：主机名 / product_uuid / CPU / 内存。
	rows, err := q.QueryRows(sqlSystemInfo)
	if err != nil {
		return h, err
	}
	if len(rows) > 0 {
		r := rows[0]
		h.Hostname = r["hostname"]
		h.ProductUUID = r["uuid"]
		h.CPUCores = atoiSafe(r["cpu_logical_cores"])
		h.MemoryBytes = atoi64Safe(r["physical_memory"])
	}
	// 2 os_version 一行：OS 名 / 版本。
	osRows, err := q.QueryRows(sqlOSVersion)
	if err != nil {
		return h, err
	}
	if len(osRows) > 0 {
		h.OSName = osRows[0]["name"]
		h.OSVersion = osRows[0]["version"]
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
