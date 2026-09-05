# BP-06 集成与开放生态（Integration & Open Ecosystem）

> 状态：**v0.1**（2026-09-05）
> 上游依据：[BP-01 §4.3/V3](./01-business-architecture.md)（生态主线）、[BP-02 §6](./02-application-architecture.md)（SPI 与装配）、[BP-03 §5](./03-data-architecture.md)（事件契约与注册表）、[BP-04 §4.2/4.4](./04-technology-architecture.md)（沙箱通道/网关）、[BP-05 §7/§8](./05-security-architecture.md)（插件安全/认证）、[ADR-0002](../adr/0002-ai-governance-and-five-factor-gate.md)、[ADR-0003](../adr/0003-technology-stack-baseline.md)
> 下游消费者：[BP-07 部署与运营](./07-deployment-and-operations.md)（市场运营参数、分成、开发者计划）

## 1. 目的与范围

本文定义 JSL 生态的三大构成与对外接口规范：

1. **开放 API（北向）**：第三方应用与租户集成如何调用平台能力；
2. **插件 SPI（南向）**：插件如何扩展平台——manifest 规范、六大 SPI 契约、生命周期与权限落地；
3. **应用市场与开发者门户**：上架审核、订阅安装、分成结算与开发者体验。

硬约束（继承）：

- 开放 API 与插件的一切调用**必须收敛到租户范围**并经 IAM（BP-05 §4 三道闸）；
- AI 能力插件必须携带**场景与风险元数据**，统一过五因子闸门（ADR-0002），无元数据不予注册；
- 插件权限执行**三级取交天花板**：运行时 ≤ 上架声明 ≤ 租户配置（BP-05 §7.3）；
- 所有事件/API Schema 进**注册表**，次版本向后兼容（BP-03 §5.2）；
- 市场制品必须**签名 + SBOM + 漏洞扫描**通过（BP-05 §7.1）。

## 2. 生态总览

### 2.1 角色与价值交换

```
平台运营方（内核维护/市场运营/审核）        开发者 / ISV（插件·API 应用·行业模板）
        │  开放 API + SPI + 市场                │  ↑ 提交制品 · 获得分成
        ▼                                      │
   租户（订阅安装 · 授权同意 · 用量付费） ◀──────┘
        ▼
   最终用户（租户门户内使用平台+生态能力）
```

### 2.2 生态合作主线（展开 BP-01 V3）

```
开发者注册 → 门户创建应用/插件 → 沙箱联调（免费沙箱租户）
→ 提交上架 → 审核（技术+安全+行业合规）→ 上架市场
→ 租户发现 → 安装/授权（权限知情确认）→ 装配生效（BP-02 §6.3）
→ 用量计量（BC-P3）→ 账期结算与分成 → 版本迭代/下架
```

## 3. 开放 API（北向）

### 3.1 开放分域

| API 域 | 开放能力（v0.1） | 写操作约束 |
|---|---|---|
| 设备与遥测 | 设备档案查询、遥测查询（含聚合）、设备影子读取 | 设备命令下发须经模板授权 + 幂等键 |
| 告警与事件 | 告警查询/认领、Webhook 事件订阅 | 认领/关闭受 RBAC |
| AI 服务 | 场景化推理 API（按 AI 场景开放，统一经闸门，按调用计量） | 仅推理，不含模型管理 |
| 业务流程 | 工单创建/查询/回写、流程触发 | 受租户流程模板约束 |
| 主数据 | Site/资产/人员/产品查询（含行业扩展字段只读） | 开放只读；写走门户/行业模块 |
| 平台 | 租户自身信息、用量与账单查询（self） | — |

### 3.2 认证与授权

| 模式 | 流程 | 适用 |
|---|---|---|
| Client Credentials | 应用凭 ClientId/Secret 换 Token（scope = 安装时租户授予的权限集） | 服务器到服务器集成 |
| Authorization Code + PKCE | 用户登录租户门户 → 同意页展示权限清单 → 发放用户委托 Token | 代表用户操作的应用 |

- Token：JWT，声明含 `app_id / tenant_id / scopes / exp`（短时效 + Refresh Token 轮换）；
- 同意页与权限展示复用插件安装知情模型（BP-05 §7.3）；
- 校验三道闸同 BP-05 §4.2（网关粗校验 → BC 细校验 → RLS 兜底）；
- Secret 管理与凭证生命周期同 BP-05 §3.2，吊销即时生效。

### 3.3 配额、限流与计量

