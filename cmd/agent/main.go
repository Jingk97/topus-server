// Command topus-agent 是 agent 端可执行入口。子命令：
//   test    链路预检：连 server 调 health.Ping（可选单向 TLS）
//   collect 本地采集：拉起 osqueryd 采 host + 进程，输出快照（不依赖 server）
package main

import (
	"fmt"
	"os"
)

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(2)
	}
	switch os.Args[1] {
	case "test":
		runTest(os.Args[2:])
	case "collect":
		runCollect(os.Args[2:])
	default:
		usage()
		os.Exit(2)
	}
}

func usage() {
	fmt.Fprintln(os.Stderr, "用法: topus-agent <子命令> [选项]")
	fmt.Fprintln(os.Stderr, "  test    --server=<addr> [--ca=<ca.pem>]   链路预检")
	fmt.Fprintln(os.Stderr, "  collect [--osqueryd=<path>] [--json]      本地采集 host+进程")
}
