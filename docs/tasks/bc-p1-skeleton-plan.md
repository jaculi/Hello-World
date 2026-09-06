# 执行计划：BC-P1 租户与站点骨架模板

> 状态：**待执行**（2026-09-05 制定）
> 上游依据：[ADR-0004 go-kratos v3](../adr/0004-go-service-framework.md)（回归验证点）、[ADR-0005 事件总线与 Schema](../adr/0005-event-bus-and-schema-management.md)、[BP-02 §3.1 BC-P1](../blueprint/02-application-architecture.md)、[BP-03 §3/§4](../blueprint/03-data-architecture.md)、[BP-04 §3/§4/§6](../blueprint/04-technology-architecture.md)、[BP-05 §3/§4/§9](../blueprint/05-security-architecture.md)
> 性质：本计划完成后，平台拥有第一块可复制的基石（模板 + 壳层组件 + 契约链 + CI 门禁），后续 BC 按此复制。

## 1. 目标

按 ADR-0004 的三条实施约束（框架只进壳层、统一 BC 脚手架、K8s DNS 服务发现），落地首个限界上下文 **BC-P1 租户与站点**，并完成 ADR-0004 的回归验证。

## 2. 范围外（本期不做）

| 项 | 处理 |
|---|---|
| BC-P2 完整 IAM | 仅 JWT 验签桩（本地验签 + tenant_id 一致性），接口预留对接 |
| 计费出账 | 不涉及 |
| T1/T2 实际路由 | 仅预留路由表接口，初版仅 T3 单库 RLS |
| 插件装配消费方 | `tenant.provisioned` 事件只发布，无真实消费者 |
| i18n 翻译表 | 只做消息码机制，不建翻译存储 |
| 多区域 / Cell 编排 | 不涉及 |

## 3. 里程碑总览

```
M0 工程基建（步 1-3）→ M1 壳层组件（步 4-9）→ M2 契约（步 10-13）
→ M3 领域实现（步 14-17）→ M4 事件 Outbox（步 18-19）
→ M5 测试与回归（步 20-22）→ M6 部署 CI/CD（步 23-25）
```

关键路径：4→8（壳层）→ 10–13（契约）→ 14–17（领域）→ 20–21（RLS 实证 + ADR-0004 回归）。

## 4. 详细步骤

状态：`☐ 待开始 / ◐ 进行中 / ✅ 完成`

### 阶段 0：仓库与工程基建

| # | 状态 | 步骤 | 内容要点 | 验收 |
|---|---|---|---|---|
| 1 | ✅ | monorepo 骨架 | `api/`（契约仓）、`pkg/`（壳层公共组件）、`services/tenant/`（BC-P1）、`deploy/`（Helm/GitOps）、`.ci/` | 目录结构评审通过（单根模块 `github.com/jsl-aiot/platform`，go 1.25） |
| 2 | ✅ | 工具链固化 | Go 1.25.14（本机便携版）、工具链镜像 `.ci/tools.Dockerfile`、Makefile 构建入口 | 本机 `go build` 验证通过 |
| 3 | ✅ | 定制 kratos-layout 模板 | `templates/bc-skeleton` 四层结构 + distroless Dockerfile + Helm skeleton；depguard 规则禁 biz 依赖框架 | 复制出 `services/tenant` 空服务：编译 + 冒烟通过 |

### 阶段 1：壳层公共组件（pkg/，全平台复用）

| # | 状态 | 组件 | 内容要点 | 验收依据 |
|---|---|---|---|---|
| 4 | ✅ | `pkg/tenantcontext` | 租户上下文（`tenant/org/site/trace`）+ HTTP/gRPC 双中间件：metadata 注入、缺失即拒、ctx 与日志字段透传 | BP-02 §2、BP-05 §4.2 |
| 5 | ✅ | `pkg/errors` | 消息码错误封装：`code + i18n_key + params`，proto 错误枚举生成；对外不携带渲染文本 | BP-03 §4.2 |
| 6 | ✅ | `pkg/authz` | JWT 校验中间件（桩：本地验签 + `token.tenant_id == ctx.tenant_id`）；接口按 BC-P2 对接设计 | BP-05 §3.1 |
| 7 | ✅ | `pkg/observability` | OTel 初始化（tracer/meter）+ slog JSON（强制租户/trace 字段；S3/S4 禁入日志） | BP-04 §5 |
| 8 | ✅ | `pkg/dataaccess` | RLS 基类：pgx 连接池 + 事务内 `SET LOCAL app.tenant_id` + 路由表接口（初版 T3 单库，T1/T2 预留）；裸 SQL 逃逸禁令 | BP-03 §3.2/§4.5 |
| 9 | ✅ | `pkg/audit` | 审计事件发射接口（先结构化审计日志，事件形态预留至 BC-P5） | BP-05 §9.1 |

### 阶段 2：契约先行（ADR-0005 首次落地）

