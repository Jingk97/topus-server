// Package server 装配 gRPC server（适配器层）。
// 它把领域服务（internal/service）注册到 gRPC 运行时，并按需启用 TLS。
package server

import (
	"crypto/tls"
	"fmt"
	"net"

	healthv1 "github.com/Jingk97/topus-server/api/topus/health/v1"
	"github.com/Jingk97/topus-server/internal/service"
	"google.golang.org/grpc"
)

// New 创建 gRPC server 并在 addr 上监听。
//
// tlsConf 非 nil 则启用 TLS（单向：server 出示证书，agent 验）。
// 返回 server 与 listener，由调用方决定何时 Serve（便于测试控制生命周期）。
//
// C1 存档点：此版**故意未接入 tlsConf**（始终明文），用于让 TLS 测试因
// "TLS 未生效"而红。C2 再接入 grpc.Creds。
func New(addr string, tlsConf *tls.Config) (*grpc.Server, net.Listener, error) {
	lis, err := net.Listen("tcp", addr)
	if err != nil {
		return nil, nil, fmt.Errorf("监听 %s: %w", addr, err)
	}

	var opts []grpc.ServerOption
	// TODO(C2): tlsConf 非 nil 时 opts = append(opts, grpc.Creds(credentials.NewTLS(tlsConf)))

	s := grpc.NewServer(opts...)
	healthv1.RegisterHealthServer(s, service.NewHealthService())
	return s, lis, nil
}
