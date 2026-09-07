package authz

import (
	"context"
	"fmt"
	"os"

	"github.com/coreos/go-oidc/v3/oidc"

	"github.com/jsl-aiot/platform/pkg/errors"
)

// JWKSVerifier 基于 OIDC discovery + JWKS 的远程验签（M18，对接 Keycloak）。
//
// 替换 StubVerifier 的生产实现，Verifier 接口保持不变（pkg/authz/authz.go）：
// RS256 非对称验签（公钥来自 IdP JWKS 端点，go-oidc 自动缓存与轮换），
// 校验签名/过期/issuer/audience；任何失败一律 fail-closed（401）。
type JWKSVerifier struct {
	verifier    *oidc.IDTokenVerifier
	tenantClaim string
}

// NewJWKSVerifier 经 OIDC discovery 初始化验签器。
//
//	issuer：令牌签发者（如 http://keycloak:8080/realms/jsl）；
//	audience：预期受众（OIDC client_id）；
//	tenantClaim：租户声明键名（空则 tenant_id）。
func NewJWKSVerifier(ctx context.Context, issuer, audience, tenantClaim string) (*JWKSVerifier, error) {
	if issuer == "" {
		return nil, fmt.Errorf("authz: JSL_AUTHZ_ISSUER is required in jwks mode")
	}
	if audience == "" {
		return nil, fmt.Errorf("authz: JSL_AUTHZ_AUDIENCE is required in jwks mode")
	}
	if tenantClaim == "" {
		tenantClaim = "tenant_id"
	}
	provider, err := oidc.NewProvider(ctx, issuer)
	if err != nil {
		return nil, fmt.Errorf("authz: oidc discovery failed for %q: %w", issuer, err)
	}
	return &JWKSVerifier{
		verifier: provider.Verifier(&oidc.Config{ClientID: audience}),
		// 受众外的其他声明（iss/exp）由 go-oidc 按 discovery 文档强制校验
		tenantClaim: tenantClaim,
	}, nil
}

// Verify 验签并提取主体声明；缺租户声明视为无效令牌（fail-closed）。
func (v *JWKSVerifier) Verify(ctx context.Context, rawToken string) (Claims, error) {
	token, err := v.verifier.Verify(ctx, rawToken)
	if err != nil {
		return Claims{}, errors.New(CodeTokenInvalid, unauthorized)
	}
	var raw map[string]any
	if err := token.Claims(&raw); err != nil {
		return Claims{}, errors.New(CodeTokenInvalid, unauthorized)
	}
	tenantID, _ := raw[v.tenantClaim].(string)
	if tenantID == "" {
		// 无租户归属的令牌（如纯平台账号）暂不放行；BC-P2 平台主体模型落地后细化
		return Claims{}, errors.New(CodeTokenInvalid, unauthorized)
	}
	return Claims{
		Subject:  token.Subject,
		TenantID: tenantID,
		Roles:    extractRealmRoles(raw),
	}, nil
}

// extractRealmRoles 提取 Keycloak realm_access.roles 声明。
func extractRealmRoles(raw map[string]any) []string {
	realm, ok := raw["realm_access"].(map[string]any)
	if !ok {
		return nil
	}
	rr, ok := realm["roles"].([]any)
	if !ok {
		return nil
	}
	out := make([]string, 0, len(rr))
	for _, r := range rr {
		if s, ok := r.(string); ok {
			out = append(out, s)
		}
	}
	return out
}

// NewVerifierFromEnv 按环境变量装配验签器（M18）：
//
//	JSL_AUTHZ_MODE 空/stub → 本地 HMAC 桩（dev/CI/单测；读 JSL_AUTHZ_JWT_SECRET）
//	JSL_AUTHZ_MODE=jwks   → OIDC JWKS 远程验签（读 JSL_AUTHZ_ISSUER /
//	                        JSL_AUTHZ_AUDIENCE / JSL_AUTHZ_TENANT_CLAIM）
func NewVerifierFromEnv(ctx context.Context) (Verifier, error) {
	switch mode := os.Getenv("JSL_AUTHZ_MODE"); mode {
	case "", "stub":
		secret := os.Getenv("JSL_AUTHZ_JWT_SECRET")
		if secret == "" {
			secret = "dev-secret-change-me" // 与历史默认值一致；仅限本地开发
		}
		return NewStubVerifier([]byte(secret)), nil
	case "jwks":
		return NewJWKSVerifier(ctx,
			os.Getenv("JSL_AUTHZ_ISSUER"),
			os.Getenv("JSL_AUTHZ_AUDIENCE"),
			os.Getenv("JSL_AUTHZ_TENANT_CLAIM"),
		)
	default:
		return nil, fmt.Errorf("authz: unknown JSL_AUTHZ_MODE %q (want stub|jwks)", mode)
	}
}
