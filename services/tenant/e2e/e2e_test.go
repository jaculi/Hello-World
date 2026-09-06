//go:build e2e

// BC-P1 事件链路端到端验证（步 19）：开通租户 → outbox → Kafka（信封字段完整）。
// 同时充当消费方契约测试预留（ADR-0005）：消费侧只依赖信封与 protojson，不依赖服务内部。
//
// 前置条件：
//  1. docker compose -f deploy/dev/docker-compose.yml up -d
//  2. tenant-bc 以 PG+Kafka 模式运行：
//     JSL_DB_DSN=postgres://jsl:jsl@127.0.0.1:15432/jsl?sslmode=disable \
//     JSL_KAFKA_BROKERS=127.0.0.1:9092 go run ./services/tenant
//
// 用法：go test -tags e2e ./services/tenant/e2e -v
package e2e

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/oklog/ulid/v2"
	"github.com/twmb/franz-go/pkg/kgo"
	"google.golang.org/protobuf/encoding/protojson"

	"github.com/jsl-aiot/platform/pkg/authz"
	eventsv1 "github.com/jsl-aiot/platform/api/events/v1"
)

const (
	provisionedType = "platform.tenant.provisioned"
	siteCreatedType = "platform.site.created"
)

func TestTenantProvisionedEventEndToEnd(t *testing.T) {
	base := envOr("JSL_E2E_HTTP", "http://127.0.0.1:8000")
	brokers := []string{envOr("JSL_E2E_KAFKA", "127.0.0.1:9092")}
	topic := envOr("JSL_E2E_TOPIC", "platform.events.v1")
	secret := envOr("JSL_E2E_JWT_SECRET", "dev-secret-change-me")

	// 1. 平台操作者令牌（验签桩；BC-P2 替换为 JWKS 后此处同步替换）
	tok, err := authz.SignStub([]byte(secret),
		authz.Claims{Subject: "e2e:tester", TenantID: "platform", Roles: []string{"platform.admin"}},
		time.Now().Add(10*time.Minute))
	if err != nil {
		t.Fatalf("sign token: %v", err)
	}

	// 2. 开通租户（创建即 ACTIVE；同事务写 tenant.provisioned + site.created 到 outbox）
	tenantName := strings.ToLower("e2e-" + ulid.Make().String())
	tenantID := createTenant(t, base, tok, tenantName)
	t.Logf("tenant created: name=%s id=%s", tenantName, tenantID)

	// 3. 消费 Kafka：校验信封字段完整性（event_id/schema_version/归属链/occurred_at/trace_id/source）
	cl, err := kgo.NewClient(
		kgo.SeedBrokers(brokers...),
		kgo.ConsumerGroup(fmt.Sprintf("e2e-%d", time.Now().UnixNano())),
		kgo.ConsumeTopics(topic),
		kgo.ConsumeStartOffset(kgo.NewOffset().AtStart()),
		kgo.DisableAutoCommit(), // 只读验证，无需提交位点
	)
	if err != nil {
		t.Fatalf("kafka consumer: %v", err)
	}
	defer cl.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	found := map[string]*eventsv1.EventEnvelope{}
	for len(found) < 2 {
		select {
		case <-ctx.Done():
			t.Fatalf("timeout waiting events: got %v, want [%s %s]", keys(found), provisionedType, siteCreatedType)
		default:
		}
		fetches := cl.PollRecords(ctx, 100)
		fetches.EachError(func(_ string, _ int32, err error) {
			t.Fatalf("kafka fetch error: %v", err)
		})
		fetches.EachRecord(func(rec *kgo.Record) {
			env := &eventsv1.EventEnvelope{}
			if err := protojson.Unmarshal(rec.Value, env); err != nil {
				t.Errorf("undecodable envelope: %v", err)
				return
			}
			if env.GetTenantId() != tenantID {
				return // 其他租户/运行的事件
			}
			checkEnvelope(t, rec, env)
			found[env.GetEventType()] = env
		})
	}

	// 4. 载荷校验：开通载荷含根组织/根站点
	p := found[provisionedType].GetTenantProvisioned()
	if p.GetTenantId() != tenantID || p.GetPlanId() != "plan-free" ||
		p.GetRootOrgId() == "" || p.GetRootSiteId() == "" {
		t.Fatalf("provisioned payload incomplete: %+v", p)
	}
	if found[siteCreatedType].GetSiteCreated().GetParentSiteId() != "" {
		t.Fatalf("root site must have no parent: %+v", found[siteCreatedType].GetSiteCreated())
	}
	t.Logf("end-to-end OK: %s + %s delivered via topic %s", provisionedType, siteCreatedType, topic)
}

// checkEnvelope 信封不变式与 Kafka 消息元数据（ADR-0005 / BP-03 §5）。
func checkEnvelope(t *testing.T, rec *kgo.Record, env *eventsv1.EventEnvelope) {
	t.Helper()
	if len(env.GetEventId()) != 26 {
		t.Errorf("event_id not ULID(26): %q", env.GetEventId())
	}
	if env.GetSchemaVersion() != "v1" {
		t.Errorf("schema_version = %q, want v1", env.GetSchemaVersion())
	}
	if env.GetSource() != "bc-p1" {
		t.Errorf("source = %q, want bc-p1", env.GetSource())
	}
	if env.GetOccurredAt() == nil || env.GetOccurredAt().AsTime().IsZero() {
		t.Error("occurred_at missing")
	}
	if env.GetTraceId() == "" {
		t.Error("trace_id missing (cross-domain tracing broken)")
	}
	// 按聚合分区：租户事件 key = tenant_id
	if string(rec.Key) != env.GetTenantId() {
		t.Errorf("kafka key = %q, want tenant_id %q", rec.Key, env.GetTenantId())
	}
	// 头部路由元数据
	hdrs := map[string]string{}
	for _, h := range rec.Headers {
		hdrs[h.Key] = string(h.Value)
	}
	if hdrs["event_id"] != env.GetEventId() || hdrs["event_type"] != env.GetEventType() {
		t.Errorf("kafka headers mismatch: %+v", hdrs)
	}
}

// createTenant 经 HTTP 开通租户并返回租户 ID。
func createTenant(t *testing.T, base, token, name string) string {
	t.Helper()
	body, _ := json.Marshal(map[string]string{"name": name, "plan_id": "plan-free"})
	req, err := http.NewRequest(http.MethodPost, base+"/v1/tenants", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("build request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("x-tenant-id", "platform")
	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("create tenant: %v", err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		t.Fatalf("create tenant: HTTP %d: %s", resp.StatusCode, raw)
	}
	// HTTP 编码命名兼容：protojson 驼峰（tenantId）或 proto 字段名（tenant_id）
	var out map[string]any
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatalf("decode response %s: %v", raw, err)
	}
	for _, k := range []string{"tenantId", "tenant_id"} {
		if v, ok := out[k].(string); ok && v != "" {
			return v
		}
	}
	t.Fatalf("tenant_id missing in response: %s", raw)
	return ""
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func keys(m map[string]*eventsv1.EventEnvelope) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}