- 配额按 **API 域 × 套餐** 定义；限流按 `app × tenant` 双键（APISIX 插件链，BP-04 §4.4）；
- 每次调用产生计量事件入 BC-P3（`metering.usage.recorded`），支撑账单与开发者分成；
- 超限返回标准错误（含配额重置时间），不静默丢弃。

### 3.4 版本与兼容

- 版本策略：URI 路径 `/v1`；**N-1 至少维护 6 个月**；破坏性变更 = 新 major + 双跑过渡；
- 事件开放子集：从 BP-02 §5.2 核心事件中开放（`iot.telemetry.*`、`iot.alert.*`、`ai.policy.decided`（脱敏）、`biz.ticket.*`、`sec.incident.*`），经 Webhook 订阅——HMAC 签名推送、指数退避重试、消费方幂等；
- Webhook 与 API Schema 同注册表管理。

### 3.5 开发者体验（DX）

- **沙箱租户**：免费配额 + 模拟设备数据集 + 事件回放器，全 API 可调；
- **SDK**：Protobuf/OpenAPI 生成 Go 与 TypeScript SDK（与 BP-04 契约链一致）；
- **文档站**：按 API 域组织、交互式调试、变更日志（由注册表自动生成兼容性说明）。

## 4. 插件 SPI（南向）

### 4.1 插件 Manifest 规范（v0.1）

```yaml
apiVersion: platform.jsl.io/v1
kind: Plugin
metadata:
  name: industry.smart-building
  displayName.i18nKey: plugin.sb.name        # i18n 走消息码
  version: 1.2.0
  vendor: <开发者/ISV id>
  category: industry-module                  # industry-module | protocol | business | ai-capability
  signature: <平台签名>
spec:
  engine: container                          # wasm | container | sandboxed（BP-04 §4.2 W/C/S 级）
  kernelCompatibility: ">=0.1 <0.2"          # 内核版本兼容区间
  artifacts:
    image: <OCI 引用（C/S 级）> 或 module: <WASM 引用（W 级）>
    sbom: <SBOM 引用>
  extensionPoints:                           # 实现的 SPI（§4.3）
    - spi: workflow
      entry: ext/workflow
    - spi: masterdata
      entry: ext/masterdata
  aiScenes:                                  # AI 能力插件/场景包必填（ADR-0002）
    - sceneId: building.energy.forecast
      riskLevel: medium                      # low|medium|high → 默认自动化级别 L1/L2/L3
      defaultAutomation: L2
      dataQualityThreshold: 70               # 该场景要求的数据质量分下限
  permissions:                               # 上架声明 = 天花板（BP-05 §7.3）
    data:
      scope: site                            # tenant | org | site
      fields: [asset.read, ticket.read]      # 含 S3/S4 字段须显式列出（单独标红）
    events:
      subscribe: ["iot.telemetry.*", "iot.alert.raised"]
      publish: ["biz.ticket.created"]
    outbound:                                # 外联白名单（经出口代理审计）
      - host: api.weather.example
        purpose: weather-data
  resources:
    cpu: "1" ; memory: 512Mi ; spiRps: 100   # 资源配额与 SPI 调用限频
  rbacTemplates: ./rbac                      # 行业角色预置（BP-05 §4.3）
  i18n: ./i18n
  ui: ./ui                                   # 声明式 UI 扩展（§4.4）
```

### 4.2 装配与权限落地流程

```
上架：manifest 校验（Schema）→ 签名验证 + SBOM/扫描 → 权限评审（S4 字段/外联重点审）
安装：租户确认权限清单 → BC-E1 校验 kernelCompatibility 与配额 → 事件广播 → 宿主 BC 装配
运行：每次插件调用，宿主校验 (租户订阅有效 × 配额 × IAM) ；Token 绑定 plugin+tenant+scope
升级：同校验流程；数据扩展表按 BP-03 §4.3 演进；不兼容升级走新版本并行
下架/停用：事件广播 → 宿主卸载扩展 → 扩展表级联停用（数据留存按套餐）→ 审计留痕
```

### 4.3 六大 SPI 契约概要

| SPI | 宿主 BC | 调用方向 | 契约形态 | 通道（按隔离级别） |
|---|---|---|---|---|
| Protocol SPI | BC-I1 | 平台↔插件（注册协议处理器 / 数据回调与上报） | Proto 定义 | W 级：WASM ABI；C 级：gRPC |
| MasterData SPI | BC-B1 | 平台→插件（子类型注册、校验钩子、展示元数据） | Proto | 同上 |
| Workflow SPI | BC-B2 | 平台→插件（流程节点执行器）；插件→平台（工单 API 经开放 API 同源） | Proto + REST | 同上 |
| Disposal SPI | BC-S2 | 平台→插件（处置动作执行、SOP 步骤扩展） | Proto | 同上 |
| AI Capability SPI | BC-A1 | 插件→平台（模型注册：含 aiScenes 元数据）；平台→插件（推理调用） | gRPC 推理协议 | 默认 C 级（GPU/进程需求）；S 级可选 |
| UI SPI | 门户 | 声明式扩展（菜单/页面/卡片/筛选器），见 §4.4 | JSON 声明 | 门户沙箱 |

