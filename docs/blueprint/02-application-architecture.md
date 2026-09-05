# BP-02 应用架构（Application Architecture）

> 状态：**v0.1**（2026-09-05）
> 上游依据：[BP-01 业务架构](./01-business-architecture.md)、[ADR-0001 平台定位与核心架构原则](../adr/0001-platform-positioning-and-architecture-principles.md)、[ADR-0002 AI 治理五因子闸门](../adr/0002-ai-governance-and-five-factor-gate.md)
> 下游消费者：[BP-03 数据架构](./03-data-architecture.md)、[BP-04 技术架构](./04-technology-architecture.md)、[BP-05 安全架构](./05-security-architecture.md)、[BP-06 集成与生态](./06-integration-and-ecosystem.md)

## 1. 目的与范围

本文将 BP-01 的业务能力地图映射为 **DDD 限界上下文（Bounded Context）**，定义：

1. 应用分层与运行视图；
2. 限界上下文清单与领域模型概要；
3. 上下文映射（Context Map）与集成模式；
4. 服务边界与通信契约（同步 API / 异步事件）；
5. 行业模块插件的装配机制与挂载点（SPI）。

硬约束（来自 ADR-0001/0002 与八项基线）：

- 模块间**只允许通过 API 或事件交互，禁止跨上下文共享库表**；
- 所有跨上下文调用必须携带**租户上下文**并经 IAM 鉴权；
- 所有 AI 调用必须经过 **AI Policy 闸门**，不得绕行；
- 对外报文使用**消息码 + 参数**，由网关/前端完成 i18n 渲染；
- 行业差异只经插件 SPI 挂载，行业模块不得修改内核代码。

## 2. 应用分层与运行视图

```
┌────────────────────────────────────────────────────────────────────┐
│ 前端应用：租户门户 │ 平台运营门户 │ 开发者门户 │ 移动端/大屏            │
├────────────────────────────────────────────────────────────────────┤
│ 开放 API 网关：认证 │ 鉴权 │ 限流 │ 计量 │ 路由 │ 版本 │ i18n 渲染    │
├────────────────────────────────────────────────────────────────────┤
│ BFF 层（按门户划分）：聚合领域服务，裁剪视图                          │
├────────────────────────────────────────────────────────────────────┤
│ 领域服务层（限界上下文 = 部署边界候选，见 §3）：                      │
│   平台基座 BC │ IoT 域 BC │ Business 域 BC │ 安全运营域 BC │ AI 域 BC│
├────────────────────────────────────────────────────────────────────┤
│ 横向基础设施：事件总线 │ 消息码/i18n │ 租户上下文传播 │ 可观测性       │
├────────────────────────────────────────────────────────────────────┤
│ 插件运行时（沙箱进程/容器）：行业模块插件 │ 协议插件 │ AI 能力插件      │
└────────────────────────────────────────────────────────────────────┘
```

要点：

- **限界上下文 ≠ 微服务**：BC 是逻辑边界；部署时可将低流量 BC 合并部署（最终由 BP-04 决定）。
- **租户上下文传播**：网关解析身份后注入 `tenant_id / org_id / site_id / trace_id`，全链路透传；每个 BC 自行做租户范围校验（纵深防御）。
- 插件运行时与领域服务**进程隔离**，通过受控契约（SPI / 事件）交互，保障沙箱与配额（BP-05 细化）。

## 3. 限界上下文清单

### 3.1 平台基座域

| BC | 名称 | 聚合/关键实体概要 | 说明 |
|---|---|---|---|
| BC-P1 | 租户与站点 | Tenant / Organization / **Site** / Subscription / Plan | 租户生命周期、行业模板装配记录、站点树；全平台归属之锚 |
| BC-P2 | 身份与权限 IAM | User / ServiceAccount / Role / Permission / ApiKey | 人、服务、插件、API Key 统一认证授权；数据权限策略（行级/字段级）|
| BC-P3 | 计量与计费 | MeterRecord / Plan / Invoice | 采集各 BC 用量事件（设备/AI/API/存储），出账单；向 AI Policy 提供成本因子 |
| BC-P4 | 通知与消息 | NotificationTemplate / Channel / Message | 多渠道投递（站内/邮件/短信/Webhook），消息码模板化支撑 i18n |
| BC-P5 | 审计 | AuditRecord | 全平台操作与裁决审计归集（含 AI 裁决留痕），只追加不可改 |

