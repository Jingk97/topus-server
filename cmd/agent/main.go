// Command topus-agent 是 agent 端可执行入口（行走骨架第一刀）。
// 当前仅实现 `topus-agent test --server=<addr>` 链路预检子命令：
// 连 server 调 health.Ping，通则退出码 0、不通则非 0，供批量纳管前样本机预检
// （方案《纳管批次与接入安全》§3.8）。
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"time"

	healthv1 "github.com/Jingk97/topus-server/api/topus/health/v1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

func main() {
	// 1 仅接受 `test` 子命令；其余用法错误退 2（区别于"链路不通"的退 1）。
	if len(os.Args) < 2 || os.Args[1] != "test" {
		fmt.Fprintln(os.Stderr, "用法: topus-agent test --server=<addr>")
		os.Exit(2)
	}

	fs := flag.NewFlagSet("test", flag.ExitOnError)
	server := fs.String("server", "127.0.0.1:9090", "server gRPC 地址")
	_ = fs.Parse(os.Args[2:])

	// 2 给整个预检设超时上限，避免网络黑洞导致 agent 卡死。
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// 3 建立连接（grpc.NewClient 惰性连接，真正拨号发生在首个 RPC）。
	//   明文凭证仅骨架期；单向 TLS 为紧接增量。
	conn, err := grpc.NewClient(*server, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		fmt.Fprintf(os.Stderr, "连接失败: %v\n", err)
		os.Exit(1)
	}
	defer func() { _ = conn.Close() }()

	// 4 调 Ping。失败 = 链路不通/server 异常 → 退 1（明确失败信号）。
	reply, err := healthv1.NewHealthClient(conn).Ping(ctx, &healthv1.PingRequest{})
	if err != nil {
		fmt.Fprintf(os.Stderr, "Ping 失败（链路不通）: %v\n", err)
		os.Exit(1)
	}
	if !reply.GetOk() {
		fmt.Fprintln(os.Stderr, "Ping 返回 not ok")
		os.Exit(1)
	}

	// 5 预检通过：打印 ok + server 版本/时间，退 0。
	fmt.Printf("ok server=%s time=%d\n", reply.GetServerVersion(), reply.GetServerTimeUnix())
}
