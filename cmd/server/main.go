// Command topus-server 是 server 端可执行入口（行走骨架第一刀）。
// 当前仅起 gRPC server 并注册 health.Ping，验证 agent↔server 通道。
// 明文 gRPC 仅骨架期使用；单向 TLS / Kratos 接线为紧接增量。
package main

import (
	"flag"
	"log"
	"net"

	healthv1 "github.com/Jingk97/topus-server/api/topus/health/v1"
	"github.com/Jingk97/topus-server/internal/service"
	"google.golang.org/grpc"
)

func main() {
	addr := flag.String("addr", ":9090", "gRPC 监听地址")
	flag.Parse()

	// 1 先占住监听端口；失败（端口被占/权限）直接退出，避免后面空跑。
	lis, err := net.Listen("tcp", *addr)
	if err != nil {
		log.Fatalf("监听 %s 失败: %v", *addr, err)
	}

	// 2 创建 gRPC server 并注册 health 服务。
	//   注意顺序：注册必须在 Serve 之前——Serve 会阻塞并接管监听，
	//   一旦开始服务就不能再改注册表。
	srv := grpc.NewServer()
	healthv1.RegisterHealthServer(srv, service.NewHealthService())

	// 3 阻塞服务，直到进程被终止。
	log.Printf("topus-server 启动，gRPC 监听 %s（明文，骨架期）", *addr)
	if err := srv.Serve(lis); err != nil {
		log.Fatalf("serve: %v", err)
	}
}
