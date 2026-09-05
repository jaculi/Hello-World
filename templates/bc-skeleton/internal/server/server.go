// Package server —— 传输层装配（壳层）。
//
// 中间件链顺序为全平台标准（ADR-0004 决策 2，步 17 固化）：
//
//	recovery → tenantcontext → authz → audit → OTel
//
// 其中 tenantcontext / authz / audit / OTel 自 M1（pkg/ 壳层组件）起逐步接入。
package server

import (
	kgrpc "github.com/go-kratos/kratos/v3/transport/grpc"
	khttp "github.com/go-kratos/kratos/v3/transport/http"

	"github.com/go-kratos/kratos/v3/middleware/recovery"
)

// 端口约定：HTTP :8000 / gRPC :9000（M1 起迁移至 configs/config.yaml 配置加载）。
const (
	httpAddr = ":8000"
	grpcAddr = ":9000"
)

// NewHTTP 创建 HTTP 服务器（同时服务开放 API 与内部调试端点）。
func NewHTTP() *khttp.Server {
	return khttp.NewServer(
		khttp.Address(httpAddr),
		khttp.Middleware(recovery.Recovery()),
	)
}

// NewGRPC 创建 gRPC 服务器（BC 间同步调用走 gRPC，契约见 api/，ADR-0005）。
func NewGRPC() *kgrpc.Server {
	return kgrpc.NewServer(
		kgrpc.Address(grpcAddr),
		kgrpc.Middleware(recovery.Recovery()),
	)
}
