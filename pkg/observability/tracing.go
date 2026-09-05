package observability

import (
	"context"

	"github.com/go-kratos/kratos/v3/middleware"
	"github.com/go-kratos/kratos/v3/transport"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"

	"github.com/jsl-aiot/platform/pkg/errors"
	"github.com/jsl-aiot/platform/pkg/tenantcontext"
)

// headerCarrier kratos 传输头（transport.Header 接口）→ OTel 载体适配。
type headerCarrier struct {
	h transport.Header
}

func (c headerCarrier) Get(key string) string { return c.h.Get(key) }
func (c headerCarrier) Set(key, value string) { c.h.Set(key, value) }
func (c headerCarrier) Keys() []string        { return c.h.Keys() }

// Tracing 服务端链路中间件（kratos v3 已移除自带 tracing 中间件，此为本平台实现）：
//
//	上游链路上文提取（TraceContext/Baggage）→ 开启 Server Span → 错误记录与状态标注
//	Span 附加 tenant 属性（租户维度链路检索，BP-04 §5）。
//
// 链内位置：最内层（recovery → tenantcontext → authz → audit → [Tracing]）。
func Tracing() middleware.Middleware {
	tracer := otel.Tracer("github.com/jsl-aiot/platform")
	prop := otel.GetTextMapPropagator()
	return func(handler middleware.Handler) middleware.Handler {
		return func(ctx context.Context, req interface{}) (interface{}, error) {
			tr, ok := transport.FromServerContext(ctx)
			if !ok {
				return handler(ctx, req)
			}
			ctx = prop.Extract(ctx, headerCarrier{h: tr.RequestHeader()})
			ctx, span := tracer.Start(ctx, tr.Operation(), trace.WithSpanKind(trace.SpanKindServer))
			defer span.End()

			if tc, ok := tenantcontext.From(ctx); ok {
				span.SetAttributes(attribute.String("platform.tenant_id", tc.TenantID))
			}

			reply, err := handler(ctx, req)
			if err != nil {
				span.RecordError(err)
				// 消息码入 Span 状态描述（不含渲染文本与敏感数据）
				span.SetStatus(codes.Error, errors.CodeOf(err))
			}
			return reply, err
		}
	}
}