| # | 状态 | 步骤 | 内容要点 | 验收 |
|---|---|---|---|---|
| 10 | ✅ | `api/` 公共包 | 事件信封 proto（`event_id/tenant_id/org_id/site_id/occurred_at/trace_id` + payload） | ADR-0005 决策 2 |
| 11 | ✅ | BC-P1 API proto | `TenantService`（创建/激活/冻结/注销）、`OrgService`、`SiteService`（站点树）、`PlanService`（套餐绑定）；单 proto 生成 HTTP+gRPC | ADR-0004 proto 双生成回归点 |
| 12 | ✅ | 事件 proto | `platform.tenant.provisioned / suspended / deactivated`、`platform.site.created` | BP-02 §5.2 |
| 13 | ✅ | CI 门禁 | `buf lint + buf breaking` 进流水线，违反兼容即拒绝合并 | ADR-0005 决策 2 |

### 阶段 3：BC-P1 领域实现

| # | 状态 | 步骤 | 内容要点 | 验收 |
|---|---|---|---|---|
| 14 | ✅ | `internal/biz`（零框架依赖） | 聚合：Tenant/Org/Site/Plan/Subscription；用例：开通租户（装配记录 + 默认站点/组织初始化）、冻结/恢复、注销（冻结期标记）；纯单测 | BP-02 §3.1、BP-01 §4.2 |
| 15 | ✅ | 数据迁移脚本 | tenants / organizations / sites / plans / subscriptions 五表 + 公共字段（`row_id` ULID、审计四件套、`version`、软删）+ RLS 策略 SQL（会话变量绑定） | BP-03 §3/§4.1 |
| 16 | ✅ | `internal/data` | PO↔领域映射、经 `pkg/dataaccess` 的仓储实现、租户路由表登记接口（隔离级别元数据；初版进程内热更新，广播预留） | BP-04 §4.5 |
| 17 | ✅ | `internal/service+server` | kratos 壳层接线；中间件链顺序固定：`recovery → tenantcontext → authz → audit → OTel`（全平台标准） | ADR-0004 决策 2 |

### 阶段 4：事件与 Outbox

| # | 状态 | 步骤 | 内容要点 | 验收 |
|---|---|---|---|---|
| 18 | ✅ | 事务性发件箱 | 业务事务同事务写 `outbox` 表 → relay 投递 Kafka（dev 用 docker-compose；relay 接口抽象支持测试替身）。表结构与 Outbox 写入口已完成（M4），relay 与 Kafka 接入待做 | 防双写不一致 |
| 19 | ✅ | `tenant.provisioned` 发布链路 | 端到端：开通 → outbox → Kafka（信封字段完整）；消费方契约测试预留 | 事件可到达 |

### 阶段 5：测试与 ADR-0004 回归验证

| # | 状态 | 步骤 | 内容要点 | 验收 |
|---|---|---|---|---|
| 20 | ✅ | 集成测试（testcontainers） | **越界实证**：租户 A 会话查租户 B 数据必须为空；事务回滚不污染 RLS 会话变量 | BP-03 §3.2 |
| 21 | ✅ | ADR-0004 回归清单 | ① 中间件链顺序 ② OTel 上报（含租户标签）③ RLS 绑定 ④ proto 双通道一致性 ⑤ biz 零框架依赖（lint）——全过则 ADR-0004 验收关闭 | ADR-0004 |
| 22 | ✅ | e2e 冒烟 | docker-compose 全栈：注册 → 开通 → 站点初始化 → 查询 → 越租户 404 | 演示可跑 |

### 阶段 6：部署与 CI/CD

| # | 状态 | 步骤 | 内容要点 | 验收 |
|---|---|---|---|---|
| 23 | ✅ | 制品 | distroless Dockerfile + Helm chart（HPA、探针、资源配额） | BP-04 §6 |
| 24 | ✅ | dev 环境部署 | k3d/kind 本地集群 + GitOps 目录骨架（ArgoCD Application 预留） | BP-04 §6/§2.4 |
| 25 | ✅ | CI 全门禁 | buf → lint（含自定义规则）→ 单测 → 集成测 → 构建签名（预留） | BP-05 §7.1 预埋 |

## 5. 完成定义（DoD）

1. 步 1–25 全部 ✅；
2. ADR-0004 回归清单（步 21）五项全过，ADR-0004 状态可标注"验证完成"；
3. e2e 冒烟（步 22）在干净环境一键可复现；
4. 模板（步 3）能复制出第二个空 BC 且全部门禁通过——"可复制性"是本计划的最终交付物。

## 6. 执行备注

- 建议执行顺序严格按阶段推进，阶段 1 的 6 个组件是全平台资产，宁可慢不可糙；
- 每完成一步更新本文档状态列；发现与 Blueprint/ADR 冲突时停下立 ADR，不绕行；
- 完成后新增 BC 的复制成本预期 ≤ 1 天（模板 + 契约 + 领域三件套）。

## 7. 执行记录

