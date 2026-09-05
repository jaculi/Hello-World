// Package observability —— OTel 初始化与结构化日志（BP-04 §5）。
//
// 硬约束（BP-05 §6.3）：
//   - 每条日志在可得上时强制携带 tenant_id / trace_id（From 自动派生）；
//   - S3/S4 级数据（凭据、生物特征、体征等）禁止写入日志。
package observability

import (
	"context"
	"log/slog"
	"os"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetricgrpc"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/trace"
	"go.opentelemetry.io/otel/trace/noop"

	"github.com/jsl-aiot/platform/pkg/tenantcontext"
)

// Config 可观测性配置。
type Config struct {
	ServiceName  string
	Version      string
	Env          string // dev / staging / prod
	OTLPEndpoint string // 为空 → noop Provider（本地开发零依赖）
}

// Init 初始化 OTel（tracer + meter + 传播器），返回关闭函数。
func Init(ctx context.Context, cfg Config) (func(context.Context) error, error) {
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{}, propagation.Baggage{},
	))
	if cfg.OTLPEndpoint == "" {
		otel.SetTracerProvider(noop.NewTracerProvider())
		return func(context.Context) error { return nil }, nil
	}

	res := resource.NewSchemaless(
		attribute.String("service.name", cfg.ServiceName),
		attribute.String("service.version", cfg.Version),
		attribute.String("deployment.environment", cfg.Env),
	)

	traceExp, err := otlptracegrpc.New(ctx,
		otlptracegrpc.WithEndpoint(cfg.OTLPEndpoint),
		otlptracegrpc.WithInsecure(), // 集群内明文，出网关由 mTLS 网关策略负责（BP-05 §3.2）
	)
	if err != nil {
		return nil, err
	}
	tp := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(traceExp),
		sdktrace.WithResource(res),
	)
	otel.SetTracerProvider(tp)

	metricExp, err := otlpmetricgrpc.New(ctx,
		otlpmetricgrpc.WithEndpoint(cfg.OTLPEndpoint),
		otlpmetricgrpc.WithInsecure(),
	)
	if err != nil {
		return nil, err
	}
	mp := sdkmetric.NewMeterProvider(
		sdkmetric.WithReader(sdkmetric.NewPeriodicReader(metricExp)),
		sdkmetric.WithResource(res),
	)
	otel.SetMeterProvider(mp)

	return func(ctx context.Context) error {
		if err := tp.Shutdown(ctx); err != nil {
			return err
		}
		return mp.Shutdown(ctx)
	}, nil
}

var baseLogger *slog.Logger

// NewLogger 构建服务基础 JSON 日志器（写入 stdout，由 Loki 采集，BP-04 §5）。
func NewLogger(cfg Config) *slog.Logger {
	baseLogger = slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	})).With(
		slog.String("service", cfg.ServiceName),
		slog.String("version", cfg.Version),
		slog.String("env", cfg.Env),
	)
	return baseLogger
}

// From 从上下文派生日志器：自动附加租户与链路字段（可得时）。
func From(ctx context.Context) *slog.Logger {
	l := baseLogger
	if l == nil {
		l = slog.Default()
	}
	if tc, ok := tenantcontext.From(ctx); ok {
		l = l.With(slog.String("tenant_id", tc.TenantID))
		if tc.OrgID != "" {
			l = l.With(slog.String("org_id", tc.OrgID))
		}
		if tc.SiteID != "" {
			l = l.With(slog.String("site_id", tc.SiteID))
		}
		if tc.Subject != "" {
			l = l.With(slog.String("subject", tc.Subject))
		}
	}
	if sc := trace.SpanContextFromContext(ctx); sc.HasTraceID() {
		l = l.With(slog.String("trace_id", sc.TraceID().String()))
	}
	return l
}
