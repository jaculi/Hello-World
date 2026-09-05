// Package tenantcontext —— 平台租户上下文（BP-02 §2 / BP-05 §4.2）。
//
// 全链路归属链 tenant → org → site（BP-03 §4.1）自请求入口注入，
// 由 dataaccess 绑定到 RLS 会话变量、由日志/审计自动透出。
package tenantcontext

import "context"

// Context 租户上下文。
type Context struct {
	TenantID string // 必填：租户 ID（RLS 绑定键）
	OrgID    string // 可选：组织 ID（行业级行级策略用）
	SiteID   string // 可选：站点 ID
	Subject  string // 可选：主体标识（authz 中间件填充）
}

type ctxKey struct{}

// With 注入租户上下文。
func With(ctx context.Context, tc Context) context.Context {
	return context.WithValue(ctx, ctxKey{}, tc)
}

// From 提取租户上下文。
func From(ctx context.Context) (Context, bool) {
	tc, ok := ctx.Value(ctxKey{}).(Context)
	return tc, ok && tc.TenantID != ""
}

// MustFrom 提取租户上下文，缺失时 panic。
// 仅限确知运行在标准中间件链内（recovery → tenantcontext 之后）的代码使用。
func MustFrom(ctx context.Context) Context {
	tc, ok := From(ctx)
	if !ok {
		panic("tenantcontext: missing tenant context (middleware chain violated)")
	}
	return tc
}

// WithSubject 在已有上下文上补充主体（authz 中间件使用）。
func WithSubject(ctx context.Context, subject string) context.Context {
	tc, _ := From(ctx)
	tc.Subject = subject
	return With(ctx, tc)
}
