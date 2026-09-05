package audit

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"testing"

	"github.com/jsl-aiot/platform/pkg/tenantcontext"
)

func TestEmitterFillsContext(t *testing.T) {
	var buf bytes.Buffer
	sink := NewLogSink(slog.New(slog.NewJSONHandler(&buf, nil)))
	em := NewEmitter(sink)

	ctx := tenantcontext.With(context.Background(), tenantcontext.Context{TenantID: "t-1", Subject: "u-1"})
	if err := em.Emit(ctx, "tenant.tenant.provision", "t-1", ResultSuccess, nil); err != nil {
		t.Fatal(err)
	}

	var line map[string]any
	if err := json.Unmarshal(buf.Bytes(), &line); err != nil {
		t.Fatal(err)
	}
	if line["audit.tenant_id"] != "t-1" || line["audit.subject"] != "u-1" {
		t.Fatalf("tenant/subject not filled: %v", line)
	}
	if line["audit.action"] != "tenant.tenant.provision" || line["audit.result"] != "success" {
		t.Fatalf("action/result mismatch: %v", line)
	}
	// 系统动作：无主体时兜底 system:unknown
	buf.Reset()
	_ = em.Emit(context.Background(), "job.rollup", "", ResultSuccess, nil)
	_ = json.Unmarshal(buf.Bytes(), &line)
	if line["audit.subject"] != "system:unknown" {
		t.Fatalf("system fallback missing: %v", line)
	}
}
