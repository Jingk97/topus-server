package service

import (
	"context"
	"net"
	"testing"

	healthv1 "github.com/Jingk97/topus-server/api/topus/health/v1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/test/bufconn"
)

// TestHealthPing 是 C1 的失败测试（存档点）。
//
// 验收映射：方案《纳管批次与接入安全》§3.8 —— 测试命令在能连通的链路返回 ok。
// 这里用 bufconn（内存管道）起 gRPC server，不占真实端口，纯验"接口契约 + 服务逻辑"：
// 调 Ping → 期望 ok=true 且 server_version 非空。
//
// 当前 health.go 的 Ping 返回 Unimplemented → 本测试应红（红的原因 = 逻辑未实现）。
func TestHealthPing(t *testing.T) {
	// 1 内存管道起 server（不占真实端口）
	lis := bufconn.Listen(1024 * 1024)
	srv := grpc.NewServer()
	healthv1.RegisterHealthServer(srv, NewHealthService())
	go func() { _ = srv.Serve(lis) }()
	t.Cleanup(srv.Stop)

	// 2 客户端经 bufconn 拨号
	dialer := func(context.Context, string) (net.Conn, error) { return lis.Dial() }
	conn, err := grpc.NewClient("passthrough:///bufnet",
		grpc.WithContextDialer(dialer),
		grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		t.Fatalf("拨号失败: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })

	// 3 调 Ping，断言链路通 + ok
	reply, err := healthv1.NewHealthClient(conn).Ping(context.Background(), &healthv1.PingRequest{})
	if err != nil {
		t.Fatalf("Ping RPC 失败: %v", err)
	}
	if !reply.GetOk() {
		t.Fatalf("期望 ok=true, 实际 %+v", reply)
	}
	if reply.GetServerVersion() == "" {
		t.Errorf("期望 server_version 非空")
	}
}
