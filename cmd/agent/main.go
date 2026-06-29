// Command topus-agent 是 agent 端可执行入口。
// `topus-agent test --server=<addr> [--ca=<ca.pem>]` 链路预检：连 server 调 health.Ping。
// 给 --ca 则单向 TLS（用 CA 验 server 身份防中间人），否则明文（骨架期）。
// 通则退出码 0、链路不通 1、用法错 2（方案《纳管批次与接入安全》§3.8）。
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"time"

	healthv1 "github.com/Jingk97/topus-server/api/topus/health/v1"
	"github.com/Jingk97/topus-server/internal/tlsutil"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/insecure"
)

func main() {
	// 1 仅接受 `test` 子命令；其余用法错退 2（区别于"链路不通"的退 1）。
	if len(os.Args) < 2 || os.Args[1] != "test" {
		fmt.Fprintln(os.Stderr, "用法: topus-agent test --server=<addr> [--ca=<ca.pem>]")
		os.Exit(2)
	}
	fs := flag.NewFlagSet("test", flag.ExitOnError)
	server := fs.String("server", "127.0.0.1:9090", "server gRPC 地址")
	ca := fs.String("ca", "", "CA 证书 PEM 路径（空=明文；给则单向 TLS 验 server）")
	_ = fs.Parse(os.Args[2:])

	// 2 按是否给 --ca 选择传输凭证：给则用 CA 验 server（单向 TLS），否则明文。
	var creds credentials.TransportCredentials
	if *ca != "" {
		caPEM, err := os.ReadFile(*ca)
		if err != nil {
			fmt.Fprintf(os.Stderr, "读 CA 失败: %v\n", err)
			os.Exit(1)
		}
		cliTLS, err := tlsutil.ClientTLSConfig(caPEM)
		if err != nil {
			fmt.Fprintf(os.Stderr, "构建 client TLS 配置失败: %v\n", err)
			os.Exit(1)
		}
		creds = credentials.NewTLS(cliTLS)
	} else {
		creds = insecure.NewCredentials()
	}

	// 3 给整个预检设超时上限，避免网络黑洞导致 agent 卡死。
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// 4 建立连接（grpc.NewClient 惰性连接，真正拨号发生在首个 RPC）。
	conn, err := grpc.NewClient(*server, grpc.WithTransportCredentials(creds))
	if err != nil {
		fmt.Fprintf(os.Stderr, "连接失败: %v\n", err)
		os.Exit(1)
	}
	defer func() { _ = conn.Close() }()

	// 5 调 Ping。失败 = 链路不通/TLS 验证失败/server 异常 → 退 1。
	reply, err := healthv1.NewHealthClient(conn).Ping(ctx, &healthv1.PingRequest{})
	if err != nil {
		fmt.Fprintf(os.Stderr, "Ping 失败（链路不通）: %v\n", err)
		os.Exit(1)
	}
	if !reply.GetOk() {
		fmt.Fprintln(os.Stderr, "Ping 返回 not ok")
		os.Exit(1)
	}

	// 6 预检通过：打印 ok + server 版本/时间，退 0。
	fmt.Printf("ok server=%s time=%d\n", reply.GetServerVersion(), reply.GetServerTimeUnix())
}
