package service

import (
	"context"

	healthv1 "github.com/Jingk97/topus-server/api/topus/health/v1"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// Version 是 server 版本号（构建期可注入 ldflags，先硬编码占位）。
const Version = "0.0.1-skeleton"

// HealthService 实现链路预检接口 Ping。
//
// 设计意图：它只实现 gRPC 生成的服务接口，**不依赖任何框架（Kratos）**——
// 由 cmd/server 的 gRPC 适配器注册。这样领域/服务逻辑可纯单元测试，
// 符合"领域内核/service 不依赖框架，handler 为适配器"（全局规则 C2）。
type HealthService struct {
	healthv1.UnimplementedHealthServer
}

// NewHealthService 构造 HealthService。
func NewHealthService() *HealthService { return &HealthService{} }

// Ping 链路预检。
//
// C1 存档点：此版**故意未实现**（返回 Unimplemented），用于让失败测试因
// "逻辑未实现"而红（而非编译/链路错）。C2 再实现到返回 ok。
func (s *HealthService) Ping(ctx context.Context, _ *healthv1.PingRequest) (*healthv1.PingReply, error) {
	return nil, status.Error(codes.Unimplemented, "Ping 尚未实现（C1 失败测试存档点）")
}
