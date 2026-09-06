// 事务性发件箱中继（ADR-0005 / BP-03 §5，步 18）：
//
//	outbox（业务事务内写入）→ Relay 轮询 → Sink 投递 → 同事务标记 published_at
//
// 语义：至少一次（at-least-once）。投递成功但标记前崩溃会重复投递，
// 消费方按 event_id 幂等去重（BP-03 §5）；Kafka 按聚合键分区保证单聚合内有序。
// Sink 为接口抽象：KafkaSink（生产）与测试替身可互换。
package data

import (
	"context"
	"log/slog"
	"time"

	"github.com/twmb/franz-go/pkg/kgo"
	"google.golang.org/protobuf/encoding/protojson"

	eventsv1 "github.com/jsl-aiot/platform/api/events/v1"
	"github.com/jsl-aiot/platform/pkg/dataaccess"
	"github.com/jsl-aiot/platform/pkg/errors"
)

// 消息码。
const (
	CodeOutboxPublishFailed    = "platform.outbox_publish_failed"
	CodeOutboxRelayStopTimeout = "platform.outbox_relay_stop_timeout"
)

// Sink 事件投递口（中继依赖抽象，测试替身可替换）。
type Sink interface {
	Publish(ctx context.Context, env *eventsv1.EventEnvelope) error
}

// KafkaSink 基于 franz-go 的同步投递实现（生产语义：acks=all，按 key 分区）。
type KafkaSink struct {
	client *kgo.Client
	topic  string
}

// NewKafkaSink 建立 Kafka 生产者。
func NewKafkaSink(brokers []string, topic string) (*KafkaSink, error) {
	cl, err := kgo.NewClient(
		kgo.SeedBrokers(brokers...),
		kgo.ClientID("tenant-bc-outbox-relay"),
		kgo.ProduceRequestTimeout(10*time.Second),
		// franz-go 默认禁止生产端自动建 topic（与 Java 客户端默认相反）；
		// 生产环境 topic 由 Strimzi 预建，此处开启仅为 dev compose 便利。
		kgo.AllowAutoTopicCreation(),
	)
	if err != nil {
		return nil, errors.Wrap(err, CodeOutboxPublishFailed, 500)
	}
	return &KafkaSink{client: cl, topic: topic}, nil
}

// Close 释放生产者（等待在途记录完成投递）。
func (s *KafkaSink) Close() { s.client.Close() }

// Publish 同步投递事件信封到 Kafka。
//
//	key = 聚合 ID（site → org → tenant 优先取用，BP-03 §5 按聚合分区）；
//	value = EventEnvelope protojson；
//	headers 携带路由与追踪元数据，消费方无需解码载荷即可分流。
func (s *KafkaSink) Publish(ctx context.Context, env *eventsv1.EventEnvelope) error {
	value, err := protojson.Marshal(env)
	if err != nil {
		return errors.Wrap(err, CodeOutboxPublishFailed, 500)
	}
	rec := &kgo.Record{
		Topic: s.topic,
		Key:   []byte(aggregateKey(env)),
		Value: value,
		Headers: []kgo.RecordHeader{
			{Key: "event_id", Value: []byte(env.GetEventId())},
			{Key: "event_type", Value: []byte(env.GetEventType())},
			{Key: "schema_version", Value: []byte(env.GetSchemaVersion())},
			{Key: "source", Value: []byte(env.GetSource())},
			{Key: "trace_id", Value: []byte(env.GetTraceId())},
			{Key: "content-type", Value: []byte("application/json")},
		},
	}
	if err := s.client.ProduceSync(ctx, rec).FirstErr(); err != nil {
		return errors.Wrap(err, CodeOutboxPublishFailed, 500)
	}
	return nil
}

// aggregateKey 聚合分区键：归属链中最具体的聚合 ID 优先（BP-03 §5 乱序容忍按聚合 ID 分区）。
func aggregateKey(env *eventsv1.EventEnvelope) string {
	switch {
	case env.GetSiteId() != "":
		return env.GetSiteId()
	case env.GetOrgId() != "":
		return env.GetOrgId()
	default:
		return env.GetTenantId()
	}
}

// ---------- 中继轮询 ----------

const (
	selectOutboxBatchSQL = `SELECT event_id, payload FROM outbox
WHERE published_at IS NULL
ORDER BY created_at
LIMIT $1
FOR UPDATE SKIP LOCKED`
	markOutboxPublishedSQL = `UPDATE outbox SET published_at = now() WHERE event_id = ANY($1)`
)

