# BP-04 技术架构（Technology Architecture）

> 状态：**v0.1**（2026-09-05）
> 上游依据：[BP-01](./01-business-architecture.md)、[BP-02 应用架构](./02-application-architecture.md)（BC 与部署边界）、[BP-03 数据架构](./03-data-architecture.md)（存储形态与隔离级别）、[BP-05 安全架构](./05-security-architecture.md)（沙箱/密钥/网络分区约束）、[ADR-0001](../adr/0001-platform-positioning-and-architecture-principles.md)、[ADR-0002](../adr/0002-ai-governance-and-five-factor-gate.md)、[ADR-0003 技术栈基线](../adr/0003-technology-stack-baseline.md)
> 下游消费者：[BP-06 集成与生态](./06-integration-and-ecosystem.md)、[BP-07 部署与运营](./07-deployment-and-operations.md)

## 1. 目的与范围

本文基于 BP-02/03/05 的架构约束，确定平台的技术栈基线与关键运行时设计：

1. 技术原则与技术栈基线总表；
2. IoT 接入运行时、插件沙箱运行时、AI 能力层运行时；
3. API 网关、数据访问与租户路由的实现机制；
4. 可观测性、部署拓扑、CI/CD 与成本容量。

硬约束（继承既有文档）：

- **云中立**：一切选型须可在标准 Kubernetes 上自托管，专有云输出用同一套制品（ADR-0003）；
- **单栈优先**：后端统一 Go；模型推理等 Python 生态组件**不进核心代码库**，以独立推理后端/插件形态存在；
- 选型须满足 BP-03 隔离（T1/T2/T3 路由）与 BP-05 安全基线（mTLS、RLS、沙箱、fail-closed 闸门）；
- 具体产品版本与参数在落地时经 ADR 锁定（本文只定方向与选型，不锁版本）。

## 2. 技术原则

1. **K8s 基座，云中立**：所有有状态/无状态组件须可自托管；公有云托管服务只允许作为"等价替代"，不得成为唯一依赖；
2. **单栈 Go**：降低团队认知与运维成本；以 gRPC 为内部通信标准；
3. **开源优先、可替换**：每个中间件必须有备选与迁移预案（数据访问层收口，见 §5.3）；
4. **模型可插拔**：AI 能力以"推理后端 + 模型包"接入（ADR-0001），平台不含训练/建模代码；
5. **成本即指标**：资源消耗全部计量（联动 BC-P3），GPU/存储/流量有预算与告警（八项基线之"长期企业运营"）。

## 3. 技术栈基线总表

### 3.1 语言与框架

| 层 | 首选 | 备选 | 说明 |
|---|---|---|---|
| 后端语言 | **Go**（全栈统一） | — | IoT 接入、业务域、AI 编排、插件运行时全部 Go |
| 服务框架 | gRPC + ConnectRPC；骨架用 go-kratos 或 go-zero（落地 ADR 二选一） | 标准库 + chi/gin（简单 BC） | 统一中间件：租户上下文、鉴权、审计、限流、消息码 |
| API 契约 | Protobuf（单一事实源，生成 Go/TS 客户端） | OpenAPI（对外开放 API 用） | 内部 gRPC / 对外 REST，经网关转换 |
| 前端 | **React + Next.js**（TypeScript） | — | 三门户共库组件；i18n 用 next-intl/react-i18next；大屏与移动端复用组件层 |
| AI 推理生态 | Python 仅存在于**推理后端容器**（Triton/vLLM 等），不进核心仓库 | ONNX Runtime | 与"模型可插拔"原则一致 |

### 3.2 数据与中间件（对应 BP-03 §2.2 存储形态）

| 形态 | 首选 | 备选 | 决策理由 |
|---|---|---|---|
| 关系库 | **PostgreSQL** | MySQL（国内生态备选） | RLS 原生支持 T3 行级隔离；JSONB 兼文档场景；单栈收敛运维 |
| 时序库 | **TimescaleDB**（PG 扩展） | TDengine / InfluxDB（超大单租户规模备选） | 与关系库同栈，租户分桶/RLS/TTL/降采样一套实现；规模触顶再引入专用时序库（ADR） |
| 事件总线 | **Kafka（类）** | NATS JetStream（轻量场景）/ RocketMQ（国内备选） | 持久日志、回放、分区按聚合键；自托管方案成熟（Strimzi 类） |
| 缓存 | **Redis（类）** | Dragonfly | 会话/影子热点/字典；键强制租户前缀（BP-05 §5） |
| 搜索 | **OpenSearch** | Elasticsearch | Apache 2.0 许可，云中立更干净；主数据/工单/警情检索 |
| 对象存储 | S3 兼容（私有 MinIO / 公有云 OSS） | — | D6 媒体、冷数据 Parquet（BP-03 §6） |
| 特征/数据集 | PG + 对象存储（Parquet）起家 | 专用特征库（规模触发再评估） | 避免过早引入组件；血缘与版本自管（BP-03 §8.2） |