### 3.2 IoT 设备域

| BC | 名称 | 聚合/关键实体概要 | 说明 |
|---|---|---|---|
| BC-I1 | 设备接入 | Connection / ProtocolAdapter(插件) | 多协议接入、设备认证、连接管理；**协议插件挂载点** |
| BC-I2 | 设备管理 | Device / DeviceShadow / Firmware / DeviceGroup | 注册、影子（期望态/上报态）、OTA、生命周期与故障 |
| BC-I3 | 遥测与数据质量 | TelemetryStream / QualityScore | 时序数据接收与治理，输出数据质量评分（供 AI 五因子） |
| BC-I4 | 规则与告警 | RuleSet / Scene / Alert | 规则引擎、场景联动、告警分级与降噪（降噪可委托 AI） |

### 3.3 Business 业务域

| BC | 名称 | 聚合/关键实体概要 | 说明 |
|---|---|---|---|
| BC-B1 | 主数据 | Space(站点子结构) / Asset / Person / Product | 行业中性建模；**站点子类型扩展点**（行业模块扩展 Space） |
| BC-B2 | 工作流与工单 | ProcessDef / Ticket / Task / Approval | 告警/建议 → 工单 → 处置 → 复盘的主干载体；**业务功能插件挂载点** |
| BC-B3 | 运营分析 | Dashboard / Report / Kpi | 驾驶舱与报表（读侧聚合，跨域经事件物化视图，不直连他域库） |

### 3.4 安全运营域

| BC | 名称 | 聚合/关键实体概要 | 说明 |
|---|---|---|---|
| BC-S1 | 视频监控 | Channel / RecordingPlan / Stream | 通道接入、录像计划、取流分发；视频分析委托 AI 能力层 |
| BC-S2 | 警情与事件 | Alarm(警情) / Incident / DisposalSop / Evidence | 警情分级、处置 SOP、应急联动编排、证据链留痕；**处置策略扩展点** |
| BC-S3 | 巡更 | PatrolPlan / Checkpoint / PatrolRecord | 计划、打卡、漏检稽核 |

### 3.5 AI 横向能力层

| BC | 名称 | 聚合/关键实体概要 | 说明 |
|---|---|---|---|
| BC-A1 | AI 能力服务 | ModelService / ModelVersion / InferenceJob | 模型服务目录与推理调度（视觉/预测/异常/LLM）；**AI 能力插件注册点**；统一计量上报 |
| BC-A2 | AI 治理（Policy） | AiScenario / PolicyRule(五因子) / AutomationLevel(L1/L2/L3) / GateDecision | **所有 AI 调用的强制闸门**（ADR-0002）；场景登记、因子评估、裁决与降级、自动化编排、审计留痕 |
| BC-A3 | 数据就绪 | FeatureSet / Dataset / LabelTask | 特征加工、标注、数据集管理（AI-Ready by Default 的执行体；与 BP-03 数据平台协同） |

### 3.6 生态域

| BC | 名称 | 聚合/关键实体概要 | 说明 |
|---|---|---|---|
| BC-E1 | 插件运行时 | Plugin / PluginInstance / ExtensionPoint / Quota | 插件生命周期（注册/审核/上架/启停/升级）、沙箱与资源配额；**统一 SPI 归口** |
| BC-E2 | 应用市场 | Listing / Subscription / RevenueShare | 上架审核、租户订阅安装、分成结算（对接 BC-P3） |

> API 网关与开发者门户是基础设施/门户组件，不设为 BC；开发者门户的应用侧流程归 BC-E2。

## 4. 上下文映射（Context Map）

