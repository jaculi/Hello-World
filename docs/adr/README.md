# 架构决策记录（ADR）

本目录记录 JSL 平台的重要架构决策（Architecture Decision Records）。

## 约定

- 格式：[MADR](https://adr.github.io/madr/) 精简风格（Context / Decision / Consequences）。
- 文件名：`NNNN-kebab-case-title.md`，编号递增、**永不复用**。
- 状态流转：`Proposed` → `Accepted`（→ `Superseded by ADR-XXXX` / `Deprecated`）。
- ADR 一旦 Accepted 即不可修改；决策变更通过新 ADR 取代并标注关系。
- 凡影响跨模块结构、技术选型、平台原则的决策，必须留 ADR。

## 索引

| 编号 | 标题 | 状态 | 日期 |
|---|---|---|---|
| [0001](./0001-platform-positioning-and-architecture-principles.md) | 平台定位与核心架构原则 | Accepted | 2026-09-05 |
| [0002](./0002-ai-governance-and-five-factor-gate.md) | AI 治理：数据 AI-Ready 与五因子决策闸门 | Accepted | 2026-09-05 |
| [0003](./0003-technology-stack-baseline.md) | 技术栈基线：Go 全栈 + 云中立 K8s + React/Next.js | Accepted | 2026-09-05 |
| [0004](./0004-go-service-framework.md) | Go 服务框架选型：go-kratos v3 | Accepted | 2026-09-05 |
| [0005](./0005-event-bus-and-schema-management.md) | 事件总线与 Schema 管理：Kafka/Strimzi + Protobuf 契约仓库 | Accepted | 2026-09-05 |
| [0006](./0006-timeseries-scale-contingency.md) | 时序库规模预案：TimescaleDB 基线与触发阈值 | Accepted | 2026-09-05 |
| [0007](./0007-edge-framework-baseline.md) | 边缘框架基线：k3s + MQTT 桥接 | Accepted | 2026-09-05 |
| [0008](./0008-llm-provider-strategy.md) | LLM 供应商策略：混合路由与数据边界 | Accepted | 2026-09-05 |
| [0009](./0009-multi-region-and-identity.md) | 多区域数据驻留与跨区身份 | Accepted | 2026-09-05 |
