package authz

import (
	"context"
	"testing"
	"time"

	"github.com/jsl-aiot/platform/pkg/errors"
	"github.com/jsl-aiot/platform/pkg/tenantcontext"
)

func TestSignVerifyRoundtrip(t *testing.T) {
	secret := []byte("test-secret")
	tok, err := SignStub(secret, Claims{Subject: "u-1", TenantID: "t-1", Roles: []string{"admin"}}, time.Now().Add(time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	got, err := NewStubVerifier(secret).Verify(context.Background(), tok)
	if err != nil {
		t.Fatal(err)
	}
	if got.Subject != "u-1" || got.TenantID != "t-1" || len(got.Roles) != 1 {
		t.Fatalf("claims mismatch: %+v", got)
	}
}

func TestVerifyTampered(t *testing.T) {
	secret := []byte("test-secret")
	tok, _ := SignStub(secret, Claims{Subject: "u-1", TenantID: "t-1"}, time.Now().Add(time.Hour))
	if _, err := NewStubVerifier([]byte("other-secret")).Verify(context.Background(), tok); err == nil {
		t.Fatal("wrong secret must fail")
	}
	if _, err := NewStubVerifier(secret).Verify(context.Background(), tok+"x"); err == nil {
		t.Fatal("tampered signature must fail")
	}
}

func TestVerifyExpired(t *testing.T) {
	secret := []byte("test-secret")
	tok, _ := SignStub(secret, Claims{Subject: "u-1", TenantID: "t-1"}, time.Now().Add(-time.Minute))
	_, err := NewStubVerifier(secret).Verify(context.Background(), tok)
	if e, ok := errors.From(err); !ok || e.Code != CodeTokenInvalid {
		t.Fatalf("expected token_invalid, got %v", err)
	}
}

func TestTenantMismatch(t *testing.T) {
	ctx := tenantcontext.With(context.Background(), tenantcontext.Context{TenantID: "t-1"})
	_, err := bindClaims(ctx, Claims{Subject: "u-1", TenantID: "t-2"})
	if e, ok := errors.From(err); !ok || e.Code != CodeTenantMismatch {
		t.Fatalf("expected tenant_mismatch, got %v", err)
	}
	out, err := bindClaims(ctx, Claims{Subject: "u-1", TenantID: "t-1"})
	if err != nil {
		t.Fatal(err)
	}
	if tc, _ := tenantcontext.From(out); tc.Subject != "u-1" {
		t.Fatalf("subject not bound: %+v", tc)
	}
}
