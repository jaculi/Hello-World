// tenant-bc 服务入口 —— BC-P1 租户与站点（BP-02 §3.1）。
// 壳层：环境装配（日志/OTel/验签桩/审计/数据层）+ 传输服务器生命周期（ADR-0004 决策 1）。
package main

import (
	"context"
	"log/slog"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/go-kratos/kratos/v3"
	"github.com/go-kratos/kratos/v3/transport"

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
	// relay 事务性发件箱中继（仅 PostgreSQL 模式且配置 Kafka 时非空）。
	relay *data.Relay
	close func()
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
	defer func() { _ = shutdown(ctx) }()

	logger := observability.NewLogger(cfg)

	// authz：按 JSL_AUTHZ_MODE 装配验签器
	//   stub（默认）：本地 HMAC 桩（dev/CI/单测）
	//   jwks：OIDC JWKS 远程验签（Keycloak，M18）
	verifier, err := authz.NewVerifierFromEnv(ctx)
	if err != nil {
		logger.Error("authz verifier init failed", "err", err)
		os.Exit(1)
	}
	if os.Getenv("JSL_AUTHZ_MODE") == "jwks" {
		logger.Info("authz: oidc jwks verifier", "issuer", os.Getenv("JSL_AUTHZ_ISSUER"))
	} else if os.Getenv("JSL_AUTHZ_JWT_SECRET") == "" {
		logger.Warn("JSL_AUTHZ_JWT_SECRET not set, using dev default secret (local development only)")
	}

	// 审计：M1 结构化日志 Sink；事件形态随 BC-P5 审计上下文接入
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

	servers := []transport.Server{server.NewHTTP(opts), server.NewGRPC(opts)}
	if d.relay != nil {
		// 发件箱中继挂入 kratos 生命周期（Start 阻塞轮询，Stop 优雅退出）
		servers = append(servers, &relayServer{r: d.relay})
	}

	app := kratos.New(
		kratos.Name(Name),
		kratos.Version(Version),
		kratos.Server(servers...),
	)
	if err := app.Run(); err != nil {
		panic(err)
	}
}

// relayServer 发件箱中继的生命周期适配（transport.Server 形态，ADR-0004 壳层接线）。
type relayServer struct{ r *data.Relay }

func (s *relayServer) Start(ctx context.Context) error { return s.r.Run(ctx) }
func (s *relayServer) Stop(ctx context.Context) error  { return s.r.Stop(ctx) }

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

		// 发件箱中继：JSL_KAFKA_BROKERS 缺省时 dev 仅告警（事件积压于 outbox），非 dev 拒绝启动
		brokers := envSlice("JSL_KAFKA_BROKERS")
		if len(brokers) == 0 {
			if envOr("JSL_ENV", "dev") != "dev" {
				logger.Error("JSL_KAFKA_BROKERS is required outside dev environment")
				os.Exit(1)
			}
			logger.Warn("JSL_KAFKA_BROKERS not set — outbox relay disabled (events accumulate in outbox)")
			return deps{
				tx: d.Tx, tenants: d.Tenants, orgs: d.Orgs, sites: d.Sites,
				plans: d.Plans, events: d.Events,
				close: pool.Close,
			}
		}
		topic := envOr("JSL_KAFKA_TOPIC", "platform.events.v1")
		sink, err := data.NewKafkaSink(brokers, topic)
		if err != nil {
			logger.Error("kafka sink init failed", "err", err)
			os.Exit(1)
		}
		relay := data.NewRelay(pool, sink,
			envInt("JSL_OUTBOX_BATCH", 100),
			envDur("JSL_OUTBOX_INTERVAL", 2*time.Second),
			logger)
		logger.Info("outbox relay: kafka", "brokers", brokers, "topic", topic)
		return deps{
			tx: d.Tx, tenants: d.Tenants, orgs: d.Orgs, sites: d.Sites,
			plans: d.Plans, events: d.Events, relay: relay,
			close: func() { pool.Close(); sink.Close() },
		}
	}
	if envOr("JSL_ENV", "dev") != "dev" {
		logger.Error("JSL_DB_DSN is required outside dev environment")
		os.Exit(1)
	}
	logger.Warn("JSL_DB_DSN not set — running with in-memory stores (development only)")
	tenants := biz.NewMemTenantRepo()
	orgs := biz.NewMemOrgRepo()
	sites := biz.NewMemSiteRepo()
	seedDemoTenant(ctx, logger, tenants, orgs, sites)
	return deps{
		tx:      biz.MemTransactor{},
		tenants: tenants,
		orgs:    orgs,
		sites:   sites,
		plans: biz.NewMemPlanRepo(
			biz.Plan{ID: "plan-free", Name: "Free", DefaultIsolation: biz.IsolationT3, Active: true},
			biz.Plan{ID: "plan-pro", Name: "Pro", DefaultIsolation: biz.IsolationT3, Active: true},
		),
		events: &biz.MemEventPublisher{},
		close:  func() {},
	}
}

