// tenant-bc 服务入口 —— BC-P1 租户与站点（BP-02 §3.1）。
// 壳层：环境装配（日志/OTel/验签桩/审计）+ 传输服务器生命周期（ADR-0004 决策 1）。
package main

import (
	"context"
	"os"

	"github.com/go-kratos/kratos/v3"

	"github.com/jsl-aiot/platform/pkg/audit"
	"github.com/jsl-aiot/platform/pkg/authz"
	"github.com/jsl-aiot/platform/pkg/observability"
	"github.com/jsl-aiot/platform/services/tenant/internal/server"
)

var (
	Name    = "tenant-bc"
	Version = "v0.0.1"
)

func main() {
	ctx := context.Background()

	cfg := observability.Config{
		ServiceName:  Name,
		Version:      Version,
		Env:          envOr("JSL_ENV", "dev"),
		OTLPEndpoint: os.Getenv("OTEL_EXPORTER_OTLP_ENDPOINT"),
	}
	shutdown, err := observability.Init(ctx, cfg)
	if err != nil {
		panic(err)
	}
	defer shutdown(ctx)

	logger := observability.NewLogger(cfg)

	// authz 桩：本地 HMAC 验签；BC-P2 落地后替换为 JWKS Verifier（接口不变）
	secret := os.Getenv("JSL_AUTHZ_JWT_SECRET")
	if secret == "" {
		logger.Warn("JSL_AUTHZ_JWT_SECRET not set, using dev default secret (local development only)")
		secret = "dev-secret-change-me"
	}
	verifier := authz.NewStubVerifier([]byte(secret))

	// 审计：M1 结构化日志 Sink；事件总线 Sink 随步 18 接入
	emitter := audit.NewEmitter(audit.NewLogSink(logger))

	opts := server.Options{Logger: logger, Verifier: verifier, Audit: emitter}
	app := kratos.New(
		kratos.Name(Name),
		kratos.Version(Version),
		kratos.Server(server.NewHTTP(opts), server.NewGRPC(opts)),
	)
	if err := app.Run(); err != nil {
		panic(err)
	}
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