```
                    ┌──────────────┐
                    │ BC-P1 租户站点 │（归属上游：一切资源挂 Tenant/Site）
                    └──────┬───────┘
        ┌──────────────────┼─────────────────────────────┐
        ▼                  ▼                             ▼
┌──────────────┐   ┌──────────────┐              ┌──────────────┐
│ BC-P2 IAM    │   │ BC-B1 主数据  │              │ BC-P3 计量计费 │
│ （鉴权上游，  │   │ （Space/Asset │              │ （用量事件下游）│
│  全 BC conformist）│  被多域消费） │              └──────────────┘
└──────────────┘   └──────┬───────┘
                          ▼
 IoT 域：BC-I1 接入 → BC-I2 设备管理 → BC-I3 遥测 → BC-I4 规则告警
                        （事件流）        │ 质量评分        │ 告警/检测请求
                                          ▼                ▼
              BC-A3 数据就绪 ◀────  BC-A2 AI 治理（闸门）▶ BC-A1 AI 能力服务
                                          │ 裁决（L1/L2/L3）
                        ┌─────────────────┼──────────────────┐
                        ▼                 ▼                  ▼
              BC-B2 工单流程       BC-S2 警情事件        BC-B3 运营分析（读）
                        │                 │
                        └──── BC-P4 通知 / BC-P5 审计 ◀（全平台事件）────┐
                                                                          │
 生态域：BC-E1 插件运行时（SPI 归口，挂载到 I1/B1/B2/S2/A1 扩展点）◀─ BC-E2 市场
```

映射模式约定：

| 关系 | 模式 | 说明 |
|---|---|---|
| 各 BC → BC-P2 | **Conformist** | 权限模型全平台统一，不各自造权限 |
| 各 BC → BC-P1 | Conformist | 归属模型（Tenant/Org/Site）统一 |
| 多域 → BC-B1 | **OHS（开放主机服务）** | 主数据以稳定 API 发布；行业扩展经扩展点而非改模型 |
| IoT/AI/Business 之间 | **PL（发布语言）= 领域事件** | 异步解耦，事件契约版本化 |
| BC-A2 闸门 ↔ BC-A1 | 强制中间人 | 任何推理请求必须携带场景与因子输入过闸 |
| 外部系统/第三方接入 | **ACL（防腐层）** | 在网关/集成层转换，不污染内核模型 |
| BC-E1 插件 ↔ 各 BC | SPI 契约 | 插件实现扩展点接口；宿主 BC 校验租户与配额 |

## 5. 服务边界与通信契约

### 5.1 通信方式

- **同步（API）**：查询、状态操作、强一致动作。内部默认 REST/gRPC；对外经网关暴露开放 API。
- **异步（事件）**：状态变化的事实广播、跨域流程解耦。统一事件总线，事件命名 `&lt;域&gt;.&lt;聚合&gt;.&lt;动作&gt;`（如 `iot.device.provisioned`、`ai.policy.decided`、`biz.ticket.closed`）。
- 事件契约：含 `tenant_id/site_id/occurred_at/payload(消息码化文本)`；消费方各自幂等。

### 5.2 核心事件清单（v0.1 基线）

| 事件 | 生产者 | 主要消费者 |
|---|---|---|
| `platform.tenant.provisioned` | BC-P1 | 各域初始化租户资源；插件装配 |
| `iot.telemetry.received` / `iot.telemetry.quality-scored` | BC-I3 | BC-I4、BC-A2/A3、BC-B3 |
| `iot.alert.raised` | BC-I4 | BC-A2（是否 AI 处置）、BC-S2、BC-P4 |
| `ai.policy.decided`（含级别与理由） | BC-A2 | 发起方 BC、BC-P5 审计、BC-B3 |
| `ai.inference.completed` | BC-A1 | 发起方、BC-P3 计量 |
| `biz.ticket.created / closed` | BC-B2 | BC-B3、BC-P4 |
| `sec.incident.escalated / resolved` | BC-S2 | BC-P4、BC-B3 |
| `plugin.lifecycle.changed` | BC-E1 | BC-P1（装配记录）、目标宿主 BC |
| `metering.usage.recorded` | 各 BC | BC-P3 |

### 5.3 端到端示例（楼宇能耗异常，对应 BP-01 V1 主线）

