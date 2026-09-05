package audit

import (
	"context"

	"github.com/go-kratos/kratos/v3/middleware"
	"github.com/go-kratos/kratos/v3/transport"

	"github.com/jsl-aiot/platform/pkg/errors"
)

// Middleware 访问级审计：每个请求产生一条审计事件。
// 业务级审计（开通租户、冻结等关键动作）由用例层显式 Emit，不依赖本中间件。
// 链内位置：recovery → tenantcontext → authz → [audit] → OTel。
func Middleware(em *Emitter) middleware.Middleware {
	return func(handler middleware.Handler) middleware.Handler {
		return func(ctx context.Context, req interface{}) (interface{}, error) {
			reply, err := handler(ctx, req)
			action := "request"
			if tr, ok := transport.FromServerContext(ctx); ok {
				action = tr.Operation()
			}
			result := ResultSuccess
			extra := map[string]string{}
			if err != nil {
				result = ResultFailure
				extra["error_code"] = errors.CodeOf(err)
			}
			// 审计失败不阻断业务（记日志由 Sink 层负责），但也不吞业务错误
			_ = em.Emit(ctx, action, "", result, extra)
			return reply, err
		}
	}
}
