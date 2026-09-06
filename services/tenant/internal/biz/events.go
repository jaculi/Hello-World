// 事件装配：统一信封组装与载荷构造（ADR-0005）。
package biz

import (
	"context"
	"fmt"

	"github.com/oklog/ulid/v2"
	"go.opentelemetry.io/otel/trace"
	"google.golang.org/protobuf/types/known/timestamppb"

	eventsv1 "github.com/jsl-aiot/platform/api/events/v1"
)

// 事件类型（<域>.<聚合>.<动作>，BP-02 §5.2）。
const (
	EventTenantProvisioned     = "platform.tenant.provisioned"
	EventTenantSuspended       = "platform.tenant.suspended"
	EventTenantReactivated     = "platform.tenant.reactivated"
	EventTenantDeactivated     = "platform.tenant.deactivated"
	EventSiteCreated           = "platform.site.created"
	EventIndustryAssemblyBound = "platform.industry.assembly_bound"
)

const (
	sourceBCP1      = "bc-p1"
	schemaVersionV1 = "v1"
)

// isEnvelopePayload 事件载荷（oneof 包装类型的共同约束由 newEnvelope 内类型开关落地；
// protoc-gen-go 生成的 oneof 接口未导出，跨包仅能以具体包装类型传递）。
type isEnvelopePayload = any

// newEnvelope 组装统一事件信封：ULID 标识、归属链、领域时间、W3C 链路。
func newEnvelope(ctx context.Context, tenantID, eventType string, payload isEnvelopePayload) *eventsv1.EventEnvelope {
	env := &eventsv1.EventEnvelope{
		EventId:       ulid.Make().String(),
		EventType:     eventType,
		SchemaVersion: schemaVersionV1,
		TenantId:      tenantID,
		OccurredAt:    timestamppb.Now(),
		Source:        sourceBCP1,
	}
	switch p := payload.(type) {
	case *eventsv1.EventEnvelope_TenantProvisioned:
		env.Events = p
	case *eventsv1.EventEnvelope_TenantSuspended:
		env.Events = p
	case *eventsv1.EventEnvelope_TenantReactivated:
		env.Events = p
	case *eventsv1.EventEnvelope_TenantDeactivated:
		env.Events = p
	case *eventsv1.EventEnvelope_SiteCreated:
		env.Events = p
	case *eventsv1.EventEnvelope_IndustryAssemblyBound:
		env.Events = p
	default:
		panic(fmt.Sprintf("bizevents: unknown payload %T for %s", payload, eventType))
	}
	if sc := trace.SpanContextFromContext(ctx); sc.HasTraceID() {
		env.TraceId = sc.TraceID().String()
	}
	return env
}

func tenantProvisionedPayload(t *Tenant, rootOrgID, rootSiteID string) isEnvelopePayload {
	return &eventsv1.EventEnvelope_TenantProvisioned{TenantProvisioned: &eventsv1.TenantProvisioned{
		TenantId: t.ID, Name: t.Name, PlanId: t.PlanID, IsolationLevel: string(t.Isolation),
		RootOrgId: rootOrgID, RootSiteId: rootSiteID,
	}}
}

func suspendedPayload(t *Tenant, reason string) isEnvelopePayload {
	return &eventsv1.EventEnvelope_TenantSuspended{TenantSuspended: &eventsv1.TenantSuspended{
		TenantId: t.ID, Reason: reason,
	}}
}

func reactivatedPayload(t *Tenant) isEnvelopePayload {
	return &eventsv1.EventEnvelope_TenantReactivated{TenantReactivated: &eventsv1.TenantReactivated{
		TenantId: t.ID,
	}}
}

func deactivatedPayload(t *Tenant, reason string) isEnvelopePayload {
	return &eventsv1.EventEnvelope_TenantDeactivated{TenantDeactivated: &eventsv1.TenantDeactivated{
		TenantId: t.ID, Reason: reason,
	}}
}

func siteCreatedPayload(s *Site) isEnvelopePayload {
	return &eventsv1.EventEnvelope_SiteCreated{SiteCreated: &eventsv1.SiteCreated{
		SiteId: s.ID, TenantId: s.TenantID, ParentSiteId: s.ParentSiteID, Name: s.Name,
	}}
}

func assemblyBoundPayload(tenantID, moduleID, moduleVersion string) isEnvelopePayload {
	return &eventsv1.EventEnvelope_IndustryAssemblyBound{IndustryAssemblyBound: &eventsv1.IndustryAssemblyBound{
		TenantId: tenantID, ModuleId: moduleID, ModuleVersion: moduleVersion,
	}}
}

// eventTypeOf 迁移目标状态 → 事件类型。
func eventTypeOf(to TenantStatus) string {
	switch to {
	case TenantStatusSuspended:
		return EventTenantSuspended
	case TenantStatusActive:
		return EventTenantReactivated
	default:
		return EventTenantDeactivated
	}
}