| 日期 | 里程碑 | 结果与偏差 |
|---|---|---|
| 2026-09-05 | M0（步 1-3） | ✅ 完成。模块策略：单根 go.mod（`github.com/jsl-aiot/platform`），模板 `templates/bc-skeleton` 与服务同模块保证可编译。kratos v3.0.0 已锁定。偏差 ①：本机为 Windows，Go 采用便携版（`~/.local/lib/go`，1.25.14）；偏差 ②：`.golangci.yml` depguard 规则已就位但本机未装 golangci-lint，首跑待 M2 CI 接入；偏差 ③：模板 Dockerfile 以 `ARG SERVICE_PATH` 支持同一 Dockerfile 构建任意 BC |
| 2026-09-05 | M1（步 4-9 + 步 17 链接线） | ✅ 完成。全链 `recovery → tenantcontext → authz → audit → Tracing` 挂入 tenant-bc，HTTP/gRPC 双端口冒烟通过，pkg 全部单测绿。偏差 ①：kratos v3 无 `middleware/tracing`，自研 `observability.Tracing()`（上游 trace 提取 + Server Span + 租户属性 + 消息码状态）；偏差 ②：pgx v5.10 无 `Pool.BeginFunc`，改用包级 `pgx.BeginFunc`；偏差 ③：步 5 的 proto 错误枚举生成与 HTTP 错误编码映射，移至 M2 与契约生成一体落地；偏差 ④：OneDrive 同步曾回退个别编辑，已通过回读+重编译确认最终状态 |
| 2026-09-06 | M4（步 14-19，data SQL + service/server + Outbox relay + e2e） | ✅ 完成。SQL 仓储五表 + 迁移（RLS/种子套餐）、service/server 装配（PlatformError 编码器 + healthz）、PG/内存双模式装配；relay（franz-go Sink 抽象 + AdminTx 轮询 SKIP LOCKED + 至少一次语义）+ dev compose（PG16/Kafka KRaft）+ e2e（开通 → outbox → Kafka 信封校验，含消费方契约预留）。偏差 ①：`pkg/dataaccess` 新增 `AdminTx`（特权事务口，relay 专用）；偏差 ②：franz-go 默认禁止生产端自动建 topic，`AllowAutoTopicCreation()` 仅 dev 便利（生产 Strimzi 预建）；偏差 ③：无 OTLP endpoint 时 noop tracer 产生不了 trace_id（破坏 BP-03 §4.1 信封不变式），改为丢弃导出器模式；偏差 ④：HTTP 编码字段名为 proto 原生 snake_case（非 protojson 驼峰），e2e 已兼容；偏差 ⑤：编辑器自动保存偶发回退未提交编辑，单文件整体重写规避 |
| 2026-09-06 | M5（步 20-22，RLS 实证 + ADR-0004 回归 + e2e 冒烟） | ✅ 完成。testcontainers RLS 集成测试（越界隔离、回滚不泄漏、fail-closed、AdminTx bypass）；ADR-0004 回归五项全过（中间件链顺序、OTel 租户属性、RLS 绑定、proto 双生成、biz/data 零 kratos——AST 静态回归测试固化）；e2e 全链路冒烟（开通 → 查询 → 越租户 404 → 根站点初始化）。偏差 ①：postgres 镜像默认用户为 superuser 导致 FORCE RLS 不生效，测试中创建普通角色 app_user 验证；偏差 ②：pgx ExecMulti 多语句简单协议对 `DO $$` 块与 `ALTER ROLE NOSUPERUSER` 不稳定，拆分为单语句规避 |
| 2026-09-06 | M6（步 23-25，制品 + 部署 + CI） | ✅ 完成。distroless 多阶段 Dockerfile（41.2MB，nonroot）；Helm chart（/healthz HTTP 探针、资源配额、HPA v2、securityContext 只读根文件系统 + cap drop ALL、envFrom ConfigMap/Secret 分离敏感项）；kind 本地集群部署实证（镜像 load → helm install → rollout → port-forward /healthz=SERVING）；GitOps ArgoCD Application 骨架；CI 全门禁（buf→golangci-lint depguard→vet/单测/AST 回归→testcontainers 集成→镜像构建+容器冒烟，签名/推送随制品库接入预留）；depguard 增补 data 层禁框架规则；清理 protoc 误生成的根级空目录。偏差：无（本机无 kind，以 go install sigs.k8s.io/kind 安装 v0.27.0） |
| 2026-09-06 | M6 预留收尾（镜像推送 + e2e 进 CI） | ✅ 完成。⑤ 制品 job 落地 GHCR 推送（docker/login-action + metadata-action：master 推 `ghcr.io/jaculi/tenant-bc:sha-<short>` 与 `latest`，PR 仅本地 load 冒烟；permissions: packages: write）；Helm values 默认仓库同步为 ghcr.io/jaculi/tenant-bc；新增 ⑥ e2e job（service containers：postgres:16-alpine + apache/kafka:3.9.1 KRaft，健康检查门控；构建二进制后台起服务，JSL_ENV=prod 实证非 dev 装配+自动迁移，等 /healthz 后跑 `go test -tags=e2e`，always 打印服务日志）。本地同构实证：compose 栈 + prod 模式服务，e2e 两用例全过（信封校验 1.07s、全链路冒烟 0.04s）。镜像签名（cosign keyless）仍为预留 |
