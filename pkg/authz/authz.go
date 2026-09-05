// Package authz —— 认证与主体-租户一致性校验（BP-05 §3.1 / §4.2）。
//
// M1 提供本地 HMAC 验签桩（StubVerifier，供开发与测试）；
// BC-P2 落地后替换为 JWKS 远程验签实现，Verifier 接口保持不变。
package authz

import "context"

// Claims 主体身份声明。
type Claims struct {
	Subject  string   // 主体标识（人/服务/插件账号）
	TenantID string   // 令牌归属租户（与请求头租户一致性强制校验）
	Roles    []string // 平台角色（BC-P2 RBAC 细化前为粗粒度）
}

// Verifier 令牌验证器（BC-P2 对接点）。
type Verifier interface {
	Verify(ctx context.Context, rawToken string) (Claims, error)
}
