package authz

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"

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
//	issuer：令牌签发者（必须与令牌 iss 声明一致；亦为 go-oidc discovery 抓取 URL）；
//	audience：预期受众（OIDC client_id）；
//	tenantClaim：租户声明键名（空则 tenant_id）；
//	backchannelBase：可选（M19）；issuer URL 在本进程不可达（如浏览器面 URL
//	  http://localhost:9080/realms/jsl）时，以本基址替换 issuer 的 scheme+host+port
//	  抓取 discovery/JWKS，iss 校验仍用 issuer 字符串。
func NewJWKSVerifier(ctx context.Context, issuer, audience, tenantClaim, backchannelBase string) (*JWKSVerifier, error) {
	if issuer == "" {
		return nil, fmt.Errorf("authz: JSL_AUTHZ_ISSUER is required in jwks mode")
	}
	if audience == "" {
		return nil, fmt.Errorf("authz: JSL_AUTHZ_AUDIENCE is required in jwks mode")
	}
	if tenantClaim == "" {
		tenantClaim = "tenant_id"
	}
	if backchannelBase != "" {
		origin, err := url.Parse(backchannelBase)
		if err != nil || origin.Scheme == "" || origin.Host == "" {
			return nil, fmt.Errorf("authz: invalid JSL_AUTHZ_BACKCHANNEL_BASE %q", backchannelBase)
		}
		ctx = oidc.ClientContext(ctx, newBackchannelClient(issuer, backchannelBase))
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

// newBackchannelClient 返回 HTTP client：请求 URL 以 issuer 前缀开头时，
// 把 scheme+host+port 替换为 backchannelBase 对应的 origin，路径/query 原样保留。
// 用途：浏览器面 issuer（http://localhost:9080/...）在容器内不可达，重写为集群内
// 地址（http://apisix:9080/...）；iss 校验仍按原 issuer 字符串。
func newBackchannelClient(issuer, backchannelBase string) *http.Client {
	iss, err := url.Parse(issuer)
	if err != nil || iss.Scheme == "" || iss.Host == "" {
		return http.DefaultClient
	}
	bc, err := url.Parse(backchannelBase)
	if err != nil || bc.Scheme == "" || bc.Host == "" {
		return http.DefaultClient
	}
	rewrite := func(raw string) string {
		if strings.HasPrefix(raw, iss.Scheme+"://"+iss.Host) {
			return bc.Scheme + "://" + bc.Host + raw[len(iss.Scheme+"://"+iss.Host):]
		}
		return raw
	}
	return &http.Client{
		Transport: &backchannelTransport{
			rewrite: rewrite,
			base:     http.DefaultTransport,
		},
	}
}

type backchannelTransport struct {
	rewrite func(string) string
	base    http.RoundTripper
}

func (t *backchannelTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	rewritten := t.rewrite(req.URL.String())
	if rewritten == req.URL.String() {
		return t.base.RoundTrip(req)
	}
	newURL, err := url.Parse(rewritten)
	if err != nil {
		return nil, err
	}
	clone := req.Clone(req.Context())
	clone.URL = newURL
	clone.Host = "" // 让 transport 按 URL 重设 Host
	if clone.Body != nil {
		// Body 是 ReadCloser，Clone 不深拷贝；重设一份可复读的副本避免竞态
		buf, rerr := io.ReadAll(req.Body)
		_ = req.Body.Close()
		if rerr != nil {
			return nil, rerr
		}
		clone.Body = io.NopCloser(bytes.NewReader(buf))
	}
	return t.base.RoundTrip(clone)
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
//	                        JSL_AUTHZ_AUDIENCE / JSL_AUTHZ_TENANT_CLAIM /
//	                        JSL_AUTHZ_BACKCHANNEL_BASE [M19，可选]）
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
			os.Getenv("JSL_AUTHZ_BACKCHANNEL_BASE"),
		)
	default:
		return nil, fmt.Errorf("authz: unknown JSL_AUTHZ_MODE %q (want stub|jwks)", mode)
	}
}
