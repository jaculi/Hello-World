// 套餐目录与事务性发件箱的 SQL 实现。
package data

import (
	"context"

	"google.golang.org/protobuf/encoding/protojson"

	eventsv1 "github.com/jsl-aiot/platform/api/events/v1"
	"github.com/jsl-aiot/platform/pkg/errors"

	"github.com/jsl-aiot/platform/services/tenant/internal/biz"
)

// PlanRepo 套餐目录（平台全局读模型，无 RLS）。
type PlanRepo struct{}

const selectPlanByIDSQL = `SELECT plan_id, name, default_isolation, active FROM plans WHERE plan_id = $1`

// GetByID 查询套餐。
func (PlanRepo) GetByID(ctx context.Context, tx biz.Tx, id string) (*biz.Plan, error) {
	var p biz.Plan
	var isolation string
	err := tx.QueryRow(ctx, selectPlanByIDSQL, id).Scan(&p.ID, &p.Name, &isolation, &p.Active)
	if errNoRows(err) {
		return nil, errNotFound(biz.CodePlanNotFound, "plan_id", id)
	}
	if err != nil {
		return nil, errors.Wrap(err, "platform.data_query_failed", 500)
	}
	p.DefaultIsolation = biz.IsolationLevel(isolation)
	return &p, nil
}

// Outbox 事务性发件箱写入口（ADR-0005：与业务同事务，失败即整体回滚）。
// relay 侧（步 18）轮询 published_at IS NULL 投递 Kafka 后标记。
type Outbox struct{}

const insertOutboxSQL = `INSERT INTO outbox (event_id, event_type, tenant_id, payload)
VALUES ($1, $2, $3, $4)
ON CONFLICT (event_id) DO NOTHING`

// Enqueue 事件信封写入发件箱（payload 为完整 EventEnvelope JSON）。
func (Outbox) Enqueue(ctx context.Context, tx biz.Tx, env *eventsv1.EventEnvelope) error {
	payload, err := protojson.Marshal(env)
	if err != nil {
		return errors.Wrap(err, "platform.data_event_marshal_failed", 500)
	}
	if _, err := tx.Exec(ctx, insertOutboxSQL,
		env.GetEventId(), env.GetEventType(), env.GetTenantId(), payload,
	); err != nil {
		return errors.Wrap(err, "platform.data_event_enqueue_failed", 500)
	}
	return nil
}
