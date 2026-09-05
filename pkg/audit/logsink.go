package audit

import (
	"context"
	"log/slog"
)

// LogSink 结构化日志审计出口（M1 默认；事件总线 Sink 随步 18 接入）。
type LogSink struct {
	log *slog.Logger
}

func NewLogSink(log *slog.Logger) *LogSink { return &LogSink{log: log} }

// Emit 以 JSON 结构化日志落地审计事件（字段名与 BP-05 §9.1 审计结构对齐）。
func (s *LogSink) Emit(_ context.Context, e Event) error {
	attrs := []any{
		slog.String("audit.id", e.ID),
		slog.Time("audit.time", e.Time),
		slog.String("audit.tenant_id", e.TenantID),
		slog.String("audit.subject", e.Subject),
		slog.String("audit.action", e.Action),
		slog.String("audit.resource", e.Resource),
		slog.String("audit.result", e.Result),
	}
	if e.TraceID != "" {
		attrs = append(attrs, slog.String("audit.trace_id", e.TraceID))
	}
	for k, v := range e.Context {
		attrs = append(attrs, slog.String("audit.ctx."+k, v))
	}
	s.log.Info("audit", attrs...)
	return nil
}
