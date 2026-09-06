package data

import (
	"context"
	"errors"
	"testing"

	"google.golang.org/protobuf/encoding/protojson"

	eventsv1 "github.com/jsl-aiot/platform/api/events/v1"
)

func TestAggregateKey(t *testing.T) {
	cases := []struct {
		name string
		env  *eventsv1.EventEnvelope
		want string
	}{
		// 按聚合 ID 分区（BP-03 §5）：最具体的聚合优先
		{"tenant event", &eventsv1.EventEnvelope{TenantId: "t1"}, "t1"},
		{"org event", &eventsv1.EventEnvelope{TenantId: "t1", OrgId: "o1"}, "o1"},
		{"site event", &eventsv1.EventEnvelope{TenantId: "t1", OrgId: "o1", SiteId: "s1"}, "s1"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := aggregateKey(c.env); got != c.want {
				t.Fatalf("aggregateKey = %q, want %q", got, c.want)
			}
		})
	}
}

func TestDecodeEnvelopeRoundTrip(t *testing.T) {
	in := &eventsv1.EventEnvelope{
		EventId:       "01M1TA6QNZ5XD4HR6ZPNTGXRF4",
		EventType:     "platform.tenant.provisioned",
		SchemaVersion: "v1",
		TenantId:      "t1",
		Source:        "bc-p1",
		Events:        &eventsv1.EventEnvelope_TenantProvisioned{TenantProvisioned: &eventsv1.TenantProvisioned{TenantId: "t1", PlanId: "plan-free"}},
	}
	raw, err := protojson.Marshal(in)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	out, err := decodeEnvelope(raw)
	if err != nil {
		t.Fatalf("decodeEnvelope: %v", err)
	}
	if out.GetEventId() != in.GetEventId() || out.GetEventType() != in.GetEventType() ||
		out.GetTenantId() != in.GetTenantId() || out.GetSchemaVersion() != in.GetSchemaVersion() {
		t.Fatalf("envelope fields mismatch: %+v", out)
	}
	if out.GetTenantProvisioned().GetPlanId() != "plan-free" {
		t.Fatalf("payload mismatch: %+v", out.GetTenantProvisioned())
	}
}

func TestDecodeEnvelopeRejectsGarbage(t *testing.T) {
	if _, err := decodeEnvelope([]byte(`{not-json`)); err == nil {
		t.Fatal("expected error for undecodable payload")
	}
}

// fakeSink 测试替身：记录投递并可控失败（failAfter < 0 表示永不失败）。
type fakeSink struct {
	failAfter int
	delivered []*eventsv1.EventEnvelope
}

func (f *fakeSink) Publish(_ context.Context, env *eventsv1.EventEnvelope) error {
	if f.failAfter >= 0 && len(f.delivered) >= f.failAfter {
		return errors.New("kafka down")
	}
	f.delivered = append(f.delivered, env)
	return nil
}

func TestSinkInterface(t *testing.T) {
	// 编译期断言：KafkaSink 与测试替身均满足 Sink 抽象
	var _ Sink = (*KafkaSink)(nil)
	var _ Sink = (*fakeSink)(nil)
}
