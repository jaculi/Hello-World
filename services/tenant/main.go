// tenant-bc 服务入口 —— BC-P1 租户与站点（BP-02 §3.1）。
// 壳层：环境装配（日志/OTel/验签桩/审计/数据层）+ 传输服务器生命周期（ADR-0004 决策 1）。
package main

import (
	"context"
	"log/slog"
	"os"

	"github.com/go-kratos/kratos/v3"

	"github.com/jsl-aiot/platform/pkg/audit"
	"github.com/jsl-aiot/platform/pkg/authz"
	"github.com/jsl-aiot/platform/pkg/dataaccess"
	"github.com/jsl-aiot/platform/pkg/observability"
	"github.com/jsl-aiot/platform/services/tenant/internal/biz"
	"github.com/jsl-aiot/platform/services/tenant/internal/data"
	"github.com/jsl-aiot/platform/services/tenant/internal/server"
	"github.com/jsl-aiot/platform/services/tenant/internal/service"
)

var (
	Name    = "tenant-bc"
	Version = "v0.0.1"
)

// deps 领域端口依赖集（PostgreSQL 或内存开发模式装配）。
type deps struct {
	tx      biz.Transactor
	tenants biz.TenantRepo
	orgs    biz.OrgRepo
	sites   biz.SiteRepo
	plans   biz.PlanRepo
	events  biz.EventPublisher
	close   func()
}

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

	d := buildDeps(ctx, logger)
	defer d.close()

	// 领域用例装配（biz 仅面向端口）
	tenantUC := biz.NewTenantUseCase(d.tx, d.tenants, d.plans, d.orgs, d.sites, d.events, emitter)
	orgUC := biz.NewOrgUseCase(d.tx, d.orgs, emitter)
	siteUC := biz.NewSiteUseCase(d.tx, d.sites, d.events, emitter)

	opts := server.Options{
		Logger:   logger,
		Verifier: verifier,
		Audit:    emitter,
		Tenant:   service.NewTenantService(tenantUC),
		Org:      service.NewOrgService(orgUC),
		Site:     service.NewSiteService(siteUC),
	}

	app := kratos.New(
		kratos.Name(Name),
		kratos.Version(Version),
		kratos.Server(server.NewHTTP(opts), server.NewGRPC(opts)),
	)
	if err := app.Run(); err != nil {
		panic(err)
	}
}

// buildDeps 依配置装配数据层：JSL_DB_DSN 存在走 PostgreSQL（先迁移）；
// dev 无 DSN 以内存替身运行（本地开发/联调）；非 dev 无 DSN 拒绝启动（fail-fast）。
func buildDeps(ctx context.Context, logger *slog.Logger) deps {
	if dsn := os.Getenv("JSL_DB_DSN"); dsn != "" {
		pool, err := dataaccess.NewPool(ctx, dsn)
		if err != nil {
			logger.Error("database pool init failed", "err", err)
			os.Exit(1)
		}
		d := data.New(pool)
		if err := d.Migrate(ctx); err != nil {
			logger.Error("schema migration failed", "err", err)
			os.Exit(1)
		}
		logger.Info("data layer: postgresql (migrations applied)")
		return deps{
			tx: d.Tx, tenants: d.Tenants, orgs: d.Orgs, sites: d.Sites,
			plans: d.Plans, events: d.Events,
			close: pool.Close,
		}
	}
	if envOr("JSL_ENV", "dev") != "dev" {
		logger.Error("JSL_DB_DSN is required outside dev environment")
		os.Exit(1)
	}
	logger.Warn("JSL_DB_DSN not set — running with in-memory stores (development only)")
	return deps{
		tx:      biz.MemTransactor{},
		tenants: biz.NewMemTenantRepo(),
		orgs:    biz.NewMemOrgRepo(),
		sites:   biz.NewMemSiteRepo(),
		plans: biz.NewMemPlanRepo(
			biz.Plan{ID: "plan-free", Name: "Free", DefaultIsolation: biz.IsolationT3, Active: true},
			biz.Plan{ID: "plan-pro", Name: "Pro", DefaultIsolation: biz.IsolationT3, Active: true},
		),
		events: &biz.MemEventPublisher{},
		close:  func() {},
	}
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
