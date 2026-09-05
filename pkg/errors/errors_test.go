package errors

import (
	stderrors "errors"
	"fmt"
	"net/http"
	"testing"
)

func TestNewAndParams(t *testing.T) {
	e := New("tenant.not_found", http.StatusNotFound).WithParam("id", "t-123")
	if e.Code != "tenant.not_found" || e.HTTPCode != http.StatusNotFound {
		t.Fatalf("unexpected error: %+v", e)
	}
	if e.Params["id"] != "t-123" {
		t.Fatalf("param missing: %+v", e.Params)
	}
}

func TestWrapAndFrom(t *testing.T) {
	cause := fmt.Errorf("connection refused")
	e := Wrap(cause, "platform.storage_unavailable", http.StatusInternalServerError)
	got, ok := From(e)
	if !ok || got.Code != "platform.storage_unavailable" {
		t.Fatalf("From failed: %v, ok=%v", got, ok)
	}
	if !stderrors.Is(e, cause) {
		t.Fatalf("Unwrap chain broken")
	}
}

func TestCodeOfFallback(t *testing.T) {
	if got := CodeOf(fmt.Errorf("boom")); got != "platform.internal" {
		t.Fatalf("fallback code = %q", got)
	}
	if got := CodeOf(New("a.b", 400)); got != "a.b" {
		t.Fatalf("code = %q", got)
	}
}