### 3.3 平台组件

| 组件 | 首选 | 备选 | 说明 |
|---|---|---|---|
| 容器基座 | **Kubernetes** | — | 云中立；专有云输出同一套 Helm Chart |
| API 网关 | **APISIX** | Kong / Envoy 系（Higress） | 开放 API 认证/限流/计量/租户上下文注入；插件生态好 |
| MQTT 接入 | **EMQX（类集群）** | NanoMQ / Mosquitto（边缘） | 承接 D1 主通道；与 BC-I1 经桥接/钩子集成 |
| 身份服务 | **Keycloak（自托管 OIDC）** | Casdoor / 云 IAM 等价替代 | 五类主体认证（BP-05 §3）；OIDC 联合登录企业 IdP |
| 沙箱运行时 | 见 §4.2 三级方案 | — | 回答 BP-05 开放问题 1 |
| CI/CD | GitLab CI 或 GitHub Actions + **ArgoCD（GitOps）** | Jenkins | 环境分层、制品签名（BP-05 §7.1） |
| 可观测 | **OpenTelemetry + Prometheus + Grafana + Loki + Tempo** | 云厂商 APM 等价 | 云中立标准栈；全部租户维度化 |

## 4. 关键运行时设计

### 4.1 IoT 接入运行时（BC-I1/I2/I3）

```
设备 → 边缘网关(k3s，可选) → 接入集群：
  MQTT 集群(EMQX 类) ──桥接──▶ BC-I1 接入服务(Go, 无状态水平扩)
  其他协议(HTTP/Modbus/OPC-UA) → BC-I1 协议适配层
        └─ 协议插件(WASM, §4.2) 热加载
BC-I1 → 事件总线 → BC-I3 写入管道(质量评分/降采样) → TimescaleDB
影子/OTA：BC-I2 读 Redis 热点 + PG 权威态
```

- 接入服务**无状态**，连接态在 MQTT 集群，扩容只加副本；
- 边缘侧：k3s 轻量基座跑协议适配与本地规则（断网续传），云边经 MQTT 桥接；KubeEdge/OpenYurt 作为云边管理备选（落地 ADR）；
- 容量模型按"连接数 × 消息频率"预估（联动 BP-07 容量规划）。

### 4.2 插件沙箱运行时（BC-E1）——三级隔离策略

| 级别 | 技术 | 适用插件 | 理由 |
|---|---|---|---|
| W 级（WASM） | **wazero**（Go 原生嵌入） | 协议适配、轻量数据加工等热路径插件 | 微秒级启动、资源占用极小、天然无网络/文件系统，宿主注入能力 |
| C 级（容器） | K8s Pod + NetworkPolicy + 出口代理（默认级） | 行业模块、业务功能、AI 能力插件 | 隔离与工程成熟度平衡；配额用 cgroup/ResourceQuota |
| S 级（强隔离） | gVisor / Kata Containers（按需启用） | 高不可信第三方插件、处理 S4 数据的插件 | 内核级隔离；性能有损，仅声明式启用 |

决策：**默认 C 级，热路径 W 级，高危 S 级**——manifest 按 BP-05 §7.3 声明所需隔离级别，BC-E1 校验三级权限天花板后调度。WASM 运行时与 Go 栈同语言，是本基线下的差异化优势。

### 4.3 AI 能力层运行时（BC-A1/A2/A3）

```
调用方 BC → BC-A2 闸门(Go, fail-closed, 无状态多副本)
              └→ BC-A1 编排(Go) → 推理后端池（独立容器，不进核心仓库）：
                    ├ 通用模型: Triton / ONNX Runtime（视觉/时序/异常检测）
                    ├ LLM: vLLM 推理池 + LLM 网关（多供应商路由/配额/脱敏前置）
                    └ 第三方模型包: 经 AI Capability SPI 注册, 按隔离级别调度(W/C/S)
GPU: K8s Device Plugin 调度；租户级配额；按用量计量入 BC-P3
特征服务: BC-A3 读 PG/Redis 特征表（BP-03 §8.2），热特征 Redis 化
```

- 闸门为**关键路径**：多副本 + 降级预案（闸门不可用 → 全部按 L3 辅助放行"只读洞察"，拒绝自动执行，即 fail-closed）；
- LLM 网关职责：供应商路由、Token 计量、Prompt 注入防护挂点（BP-05 §8）、数据脱敏前置；
- 模型包版本化管理（BC-A1 资产管理），支持灰度与回滚。

### 4.4 API 网关与 BFF

