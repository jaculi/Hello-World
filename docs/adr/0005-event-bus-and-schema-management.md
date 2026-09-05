# ADR-0005：事件总线与 Schema 管理——Kafka/Strimzi + Protobuf 契约仓库

- 状态：**Accepted**
- 日期：2026-09-05
- 关联：[ADR-0003](./0003-technology-stack-baseline.md)、[ADR-0004](./0004-go-service-framework.md)、[BP-03 §5](../blueprint/03-data-architecture.md)、[BP-04](../blueprint/04-technology-architecture.md)

## 背景（Context）

BP-03 §5 已定事件信封与兼容规则（次版本向后兼容、消费幂等、按聚合分区），BP-02 §5.2 列出 9 条核心事件。首个事件契约发布前需定：总线产品、Schema 管理机制、运行时注册表。

约束：云中立（ADR-0003，不依赖云厂商专属服务）、proto-first（ADR-0004 契约链）、开源优先可替换。

## 决策（Decision）

### 1. 事件总线：Apache Kafka（Strimzi Operator 自托管于 K8s）

- Apache 2.0 许可，与云中立/开源优先原则一致（与 OpenSearch 的选择逻辑相同）；
- 持久日志 + 按聚合 ID 分区保序 + 回放窗口 ≥30d（BP-03 §2.2）；
- Cell 级部署：每 Cell 独立 Kafka 集群（隔离单元一致，BP-07 §2.2），规模小可先共享；
- 备选登记：NATS JetStream（轻量场景/边缘侧汇聚）、RocketMQ（国内托管需求出现时）。

### 2. Schema 管理：Protobuf 契约仓库 + CI 兼容门禁（初期不引入独立 Registry 服务）

- 建立 **api-contracts 契约单仓**：领域事件 `.proto`（含公共信封包：`event_id/tenant_id/org_id/site_id/occurred_at/trace_id`）+ 开放 API proto，全部事件/API 同源（呼应 ADR-0004 契约链）；
- CI 以 **buf breaking** 做向后兼容门禁：次版本只增字段、不改语义、不删必填，违反即拒绝合并（把 BP-03 §5.2 兼容规则工具化）；
- 事件版本用 proto package 语义（`jsl.events.iot.v1`），破坏性变更 = 新包并行（双写过渡，BP-03 §5.2）。

### 3. 运行时 Registry：按需引入，备选 Apicurio

- 初期消费方一律使用生成代码静态消费，无运行时动态解析需求；
- 若插件生态出现"订阅未预知事件"需求，引入 **Apicurio Registry**（Apache 2.0，支持 Protobuf）；
- **不选 Confluent Schema Registry**：社区许可限制与云中立/开源优先原则冲突。

### 4. 消费约定

- 消费方按 `event_id` 幂等去重、按聚合键分区保序、乱序容忍（BP-03 §5.2）；
- 计量、审计、分析等"广域消费者"经独立 consumer group，互不影响位点。

## 重审触发条件

- 单 Cell 持续吞吐 >100 MB/s 或 topic 数量造成运维痛点 → 重估分级集群（遥测专用集群 vs 业务事件集群）；
- 边缘侧事件汇聚延迟不可接受 → 边缘 NATS + 云端 Kafka 桥接评估。

## 后果（Consequences）

**正面**：契约链全栈单一（proto 覆盖 API+事件）；兼容性从"约定"变为"CI 强制"；许可与云中立无瑕疵。
**负面/约束**：自托管 Kafka 运维投入（Strimzi 缓解）；契约仓库成为跨团队协作枢纽，需明确所有权与评审流程；无运行时 Schema 解析，插件消费新事件须走版本发布。
