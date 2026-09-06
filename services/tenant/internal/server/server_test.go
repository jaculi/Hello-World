// HTTP 错误编码器测试：PlatformError JSON 形态与内部信息剥离。
package server

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	kerrors "github.com/go-kratos/kratos/v3/errors"

	platerrors "github.com/jsl-aiot/platform/pkg/errors"
)

func TestEncodePlatformError(t *testing.T) {
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/v1/tenants/t1", nil)
	err := platerrors.New("platform.tenant.not_found", http.StatusNotFound).
		WithParam("tenant_id", "t1")

	encodePlatformError(w, r, err)

	if w.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", w.Code)
	}
	body := w.Body.String()
	for _, want := range []string{`"code":404`, `"message_code":"platform.tenant.not_found"`, `"tenant_id":"t1"`} {
		if !strings.Contains(body, want) {
			t.Fatalf("body %q missing %s", body, want)
		}
	}
}

func TestEncodeInternalErrorStripped(t *testing.T) {
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/", nil)
	// 非平台错误（含敏感内部原因）→ 500 且原因文本不得外泄
	err := errors.New("connection refused to 10.0.0.1:5432")

	encodePlatformError(w, r, err)

	if w.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500", w.Code)
	}
	body := w.Body.String()
	if strings.Contains(body, "10.0.0.1") {
		t.Fatalf("内部信息泄漏: %s", body)
	}
	if !strings.Contains(body, `"message_code":"platform.internal"`) {
		t.Fatalf("body %q missing platform.internal", body)
	}
}

func TestEncodeFrameworkError(t *testing.T) {
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, "/v1/tenants", nil)
	err := kerrors.BadRequest("CODEC", "body unmarshal failed")

	encodePlatformError(w, r, err)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", w.Code)
	}
	if !strings.Contains(w.Body.String(), `"message_code":"CODEC"`) {
		t.Fatalf("框架错误 reason 透传缺失: %s", w.Body.String())
	}
}
