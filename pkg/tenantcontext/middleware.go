package tenantcontext

import (
	"context"
	"net/http"

	"github.com/go-kratos/kratos/v3/middleware"
	"github.com/go-kratos/kratos/v3/transport"

	"github.com/jsl-aiot/platform/pkg/errors"
)

// 平台元数据头约定（HTTP 与 gRPC metadata 同键）。
const (
	MetadataTenantID = "x-tenant-id"
	MetadataOrgID    = "x-org-id"
	MetadataSiteID   = "x-site-id"
)

// 缺失租户上下文的消息码。
const CodeTenantMissing = "platform.tenant_context_missing"

// Middleware 从请求头注入租户上下文；租户头缺失即拒（BP-05 §4.2 第一道闸，fail-closed）。
//
// 链内位置：recovery → [tenantcontext] → authz → audit → OTel。
// 内部管理端点（健康检查等）的豁免清单于 M2 随路由注册引入。
func Middleware() middleware.Middleware {
	return func(handler middleware.Handler) middleware.Handler {
		return func(ctx context.Context, req interface{}) (interface{}, error) {
			if tr, ok := transport.FromServerContext(ctx); ok {
				hdr := tr.RequestHeader()
				tenantID := hdr.Get(MetadataTenantID)
				if tenantID == "" {
					return nil, errors.New(CodeTenantMissing, http.StatusBadRequest)
				}
				ctx = With(ctx, Context{
					TenantID: tenantID,
					OrgID:    hdr.Get(MetadataOrgID),
					SiteID:   hdr.Get(MetadataSiteID),
				})
			}
			return handler(ctx, req)
		}
	}
}