```
1 空调电表遥测 → BC-I3（质量评分 ok）
2 BC-I4 规则命中"能耗超基线" → 产生告警
3 BC-I4 → 请求 BC-A2 闸门：场景=楼宇能耗优化
   BC-A2 五因子评估：价值✓ 质量✓ 风险=中→L2 权限✓ 成本（BC-P3 实时价）✓
4 BC-A2 → 调度 BC-A1 推理（预测性温控模型）→ 返回建议（调高设定温度 2℃）
5 BC-A2 发 `ai.policy.decided{level:L2, suggestion}` → BC-B2 开工单（待确认）
6 BC-P4 通知租户值班员 → 人工确认 → 工单执行 → `biz.ticket.closed`
7 全链路裁决与执行留痕 BC-P5；AI 用量入 BC-P3
（若同场景风险=低（如只出报表），第 5 步直接 L1 自动闭环）
```

## 6. 行业模块装配机制（行业插件如何挂载）

### 6.1 行业模块包结构（manifest 声明）

```
industry-module-package/
├── manifest.yaml          # 名称/版本/依赖的内核版本/所需权限/资源配额声明
├── extensions/            # 扩展点实现（SPI 插件体）
│   ├── masterdata/        #   站点子类型与主数据扩展（挂 BC-B1）
│   ├── workflow-templates/#   行业流程/工单模板（挂 BC-B2）
│   ├── disposal-policies/ #   处置策略（挂 BC-S2，如警备 SOP）
│   └── protocols/         #   设备协议适配（挂 BC-I1，可选独立协议插件）
├── ai-scenes/             # 预置 AI 场景包（挂 BC-A1/A2：场景登记含风险分级、默认自动化级别）
├── rbac/                  # 角色/权限模板（挂 BC-P2）
├── i18n/                  # 行业词条（消息码 → 多语言）
└── ui/                    # 页面/菜单扩展声明（门户渲染）
```

### 6.2 统一扩展点（SPI）清单

| SPI | 宿主 BC | 用途 |
|---|---|---|
| Protocol SPI | BC-I1 | 新设备协议/驱动接入 |
| MasterData SPI | BC-B1 | 站点子类型、行业对象扩展 |
| Workflow SPI | BC-B2 | 行业流程模板、业务功能插件（巡检/维保/能源等） |
| Disposal SPI | BC-S2 | 警情处置策略与联动动作 |
| AI Capability SPI | BC-A1 | 第三方模型/算法注册为服务（必须带场景与风险元数据，过闸） |
| UI SPI | 门户 | 菜单/页面/卡片扩展 |

### 6.3 装配流程（对应 BP-01 §4.2）

```
租户订阅行业模板（BC-E2）
→ BC-E1 校验 manifest（内核版本兼容/权限/配额）
→ `plugin.lifecycle.changed{installed}` 事件
→ 各宿主 BC 装配对应扩展（主数据模板、流程、RBAC、AI 场景登记）
→ BC-P1 记录装配快照（租户能力清单 = 内核 + 已装插件）
→ 租户门户出现行业菜单与预置数据
```

约束：插件只能通过 SPI 与事件影响平台行为；宿主 BC 在每次调用插件时校验**租户订阅有效性 + 配额 + IAM 权限**。

## 7. 对下游的输入

| 消费方 | 承接内容 |
|---|---|
| BP-03 数据架构 | 每 BC 的存储形态（时序/关系/文档/对象）、事件契约 Schema、租户隔离落地 |
| BP-04 技术架构 | BC 部署形态与合并策略、事件总线/网关选型、沙箱运行时技术 |
| BP-05 安全架构 | IAM 策略模型、租户上下文传播与校验、插件沙箱、审计实现 |
| BP-06 生态 | SPI 详细接口定义、manifest 规范、市场与分成流程 |
| BP-07 部署运营 | 用量事件口径、行业模板发布流程 |

## 8. 开放问题（后续决策）

1. BC 部署粒度：哪些低流量 BC（如 BC-P4/P5、BC-S3）初期合并部署（BP-04 ADR）。
2. 事件 Schema 管理：注册表与版本兼容策略（BP-03/BP-06）。
3. MasterData SPI 的行业扩展深度：字段扩展 vs 子类型聚合扩展的边界。
4. AI Policy 闸门的策略 DSL 与 fail-closed 具体实现（ADR 待立，承接 ADR-0002 后续项）。
5. UI SPI 的能力边界：仅声明式扩展，还是允许插件自定义前端运行时。