// Relay 发件箱中继：轮询未发布事件并经 Sink 投递，同事务标记防双写不一致。
type Relay struct {
	pool     *dataaccess.Pool
	sink     Sink
	batch    int
	interval time.Duration
	logger   *slog.Logger

	// stop 容量 1 的停止信号（Send 语义天然幂等，无需锁）
	stop chan struct{}
	done chan struct{}
}

// NewRelay 装配中继（batch/interval 非正值取默认：100 条 / 2s）。
func NewRelay(pool *dataaccess.Pool, sink Sink, batch int, interval time.Duration, logger *slog.Logger) *Relay {
	if batch <= 0 {
		batch = 100
	}
	if interval <= 0 {
		interval = 2 * time.Second
	}
	return &Relay{
		pool: pool, sink: sink, batch: batch, interval: interval, logger: logger,
		stop: make(chan struct{}, 1), done: make(chan struct{}),
	}
}

// Run 阻塞轮询直至 Stop 或 ctx 取消（kratos 生命周期适配点）。
func (r *Relay) Run(ctx context.Context) error {
	defer close(r.done)
	t := time.NewTicker(r.interval)
	defer t.Stop()
	for {
		r.pollOnce(ctx)
		select {
		case <-ctx.Done():
			return nil
		case <-r.stop:
			return nil
		case <-t.C:
		}
	}
}

// Stop 优雅停止（幂等；等待轮询循环退出）。
func (r *Relay) Stop(_ context.Context) error {
	select {
	case r.stop <- struct{}{}:
	default:
	}
	select {
	case <-r.done:
		return nil
	case <-time.After(10 * time.Second):
		return errors.New(CodeOutboxRelayStopTimeout, 500)
	}
}

// pollOnce 认领一批未发布事件（SKIP LOCKED 支持多实例并行）：
// 逐条 Sink 投递 → 批量标记 published_at → 提交。
// 投递失败即整体回滚，批次原样保留待下轮重试（至少一次）；
// 解码失败的毒消息标记跳过并记录日志，避免阻塞通道。
func (r *Relay) pollOnce(parent context.Context) {
	ctx, cancel := context.WithTimeout(parent, 30*time.Second)
	defer cancel()

	err := r.pool.AdminTx(ctx, func(ctx context.Context, tx dataaccess.Tx) error {
		rows, err := tx.Query(ctx, selectOutboxBatchSQL, r.batch)
		if err != nil {
			return errors.Wrap(err, "platform.data_query_failed", 500)
		}
		var batch []outboxRow
		for rows.Next() {
			var it outboxRow
			if err := rows.Scan(&it.eventID, &it.payload); err != nil {
				rows.Close()
				return errors.Wrap(err, "platform.data_query_failed", 500)
			}
			batch = append(batch, it)
		}
		rows.Close()
		if err := rows.Err(); err != nil {
			return errors.Wrap(err, "platform.data_query_failed", 500)
		}
		if len(batch) == 0 {
			return nil
		}

		delivered := make([]string, 0, len(batch))
		for _, it := range batch {
			env, err := decodeEnvelope(it.payload)
			if err != nil {
				// 毒消息：立即标记跳过（人工按日志介入），防止死循环阻塞后续事件
				r.logger.Error("outbox poison message (undecodable payload), skipping",
					"event_id", it.eventID, "err", err)
				delivered = append(delivered, it.eventID)
				continue
			}
			if err := r.sink.Publish(ctx, env); err != nil {
				return err // 回滚整批，下轮重试
			}
			delivered = append(delivered, it.eventID)
		}
		if _, err := tx.Exec(ctx, markOutboxPublishedSQL, delivered); err != nil {
			return errors.Wrap(err, "platform.data_event_mark_failed", 500)
		}
		r.logger.Debug("outbox batch delivered", "count", len(delivered))
		return nil
	})
	// 单轮失败仅告警不中断循环（ctx 取消除外）
	if err != nil && parent.Err() == nil {
		r.logger.Warn("outbox relay poll failed, will retry", "err", err)
	}
}

// outboxRow 发件箱行（认领批）。
type outboxRow struct {
	eventID string
	payload []byte
}

// decodeEnvelope 发件箱载荷 → 事件信封（protojson 往返，写侧同格式）。
func decodeEnvelope(payload []byte) (*eventsv1.EventEnvelope, error) {
	env := &eventsv1.EventEnvelope{}
	if err := protojson.Unmarshal(payload, env); err != nil {
		return nil, errors.Wrap(err, CodeOutboxPublishFailed, 500)
	}
	return env, nil
}
