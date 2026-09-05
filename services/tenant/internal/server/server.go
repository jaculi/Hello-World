// Package server —— 传输层装配（壳层）。
//
// 全平台标准中间件链（ADR-0004 决策 2，顺序不可调整）：
//
//	recovery → tenantcontext → authz → audit → OTel(tracing)
package server

import (
	"log/slog"

	"github.com/go-kratos/kratos/v3/middleware"
	"github.com/go-kratos/kratos/v3/middleware/recovery"
	kgrpc "github.com/go-kratos/kratos/v3/transport/grpc"
	khttp "github.com/go-kratos/kratos/v3/transport/http"

	"github.com/jsl-aiot/platform/pkg/audit"
	"github.com/jsl-aiot/platform/pkg/authz"
	"github.com/jsl-aiot/platform/pkg/observability"
	"github.com/jsl-aiot/platform/pkg/tenantcontext"
)

const (
	httpAddr = ":8000"
	grpcAddr = ":9000"
)

// Options 服务器装配选项（main 中组合，server 不感知环境配置来源）。
type Options struct {
	Logger   *slog.Logger
	Verifier authz.Verifier
	Audit    *audit.Emitter
}

// baseMiddleware 全平台标准中间件链。
func baseMiddleware(o Options) []middleware.Middleware {
	_ = o.Logger // 日志器当前经 observability.From(ctx) 使用；显式注入链占位（M2 复核）
	return []middleware.Middleware{
		recovery.Recovery(),
		tenantcontext.Middleware(),
		authz.Middleware(o.Verifier),
		audit.Middleware(o.Audit),
		observability.Tracing(),
	}
}

// NewHTTP 创建 HTTP 服务器（开放 API 与调试端点）。
func NewHTTP(o Options) *khttp.Server {
	return khttp.NewServer(
		khttp.Address(httpAddr),
		khttp.Middleware(baseMiddleware(o)...),
	)
}

// NewGRPC 创建 gRPC 服务器（BC 间同步调用，契约见 api/，ADR-0005）。
func NewGRPC(o Options) *kgrpc.Server {
	return kgrpc.NewServer(
		kgrpc.Address(grpcAddr),
		kgrpc.Middleware(baseMiddleware(o)...),
	)
}
