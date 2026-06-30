package main

import (
	"context"
	"encoding/json"
	"flag"
	"log/slog"
	"os"
	"time"

	"github.com/Jingk97/topus-server/internal/agent/collect"
	"github.com/Jingk97/topus-server/internal/agent/osq"
)

// runCollect 实现 `topus-agent collect`：拉起 osqueryd 采一次 host + 进程，
// 用结构化日志记录"采到了什么"，并把完整快照以 JSON 输出（不依赖 server）。
func runCollect(args []string) {
	fs := flag.NewFlagSet("collect", flag.ExitOnError)
	osqd := fs.String("osqueryd", "", "osqueryd 路径（空=自动查找：同目录/项目/PATH，见 osq.ResolvePath）")
	asJSON := fs.Bool("json", true, "输出快照 JSON 到 stdout")
	_ = fs.Parse(args)

	log := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelInfo}))

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// 1 定位 osqueryd（自动查找；--osqueryd 可显式覆盖）。
	osqdPath, err := osq.ResolvePath(*osqd)
	if err != nil {
		log.Error("定位 osqueryd 失败", "err", err)
		os.Exit(1)
	}
	log.Info("拉起 osqueryd", "path", osqdPath)
	d, err := osq.Start(ctx, osqdPath)
	if err != nil {
		log.Error("osqueryd 启动失败", "err", err)
		os.Exit(1)
	}
	defer d.Stop()
	log.Info("osqueryd 就绪，extension socket 已连通")

	client, err := d.Client()
	if err != nil {
		log.Error("连 osqueryd 失败", "err", err)
		os.Exit(1)
	}
	defer client.Close()

	// 2 采一次 host + 进程。
	t0 := time.Now()
	snap, err := collect.Collect(client)
	if err != nil {
		log.Error("采集失败", "err", err)
		os.Exit(1)
	}

	// 3 日志记录采到了什么（可见性：你能直接看到采集内容概览）。
	log.Info("采集完成",
		"host", snap.Host.Hostname,
		"product_uuid", snap.Host.ProductUUID,
		"os", snap.Host.OSName+" "+snap.Host.OSVersion,
		"cpu", snap.Host.CPUBrand,
		"cpu_cores", snap.Host.CPULogicalCores,
		"mem_bytes", snap.Host.MemoryBytes,
		"serial", snap.Host.HardwareSerial,
		"interfaces", len(snap.Host.Interfaces),
		"disks", len(snap.Host.Disks),
		"uptime_s", snap.Host.UptimeSeconds,
		"users", len(snap.Host.LoggedInUsers),
		"process_count", len(snap.Processes),
		"elapsed", time.Since(t0).String(),
	)

	// 4 输出完整快照 JSON（看到采了哪些字段/内容）。
	if *asJSON {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		if err := enc.Encode(snap); err != nil {
			log.Error("输出 JSON 失败", "err", err)
			os.Exit(1)
		}
	}
}
