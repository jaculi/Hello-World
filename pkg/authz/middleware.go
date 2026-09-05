package authz

import (
	"context"
	"net/http"
	"strings"

	"github.com/go-kratos/kratos/v3/middleware"
	"github.com/go-kratos/kratos/v3/transport"

	"github.com/jsl-aiot/platform/pkg/errors"
	"github.com/jsl-aiot/platform/pkg/tenantcontext"
)

// 消息码。
const (
	CodeTokenMissing   = "platform.token_missing"
	CodeTenantMismatch = "platform.token_tenant_mismatch"
)

// bindClaims 主体声明 → 租户一致性校验 + 主体注入（独立函数便于单测）。
func bindClaims(ctx context.Context, claims Claims) (context.Context, error) {
	tc, ok := tenantcontext.From(ctx)
	// 租户一致性：令牌归属租户必须与请求头租户一致（跨租户令牌即拒）
	if ok && claims.TenantID != "" && claims.TenantID != tc.TenantID {
		return nil, errors.New(CodeTenantMismatch, http.StatusForbidden)
	}
	return tenantcontext.WithSubject(ctx, claims.Subject), nil
}

// Middleware 认证中间件：
//
//	Bearer 令牌 → Verifier 验签 → 令牌租户与请求头租户一致性校验（BP-05 §4.2 三道闸第一道）
//	→ 主体注入租户上下文。
//
// 链内位置：recovery → tenantcontext → [authz] → audit → OTel。
func Middleware(v Verifier) middleware.Middleware {
	return func(handler middleware.Handler) middleware.Handler {
		return func(ctx context.Context, req interface{}) (interface{}, error) {
			tr, ok := transport.FromServerContext(ctx)
			if !ok {
				return handler(ctx, req)
			}
			raw := tr.RequestHeader().Get("Authorization")
			token, ok := strings.CutPrefix(raw, "Bearer ")
			if !ok || token == "" {
				return nil, errors.New(CodeTokenMissing, http.StatusUnauthorized)
			}
			claims, err := v.Verify(ctx, token)
			if err != nil {
				return nil, err
			}
			out, err := bindClaims(ctx, claims)
			if err != nil {
				return nil, err
			}
			return handler(out, req)
		}
	}
}
