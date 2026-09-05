# ADR-0003：技术栈基线——Go 全栈 + 云中立 K8s + React/Next.js

- 状态：**Accepted**
- 日期：2026-09-05
- 关联：[ADR-0001](./0001-platform-positioning-and-architecture-principles.md)、[BP-04 技术架构](../blueprint/04-technology-architecture.md)

## 背景（Context）

平台进入技术落地准备阶段，需锁定长期技术方向。约束来自既有决策：多租户 SaaS（BP-01）、18 个限界上下文与事件契约（BP-02）、T1–T3 隔离与 RLS（BP-03）、沙箱与零信任（BP-05）、长期企业运营要求成本可控（八项基线）。

候选方案涉及三个长期承诺：后端语言栈、部署与云环境、前端栈。一旦选定，迁移成本极高。

## 决策（Decision）

1. **后端：Go 全栈**
   - IoT 接入、业务域、AI 编排、插件运行时统一 Go；内部通信 gRPC（Protobuf 为契约单一事实源）；
   - Python 生态（模型训练/推理框架）**不进核心代码库**，以独立推理后端容器（Triton/ONNX Runtime/vLLM）与 AI 能力插件形态存在（符合 ADR-0001 模型可插拔）。

2. **部署：云中立 Kubernetes，SaaS 为主 + 专有云输出**
   - 所有组件须可在标准 K8s 自托管；公有云托管服务仅作等价替代，不得成为唯一依赖；
   - 医疗/警备大客户的专有云（BP-03 T1）使用同一套 Helm Chart 与离线制品库；
   - 边缘节点以 k3s 为基线（云边 MQTT 桥接，边缘框架细化另立 ADR）。

3. **前端：React + Next.js（TypeScript）**
   - 三门户（租户/运营/开发者）共享组件库；i18n 用 next-intl/react-i18next，配合消息码体系（BP-03 §4.2）。

4. **中间件基线**（详细论证见 BP-04 §3）：
   PostgreSQL（RLS 承载 T3 隔离）、TimescaleDB（时序，与 PG 同栈）、Kafka 类事件总线、Redis、OpenSearch、S3 兼容对象存储、APISIX 网关、Keycloak 类自托管 OIDC、OpenTelemetry + Prometheus/Grafana/Loki/Tempo 可观测栈。

5. **插件沙箱三级策略**：WASM（wazero，热路径协议插件）/ 容器（默认）/ gVisor/Kata（高危按需）——回答 BP-05 开放问题 1。

## 后果（Consequences）

### 正面

- 单栈 Go 显著降低团队认知成本与跨语言运维负担；Go 与 IoT 接入、WASM 沙箱（wazero 同语言嵌入）、云原生生态高度契合；
- 云中立 K8s 同时支撑 SaaS 规模化与专有云交付，避免厂商锁定，医疗/警备合规可落地；
- React/Next.js 生态与 i18n 工具链支撑多门户与国际化基线；
- PG + TimescaleDB 同栈收敛，T3 隔离、时序治理一套实现，运维面小。

### 负面 / 约束

- Go 在复杂企业业务工程化（如成熟 ORM/规则引擎/工作流生态）上需更多自建或选型评估，已在开放问题清单列 ADR 逐项决策；
- AI 推理引入 Python 组件边界管理成本：推理后端容器与核心仓库严格分离，接口仅经 gRPC/模型包；
- 云中立意味着放弃部分云托管便利（如托管消息队列的深度特性），自运维投入更高；
- 三级沙箱增加插件调度复杂度，manifest 需携带隔离级别声明（BP-06 落地）。

### 后续决策依赖

- 服务框架二选一、事件总线产品、时序规模预案、边缘框架、LLM 供应商策略等，见 [BP-04 §9 落地 ADR 清单](../blueprint/04-technology-architecture.md)。
