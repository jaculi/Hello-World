// Package audit —— 审计事件发射（BP-05 §9.1）。
//
// 审计结构四要素：who（Subject）/ when（Time）/ what（Action+Resource）/ result（Result），
// 外加 tenant/trace 上下文（Context 仅允许低敏补充信息，S3/S4 禁入）。
// M1：结构化日志 Sink（Loki 采集，WORM 化随 BP-07 §4.6 运营日历落地）；
// 事件总线 Sink（outbox → Kafka）随步 18 接入，Sink 接口已预留。
package audit

import (
	"context"
	"time"

	"github.com/oklog/ulid/v2"
	"go.opentelemetry.io/otel/trace"

	"github.com/jsl-aiot/platform/pkg/tenantcontext"
)

// 审计结果枚举。
const (
	ResultSuccess = "success"
	ResultDenied  = "denied"
	ResultFailure = "failure"
)

// Event 审计事件。
type Event struct {
	ID       string            // ULID（BP-03 §4.1 标识规范）
	Time     time.Time
	TenantID string
	Subject  string            // 谁（系统动作为 "system:<name>"）
	Action   string            // 做了什么，约定 "<域>.<对象>.<动作>"（访问级审计为 operation）
	Resource string            // 对象标识
	Result   string
	TraceID  string
	Context  map[string]string // 补充上下文（如错误消息码、目标租户）
}

// Sink 审计事件出口（日志 Sink / 事件总线 Sink / WORM 存储 Sink 均实现此接口）。
type Sink interface {
	Emit(ctx context.Context, e Event) error
}

// Emitter 自动补全租户/主体/链路字段的审计发射器（业务代码统一入口）。
type Emitter struct {
	sink Sink
}

func NewEmitter(s Sink) *Emitter { return &Emitter{sink: s} }

// Emit 发射审计事件；ctx 中的租户上下文与 OTel 链路自动补全。
func (em *Emitter) Emit(ctx context.Context, action, resource, result string, extra map[string]string) error {
	e := Event{
		ID:       ulid.Make().String(),
		Time:     time.Now(),
		Action:   action,
		Resource: resource,
		Result:   result,
		Context:  extra,
	}
	if tc, ok := tenantcontext.From(ctx); ok {
		e.TenantID = tc.TenantID
		if e.Subject == "" {
			e.Subject = tc.Subject
		}
	}
	if sc := trace.SpanContextFromContext(ctx); sc.HasTraceID() {
		e.TraceID = sc.TraceID().String()
	}
	if e.Subject == "" {
		e.Subject = "system:unknown"
	}
	return em.sink.Emit(ctx, e)
}