- APISIX 插件链：认证（OIDC 令牌校验）→ 粗粒度授权 → 套餐/限流 → **租户上下文注入**（`tenant/org/site/trace`）→ 计量埋点 → 消息码 i18n 渲染 → 路由；
- BFF（Go，按门户三个实例）聚合 BC 接口，产出前端视图；对内 gRPC、对外 REST/GraphQL 可选（开放 API 以 REST 为主，BP-06）。

### 4.5 数据访问层与租户路由（落实 BP-03 §3）

- Go 统一数据访问库：DAO 自动注入 `tenant_id` 过滤 + RLS 会话变量绑定；禁止裸 SQL 逃逸（代码规约 + lint 门禁）；
- **路由表热更新**：BC-P1 登记租户隔离级别与数据单元映射 → 访问层按路由表分发（T1 连接串 / T2 schema / T3 RLS）；
- 时序与对象存储同理由路由表租户化（分桶/前缀）。

## 5. 可观测性

| 支柱 | 标准 | 租户维度 |
|---|---|---|
| 指标 | Prometheus + OTel SDK | 核心指标带 `tenant_id/site_id` 标签（高基数指标限白名单，防爆炸） |
| 日志 | 结构化 JSON → Loki | 强制租户/trace 字段；S3/S4 数据禁止入日志（BP-05 §6.3） |
| 追踪 | OTel → Tempo | 网关注入 trace_id 全链路（含插件调用、AI 推理、闸门裁决） |
| 业务观测 | 事件总线指标（各事件吞吐/延迟/积压） | 按租户/事件类型聚合 |

SLO 基线（v0.1，正式值由 BP-07 定）：接入层可用性 99.9%、闸门 P99 < 50ms、开放 API P99 < 500ms、遥测端到端延迟 P95 < 2s。

## 6. 部署拓扑

```
SaaS 公有云（主）：
  公网区(APISIX/WAF) → 接入区(BFF/门户) → 核心区(BC 服务/事件总线) → 数据区(PG/时序/缓存/对象)
  插件区（独立节点池，跑 W/C/S 沙箱）│ GPU 节点池（AI 推理后端）
专有云输出（医疗/警备 T1 客户）：
  同一套 Helm Chart + 离线制品库；规模裁剪（可单集群）；BYOK 密钥；运维代理通道可选
边缘节点：k3s（协议适配/本地规则/断网续传），云边 MQTT 桥接
多区域（后续 BP-07）：数据驻留按区域化集群实现，区域间不跨区存数据
```

- 制品链：源码 → CI 构建/扫描/签名 → 制品库 → ArgoCD 按环境 GitOps 发布（dev/staging/prod/专有云通道）；
- 发布策略：BC 滚动发布；事件契约按 BP-03 §5.2 兼容规则双写过渡；插件上架独立于平台发版。

## 7. 成本与容量

- 全资源计量入 BC-P3（算力/GPU/存储/流量/AI Token），内部成本可见，对外支撑计费；
- GPU 成本策略：通用模型共享池 + 租户配额；大客户可买专用池（套餐项）；
- 存储成本：时序冷热分层（BP-03 §6）+ 对象生命周期规则自动化；
- 容量规划：以"设备连接数/消息速率/AI 调用峰值"三指标驱动压测基线（BP-07 承接）。

## 8. 对下游的输入

| 消费方 | 承接内容 |
|---|---|
| BP-06 集成与生态 | 网关插件链细节、SPI 调用通道（WASM ABI/RPC）、市场制品签名流程 |
| BP-07 部署与运营 | 部署拓扑与区域化、容量三指标、SLO 正式值、成本核算口径 |

## 9. 开放问题 → 落地 ADR 清单（2026-09-05 全部闭环）

| 编号计划 | 决策点 | 结论 |
|---|---|---|
| ✅ | Go 服务框架 | go-kratos v3（[ADR-0004](../adr/0004-go-service-framework.md)） |
| ✅ | 事件总线产品与 Schema 管理 | Kafka/Strimzi + Protobuf 契约仓库 + buf CI 门禁（[ADR-0005](../adr/0005-event-bus-and-schema-management.md)） |
| ✅ | 时序库规模预案 | TimescaleDB 基线 + 专用库触发阈值与迁移路径（[ADR-0006](../adr/0006-timeseries-scale-contingency.md)） |
| ✅ | 边缘框架 | k3s + MQTT 桥接，KubeEdge/OpenYurt 设重估条件（[ADR-0007](../adr/0007-edge-framework-baseline.md)） |
| ✅ | LLM 供应商策略 | 混合路由：自托管 vLLM（敏感）+ 外部 API（非敏感，脱敏前置+同区域）（[ADR-0008](../adr/0008-llm-provider-strategy.md)） |
| ✅ | 多区域数据驻留与跨区身份 | 区域独立数据面 + 区域独立 Keycloak 联合登录 + 每区独立 KMS（[ADR-0009](../adr/0009-multi-region-and-identity.md)） |