// demoTenantID 与 deploy/identity/keycloak/jsl-realm.json demo-admin 用户的 tenant_id 一致。
// dev 内存模式 seed 该租户 + 根组织 + 根站点 + 示例子站点，使浏览器登录后端到端可见。
const demoTenantID = "01JSM18DEM0TENANT000000001"

// seedDemoTenant 仅在 dev 内存模式调用（M19）：预填与 Keycloak demo-admin 令牌
// tenant_id 一致的租户、根组织、根站点与一个示例子站点，使门户浏览器流程可读。
func seedDemoTenant(ctx context.Context, logger *slog.Logger, tenants biz.TenantRepo, orgs biz.OrgRepo, sites biz.SiteRepo) {
	now := time.Now().UTC()
	au := biz.Audit{CreatedAt: now, CreatedBy: "system:seed", UpdatedAt: now, UpdatedBy: "system:seed", Version: 1}
	t := &biz.Tenant{
		ID: demoTenantID, Name: "demo", DisplayName: "Demo Tenant",
		Status: biz.TenantStatusActive, Isolation: biz.IsolationT3,
		PlanID: "plan-free", HomeRegion: "cn-north-1", Audit: au,
	}
	if err := tenants.Insert(ctx, nil, t, &biz.Subscription{
		ID: "01JSM18DEM0SUB0000000000001", TenantID: demoTenantID, PlanID: "plan-free", Status: "active", Audit: au,
	}, nil); err != nil {
		logger.Warn("seed demo tenant: insert tenant failed (may already exist)", "err", err)
		return
	}
	rootOrg := &biz.Organization{ID: "01JSM18DEM0ORG0000000000001", TenantID: demoTenantID, Name: "root", DisplayName: "根组织", Audit: au}
	rootSite := &biz.Site{ID: "01JSM18DEM0SITE0000000000001", TenantID: demoTenantID, Name: "root", DisplayName: "根站点", SiteType: "root", Audit: au}
	factoryA := &biz.Site{ID: "01JSM18DEM0SITE0000000000002", TenantID: demoTenantID, ParentSiteID: rootSite.ID, Name: "factory-a", DisplayName: "A 号厂区", SiteType: "factory", Audit: au}
	for _, s := range []*biz.Site{rootSite, factoryA} {
		if err := sites.Insert(ctx, nil, s); err != nil {
			logger.Warn("seed demo site: insert failed (may already exist)", "id", s.ID, "err", err)
		}
	}
	if err := orgs.Insert(ctx, nil, rootOrg); err != nil {
		logger.Warn("seed demo org: insert failed (may already exist)", "err", err)
	}
	logger.Info("seed: demo tenant provisioned", "tenant_id", demoTenantID,
		"site_ids", []string{rootSite.ID, factoryA.ID})
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

// envSlice 逗号分隔环境变量（如 Kafka broker 列表）。
func envSlice(key string) []string {
	raw := os.Getenv(key)
	if raw == "" {
		return nil
	}
	var out []string
	for _, s := range strings.Split(raw, ",") {
		if s = strings.TrimSpace(s); s != "" {
			out = append(out, s)
		}
	}
	return out
}

func envInt(key string, fallback int) int {
	if v := os.Getenv(key); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
	}
	return fallback
}

func envDur(key string, fallback time.Duration) time.Duration {
	if v := os.Getenv(key); v != "" {
		if d, err := time.ParseDuration(v); err == nil {
			return d
		}
	}
	return fallback
}
