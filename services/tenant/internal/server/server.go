// Package server —— 传输层装配（壳层，步 17）。
//
// 全平台标准中间件链（ADR-0004 决策 2，顺序不可调整）：
//
//	recovery → tenantcontext → authz → audit → OTel(tracing)
//
// HTTP 错误统一编码为 PlatformError JSON（{code, message_code, params}），
// gRPC 经 kratos Error.GRPCStatus 携带 ErrorInfo（reason + params）。
package server

import (
	"encoding/json"
	"log/slog"
	"net/http"

	"github.com/go-kratos/kratos/v3/errors"
	"github.com/go-kratos/kratos/v3/middleware"
	"github.com/go-kratos/kratos/v3/middleware/recovery"
	kgrpc "github.com/go-kratos/kratos/v3/transport/grpc"
	khttp "github.com/go-kratos/kratos/v3/transport/http"

	orgv1 "github.com/jsl-aiot/platform/api/org/v1"
	sitev1 "github.com/jsl-aiot/platform/api/site/v1"
	tenantv1 "github.com/jsl-aiot/platform/api/tenant/v1"
	"github.com/jsl-aiot/platform/pkg/audit"
	"github.com/jsl-aiot/platform/pkg/authz"
	platerrors "github.com/jsl-aiot/platform/pkg/errors"
	"github.com/jsl-aiot/platform/pkg/observability"
	"github.com/jsl-aiot/platform/pkg/tenantcontext"
	"github.com/jsl-aiot/platform/services/tenant/internal/service"
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

	Tenant *service.TenantService
	Org    *service.OrgService
	Site   *service.SiteService
}

// baseMiddleware 全平台标准中间件链。
func baseMiddleware(o Options) []middleware.Middleware {
	_ = o.Logger // 日志器当前经 observability.From(ctx) 使用；显式注入链占位（M5 复核）
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
	srv := khttp.NewServer(
		khttp.Address(httpAddr),
		khttp.Middleware(baseMiddleware(o)...),
		khttp.ErrorEncoder(encodePlatformError),
	)
	tenantv1.RegisterTenantServiceHTTPServer(srv, o.Tenant)
	orgv1.RegisterOrgServiceHTTPServer(srv, o.Org)
	sitev1.RegisterSiteServiceHTTPServer(srv, o.Site)
	srv.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"SERVING"}`))
	})
	return srv
}

// NewGRPC 创建 gRPC 服务器（BC 间同步调用，契约见 api/，ADR-0005）。
func NewGRPC(o Options) *kgrpc.Server {
	srv := kgrpc.NewServer(
		kgrpc.Address(grpcAddr),
		kgrpc.Middleware(baseMiddleware(o)...),
	)
	tenantv1.RegisterTenantServiceServer(srv, o.Tenant)
	orgv1.RegisterOrgServiceServer(srv, o.Org)
	sitev1.RegisterSiteServiceServer(srv, o.Site)
	return srv
}

// platformErrorPayload HTTP 错误负载（契约 common.v1.PlatformError 的 JSON 形态）。
type platformErrorPayload struct {
	Code        int               `json:"code"`
	MessageCode string            `json:"message_code"`
	Params      map[string]string `json:"params"`
}

// encodePlatformError 统一错误编码：消息码 + 渲染参数；非平台错误归并 500（Cause 永不外传）。
func encodePlatformError(w http.ResponseWriter, _ *http.Request, err error) {
	code := 500
	msgCode := "platform.internal"
	params := map[string]string{}

	if e, ok := platerrors.From(err); ok {
		code = e.HTTPCode
		msgCode = e.Code
		params = e.Params
	} else if se := errors.FromError(err); se != nil {
		code = int(se.Code)
		if se.Reason != "" {
			msgCode = se.Reason
		}
		for k, v := range se.Metadata {
			params[k] = v
		}
	}
	if params == nil {
		params = map[string]string{}
	}

	body, merr := json.Marshal(platformErrorPayload{Code: code, MessageCode: msgCode, Params: params})
	if merr != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(code)
	_, _ = w.Write(body)
}
