package authz

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/go-jose/go-jose/v4"
	"github.com/go-jose/go-jose/v4/jwt"
)

// newTestOIDC 启动本地 OIDC 提供者（discovery + JWKS），返回 issuer 与签名私钥。
func newTestOIDC(t *testing.T) (string, *rsa.PrivateKey) {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generate rsa key: %v", err)
	}
	mux := http.NewServeMux()
	var issuer string
	writeJSON := func(w http.ResponseWriter, v any) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(v)
	}
	mux.HandleFunc("/.well-known/openid-configuration", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, map[string]any{
			"issuer":                                issuer,
			"jwks_uri":                              issuer + "/certs",
			"authorization_endpoint":                issuer + "/auth",
			"token_endpoint":                        issuer + "/token",
			"id_token_signing_alg_values_supported": []string{"RS256"},
		})
	})
	mux.HandleFunc("/certs", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, jose.JSONWebKeySet{Keys: []jose.JSONWebKey{{
			Key: &key.PublicKey, KeyID: "test", Algorithm: "RS256", Use: "sig",
		}}})
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	issuer = srv.URL
	return issuer, key
}

// signToken 用给定私钥签发 RS256 JWT（kid=test，与 JWKS 对齐）。
func signToken(t *testing.T, key *rsa.PrivateKey, claims map[string]any) string {
	t.Helper()
	signer, err := jose.NewSigner(
		jose.SigningKey{Algorithm: jose.RS256, Key: key},
		(&jose.SignerOptions{}).WithHeader(jose.HeaderKey("kid"), "test"),
	)
	if err != nil {
		t.Fatalf("new signer: %v", err)
	}
	raw, err := jwt.Signed(signer).Claims(claims).Serialize()
	if err != nil {
		t.Fatalf("serialize token: %v", err)
	}
	return raw
}

func baseClaims(issuer string) map[string]any {
	return map[string]any{
		"iss":       issuer,
		"aud":       []string{"jsl-gateway"},
		"sub":       "user-001",
		"iat":       time.Now().Unix(),
		"exp":       time.Now().Add(time.Hour).Unix(),
		"tenant_id": "01JSM18DEM0TENANT000000001",
		"realm_access": map[string]any{
			"roles": []string{"TENANT_ADMIN", "default-roles-jsl"},
		},
	}
}

func TestJWKSVerifier_Valid(t *testing.T) {
	issuer, key := newTestOIDC(t)
	v, err := NewJWKSVerifier(context.Background(), issuer, "jsl-gateway", "")
	if err != nil {
		t.Fatalf("new verifier: %v", err)
	}
	claims, err := v.Verify(context.Background(), signToken(t, key, baseClaims(issuer)))
	if err != nil {
		t.Fatalf("verify: %v", err)
	}
	if claims.Subject != "user-001" {
		t.Errorf("subject = %q, want user-001", claims.Subject)
	}
	if claims.TenantID != "01JSM18DEM0TENANT000000001" {
		t.Errorf("tenant = %q", claims.TenantID)
	}
	found := false
	for _, r := range claims.Roles {
		if r == "TENANT_ADMIN" {
			found = true
		}
	}
	if !found {
		t.Errorf("roles = %v, want TENANT_ADMIN", claims.Roles)
	}
}

func TestJWKSVerifier_WrongSignature(t *testing.T) {
	issuer, _ := newTestOIDC(t)
	other, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	v, err := NewJWKSVerifier(context.Background(), issuer, "jsl-gateway", "")
	if err != nil {
		t.Fatalf("new verifier: %v", err)
	}
	if _, err := v.Verify(context.Background(), signToken(t, other, baseClaims(issuer))); err == nil {
		t.Fatal("token signed by foreign key must be rejected")
	}
}

func TestJWKSVerifier_Expired(t *testing.T) {
	issuer, key := newTestOIDC(t)
	v, _ := NewJWKSVerifier(context.Background(), issuer, "jsl-gateway", "")
	claims := baseClaims(issuer)
	claims["exp"] = time.Now().Add(-time.Hour).Unix()
	if _, err := v.Verify(context.Background(), signToken(t, key, claims)); err == nil {
		t.Fatal("expired token must be rejected")
	}
}

func TestJWKSVerifier_WrongIssuer(t *testing.T) {
	issuer, key := newTestOIDC(t)
	v, _ := NewJWKSVerifier(context.Background(), issuer, "jsl-gateway", "")
	claims := baseClaims(issuer)
	claims["iss"] = "http://evil.example.com/realms/jsl"
	if _, err := v.Verify(context.Background(), signToken(t, key, claims)); err == nil {
		t.Fatal("token with mismatched issuer must be rejected")
	}
}

func TestJWKSVerifier_WrongAudience(t *testing.T) {
	issuer, key := newTestOIDC(t)
	v, _ := NewJWKSVerifier(context.Background(), issuer, "jsl-gateway", "")
	claims := baseClaims(issuer)
	claims["aud"] = []string{"other-client"}
	if _, err := v.Verify(context.Background(), signToken(t, key, claims)); err == nil {
		t.Fatal("token with wrong audience must be rejected")
	}
}

func TestJWKSVerifier_MissingTenantClaim(t *testing.T) {
	issuer, key := newTestOIDC(t)
	v, _ := NewJWKSVerifier(context.Background(), issuer, "jsl-gateway", "")
	claims := baseClaims(issuer)
	delete(claims, "tenant_id")
	if _, err := v.Verify(context.Background(), signToken(t, key, claims)); err == nil {
		t.Fatal("token without tenant claim must be rejected (fail-closed)")
	}
}

func TestJWKSVerifier_DiscoveryUnavailable(t *testing.T) {
	if _, err := NewJWKSVerifier(context.Background(), "http://127.0.0.1:1/realms/none", "c", ""); err == nil {
		t.Fatal("discovery failure must return error")
	}
}

func TestNewVerifierFromEnv_Stub(t *testing.T) {
	t.Setenv("JSL_AUTHZ_MODE", "stub")
	t.Setenv("JSL_AUTHZ_JWT_SECRET", "unit-secret")
	v, err := NewVerifierFromEnv(context.Background())
	if err != nil {
		t.Fatalf("stub mode: %v", err)
	}
	if _, ok := v.(*StubVerifier); !ok {
		t.Fatalf("want *StubVerifier, got %T", v)
	}
}

func TestNewVerifierFromEnv_JWKS(t *testing.T) {
	issuer, key := newTestOIDC(t)
	t.Setenv("JSL_AUTHZ_MODE", "jwks")
	t.Setenv("JSL_AUTHZ_ISSUER", issuer)
	t.Setenv("JSL_AUTHZ_AUDIENCE", "jsl-gateway")
	v, err := NewVerifierFromEnv(context.Background())
	if err != nil {
		t.Fatalf("jwks mode: %v", err)
	}
	jv, ok := v.(*JWKSVerifier)
	if !ok {
		t.Fatalf("want *JWKSVerifier, got %T", v)
	}
	if _, err := jv.Verify(context.Background(), signToken(t, key, baseClaims(issuer))); err != nil {
		t.Fatalf("verify via env-wired verifier: %v", err)
	}
}

func TestNewVerifierFromEnv_UnknownMode(t *testing.T) {
	t.Setenv("JSL_AUTHZ_MODE", "bogus")
	if _, err := NewVerifierFromEnv(context.Background()); err == nil {
		t.Fatal("unknown mode must return error")
	}
}
