package server

import (
	"context"
	"testing"
	"time"

	healthv1 "github.com/Jingk97/topus-server/api/topus/health/v1"
	"github.com/Jingk97/topus-server/internal/tlsutil"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
)

// dialAndPing 用给定 TLS 配置连 addr 调 Ping，返回 reply 与 err。
func dialAndPing(t *testing.T, addr string, cliTLS *credentials.TransportCredentials) (*healthv1.PingReply, error) {
	t.Helper()
	conn, err := grpc.NewClient(addr, grpc.WithTransportCredentials(*cliTLS))
	if err != nil {
		t.Fatalf("拨号: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	return healthv1.NewHealthClient(conn).Ping(ctx, &healthv1.PingRequest{})
}

// TestPingOverTLS 是 feat/health-tls 的失败测试（存档点）。
//
// 验收：单向 TLS 下 agent 用 CA 验 server 成功后 Ping → ok。
// 当前 server.New 未接入 tlsConf（始终明文）→ TLS 客户端握手失败 → 本测试应红
// （红的原因 = TLS 未生效，非 Ping 逻辑错）。
func TestPingOverTLS(t *testing.T) {
	// 1 生成自签证书链（SAN 含 127.0.0.1）
	caPEM, certPEM, keyPEM, err := tlsutil.GenerateDevCert([]string{"127.0.0.1"})
	if err != nil {
		t.Fatalf("生成证书: %v", err)
	}
	srvTLS, err := tlsutil.ServerTLSConfig(certPEM, keyPEM)
	if err != nil {
		t.Fatalf("server TLS 配置: %v", err)
	}

	// 2 用 TLS 起 server
	s, lis, err := New("127.0.0.1:0", srvTLS)
	if err != nil {
		t.Fatalf("起 server: %v", err)
	}
	go func() { _ = s.Serve(lis) }()
	t.Cleanup(s.Stop)

	// 3 agent 用 CA 验 server 连，Ping → 期望 ok
	cliTLS, err := tlsutil.ClientTLSConfig(caPEM)
	if err != nil {
		t.Fatalf("client TLS 配置: %v", err)
	}
	creds := credentials.NewTLS(cliTLS)
	reply, err := dialAndPing(t, lis.Addr().String(), &creds)
	if err != nil {
		t.Fatalf("TLS Ping 失败: %v", err)
	}
	if !reply.GetOk() {
		t.Fatalf("期望 ok=true, 实际 %+v", reply)
	}
}

// TestPingOverTLS_RejectsWrongCA 负向：agent 用不匹配的 CA 验 server → 握手失败、拒连。
func TestPingOverTLS_RejectsWrongCA(t *testing.T) {
	// server 用证书链 A
	_, certPEM, keyPEM, _ := tlsutil.GenerateDevCert([]string{"127.0.0.1"})
	srvTLS, _ := tlsutil.ServerTLSConfig(certPEM, keyPEM)
	s, lis, err := New("127.0.0.1:0", srvTLS)
	if err != nil {
		t.Fatalf("起 server: %v", err)
	}
	go func() { _ = s.Serve(lis) }()
	t.Cleanup(s.Stop)

	// agent 用另一套 CA（证书链 B）验 → 应失败
	otherCAPEM, _, _, _ := tlsutil.GenerateDevCert([]string{"127.0.0.1"})
	cliTLS, _ := tlsutil.ClientTLSConfig(otherCAPEM)
	creds := credentials.NewTLS(cliTLS)
	if _, err := dialAndPing(t, lis.Addr().String(), &creds); err == nil {
		t.Fatalf("期望握手失败(CA 不匹配), 实际成功")
	}
}