### 4.4 UI 扩展策略（回答 BP-02 开放问题 5）

- **一级（声明式）**：菜单项、列表卡片、详情区块、筛选器、主题词条——纯 JSON 声明，门户原生渲染（安全、首选）；
- **二级（自定义页面）**：插件自定义页面以 **iframe 沙箱**运行，经受控 postMessage API 与门户交互（取数走开放 API 同源鉴权），**不接受任意脚本注入内核**；
- 行业模块优先用一级；确需二级时在 manifest `ui` 中声明，评审加严。

## 5. 应用市场与开发者门户（BC-E2 / 门户）

### 5.1 市场商品类型

| 类型 | 说明 | 典型 |
|---|---|---|
| 行业模块插件 | 行业能力包（含 RBAC/流程/AI 场景/UI） | 智慧楼宇模块 |
| 业务功能插件 | 通用业务扩展 | 高级维保、能耗报表 |
| 协议插件 | 设备协议适配 | OPC-UA 驱动 |
| AI 能力包 | 模型/算法（必须含 aiScenes 元数据） | 视频周界检测 |
| API 应用 | 纯北向集成应用（无插件体） | BI 集成、企业系统连接器 |
| 行业模板 | 插件组合 + 配置 + 预置数据（BP-01 §4.2） | 医疗院区整套模板 |

### 5.2 上架审核流水线

```
提交 → ① 自动校验：manifest Schema、签名、SBOM、漏洞扫描、协议兼容
     → ② 技术评审：权限声明与功能相符、配额合理、SPI 使用正确
     → ③ 安全评审：S4 字段访问/外联/隔离级别（BP-05 §7.1）；AI 插件核验风险元数据（ADR-0002）
     → ④ 行业合规评审（医疗/警备类目）：行业资质与数据要求
     → ⑤ 上架（版本化；每次升级重走 ①③）
```

审核 SLA、驳回与申诉流程由运营侧定义（BP-07）。

### 5.3 商业化与分成

- 计费模式：**订阅制**（月/年）与**用量制**（经 BC-P3 计量）两种，商品声明其一或组合；
- 分成基线：平台/开发者分成比例与结算周期为运营参数（BP-07 定；账期数据来自 BC-P3 计量流水）；
- 试用：支持限时试用（到期自动停用，数据保留策略按 §4.2）。

### 5.4 开发者门户功能

注册与企业认证 → 应用/插件管理（版本、状态、审核进度）→ 沙箱环境（§3.5）→ 凭证管理（Secret 轮换）→ 用量与收益看板 → 文档与变更通知。

## 6. 外部系统集成模式（ACL）

| 模式 | 用途 | 机制 |
|---|---|---|
| 防腐层（ACL） | 第三方系统模型转换 | 集成适配器经开放 API/事件接入，不进内核（BP-02 §4） |
| 批量导入/导出 | 主数据、历史数据迁移 | 异步任务 + 模板校验 + 进度反馈；S3/S4 导出审批（BP-05 §6.3） |
| Webhook 入站 | 第三方回调（支付、外部系统事件） | 签名验证 + 时间窗防重放 + 白名单 |
| 连接器（演进） | 企业系统（ERP/OA/工单）SaaS 化连接器 | 生态商品形态承接，平台只定规范 |

## 7. 对下游的输入

| 消费方 | 承接内容 |
|---|---|
| BP-07 部署与运营 | 审核 SLA、分成与结算参数、开发者计划与沙箱成本、市场运营指标 |

## 8. 开放问题（后续决策）

1. Manifest 正式 Schema 定稿与签名规范（含多插件依赖声明、组合模板的锁定版本策略）——首个插件开发前立 ADR。
2. WASM ABI 的宿主接口集（W 级能力注入清单：IO、时钟、随机数等）——与 Protocol SPI 定稿同步。
3. UI 二级（iframe）postMessage API 的能力范围与安全细则——首个自定义页面需求出现时定。
4. 开放 API 是否提供 GraphQL/流式（SSE/gRPC-stream）形态——按生态需求评估。
5. 市场跨区域上架（商品与数据驻留的一致性）——联动 BP-07 多区域决策。
