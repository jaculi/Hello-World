package authz

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/jsl-aiot/platform/pkg/errors"
)

// 消息码。
const (
	CodeTokenInvalid = "platform.token_invalid"
)

// StubVerifier 本地 HMAC-SHA256 JWT 验签（仅限开发/测试；BC-P2 替换为 JWKS 实现）。
type StubVerifier struct {
	secret []byte
}

func NewStubVerifier(secret []byte) *StubVerifier {
	return &StubVerifier{secret: secret}
}

// Verify 验签 + 过期校验 + 算法锁定（拒绝 none/其他算法，防降级攻击）。
func (v *StubVerifier) Verify(_ context.Context, rawToken string) (Claims, error) {
	parts := strings.Split(rawToken, ".")
	if len(parts) != 3 {
		return Claims{}, errors.New(CodeTokenInvalid, unauthorized)
	}
	if !hmac.Equal(
		[]byte(signStub(v.secret, parts[0]+"."+parts[1])),
		[]byte(parts[2]),
	) {
		return Claims{}, errors.New(CodeTokenInvalid, unauthorized)
	}
	header, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil || !strings.Contains(string(header), `"HS256"`) {
		return Claims{}, errors.New(CodeTokenInvalid, unauthorized)
	}
	var payload struct {
		Sub      string   `json:"sub"`
		TenantID string   `json:"tenant_id"`
		Roles    []string `json:"roles"`
		Exp      int64    `json:"exp"`
	}
	raw, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil || json.Unmarshal(raw, &payload) != nil {
		return Claims{}, errors.New(CodeTokenInvalid, unauthorized)
	}
	if payload.Exp > 0 && time.Now().Unix() > payload.Exp {
		return Claims{}, errors.New(CodeTokenInvalid, unauthorized)
	}
	return Claims{Subject: payload.Sub, TenantID: payload.TenantID, Roles: payload.Roles}, nil
}

const unauthorized = 401

// SignStub 签发开发/测试用令牌（与 StubVerifier 配对使用）。
func SignStub(secret []byte, c Claims, exp time.Time) (string, error) {
	if c.Subject == "" {
		return "", fmt.Errorf("authz: subject required")
	}
	header := base64.RawURLEncoding.EncodeToString([]byte(`{"alg":"HS256","typ":"JWT"}`))
	payloadJSON, err := json.Marshal(map[string]any{
		"sub":       c.Subject,
		"tenant_id": c.TenantID,
		"roles":     c.Roles,
		"exp":       exp.Unix(),
	})
	if err != nil {
		return "", err
	}
	payload := base64.RawURLEncoding.EncodeToString(payloadJSON)
	return header + "." + payload + "." + signStub(secret, header+"."+payload), nil
}

func signStub(secret []byte, signingInput string) string {
	mac := hmac.New(sha256.New, secret)
	mac.Write([]byte(signingInput))
	return base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}
