package service

import (
	"context"
	"time"

	healthv1 "github.com/Jingk97/topus-server/api/topus/health/v1"
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

// Ping 链路预检：连通即返回 ok + 版本 + 服务端时间。
//
// 不需 secret/证书——它只验"链路可达 + server 存活"，供 agent 批量纳管前预检。
// 返回 server_version / server_time 便于排查版本不一致与时钟漂移。
func (s *HealthService) Ping(ctx context.Context, _ *healthv1.PingRequest) (*healthv1.PingReply, error) {
	return &healthv1.PingReply{
		Ok:             true,
		ServerVersion:  Version,
		ServerTimeUnix: time.Now().Unix(),
	}, nil
}
