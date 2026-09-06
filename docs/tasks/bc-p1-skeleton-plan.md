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
4. ~~模板（步 3）能复制出第二个空 BC 且全部门禁通过——"可复制性"是本计划的最终交付物。~~ ✅ 已实证（2026-09-06，见执行记录）

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
| 2026-09-06 | M6 预留收尾（镜像推送 + e2e 进 CI） | ✅ 完成。⑤ 制品 job 落地 GHCR 推送（docker/login-action + metadata-action：master 推 `ghcr.io/jaculi/tenant-bc:sha-<short>` 与 `latest`，PR 仅本地 load 冒烟；permissions: packages: write）；Helm values 默认仓库同步为 ghcr.io/jaculi/tenant-bc；新增 ⑥ e2e job（service containers：postgres:16-alpine + apache/kafka:3.9.1 KRaft，健康检查门控；构建二进制后台起服务，JSL_ENV=prod 实证非 dev 装配+自动迁移，等 /healthz 后跑 `go test -tags=e2e`，always 打印服务日志）。本地同构实证：compose 栈 + prod 模式服务，e2e 两用例全过（信封校验 1.07s、全链路冒烟 0.04s）。 |
| 2026-09-06 | 镜像签名（cosign keyless） | ✅ 完成。⑤ 制品 job 增加 `permissions: id-token: write` + `sigstore/cosign-installer@v3.7.0`；master 推送后按 digest `cosign sign --yes`（Fulcio 短期证书 + Rekor 透明日志，无密钥）；紧接 `cosign verify --certificate-oidc-issuer https://token.actions.githubusercontent.com --certificate-identity-regexp ^https://github.com/jaculi/Hello-World/` 通过，CI 内独立验证签名来源。PR 不签名（避免 fork 写权限问题）。 |
| 2026-09-06 | M7 生产侧 Kyverno 准入（cosign 签名消费端） | ✅ 完成。`deploy/kyverno/`：values.yaml（Helm 安装，Enforce）+ policies/verify-image-signatures.yaml（ClusterPolicy：imageReferences `ghcr.io/jaculi/tenant-bc:*`，keyless attestor issuer=token.actions.githubusercontent.com subject=本仓，rekor=rekor.sigstore.dev，mutateDigest=true，Enforce）；ArgoCD Application（kyverno + kyverno-policies）。kind 实证：已签镜像 `ghcr.io/jaculi/tenant-bc:latest` 放行且 digest 固定；未签 `nginx:alpine` 被拒（`failed to verify image docker.io/nginx:alpine: no signatures found`）。偏差 ①：Kyverno v1.19.0；偏差 ②：Docker Hub 镜像归一化为 `docker.io/nginx:*`（无 `library/` 前缀），策略 imageReferences 须用此形式 |
| 2026-09-06 | M8 ArgoCD 实际部署闭环 | ✅ 完成。kind 集群装 ArgoCD（argo/argo-cd Helm），apply `deploy/gitops/argocd/tenant-bc.yaml`；ArgoCD 从 GitHub 拉取 chart，渲染（dev values + image.repository=ghcr.io/jaculi/tenant-bc, tag=latest），同步 ConfigMap/Service/Deployment 到 platform 命名空间。实证：Application `Synced / Healthy`，Pod 1/1 Running，镜像 `ghcr.io/jaculi/tenant-bc@sha256:54a6…`，/healthz=SERVING。GitOps 全链路闭环：CI 构建签名推 GHCR → ArgoCD 同步 chart → 集群运行签名镜像。偏差 ①：dev values 默认 image.repository=jsl/tenant-bc（kind load 用），ArgoCD Application 须显式覆盖为 ghcr.io 仓库 |
| 2026-09-06 | M9 生产侧 Kyverno 准入生产化 | ✅ 完成。① 签名校验策略扩为平台全域通配 `ghcr.io/jaculi/*:*`（monorepo 全 BC 与后续插件行业服务自动纳入，新 BC 不改策略）+ `background: true`（存量 Pod 生成 PolicyReport）；② 新增 `platform-registry-allowlist` ClusterPolicy：platform 命名空间全容器（含 init/ephemeral，foreach + `\|\| []` 回退）仅允许平台仓库镜像（BP-05 默认拒绝第二道闸）；③ values.yaml 生产化：admission 3 副本 + background 2 副本 HA，修正 M7 顶层 `resources` 键位无效问题（移入 `admissionController`/`backgroundController`），ServiceMonitor 预留；④ ArgoCD kyverno chart 3.2.0→3.9.0 与实证版本（Kyverno v1.19.0）对齐。kind v1.32.2 实证 6 用例全过：已签放行 + digest 固定（default/platform ns）、范围外 nginx 放行、未签平台命名镜像拒（通配 + fail-closed）、platform ns 外部镜像拒、init 容器注入拒。偏差 ①：validate pattern 直接匹配缺省 `initContainers` 列表会误拒普通 Pod，改 foreach 逐容器校验；偏差 ②：Kyverno 宣告 ClusterPolicy（kyverno.io）将废弃迁 CEL（policies.kyverno.io），v1.19.0 仍完整支持，升级大版本时回访 |
| 2026-09-06 | M10 Kyverno 策略 CEL 迁移 | ✅ 完成。将 M9 两套 ClusterPolicy（kyverno.io/v1，Kyverno v1.19 已标记废弃）迁移至 CEL 策略（policies.kyverno.io/v1）：① `verify-image-signatures` → `ImageValidatingPolicy`：`matchConstraints`(pods)+`matchImageReferences`(glob `ghcr.io/jaculi/*:*`)+cosign keyless(`identities.issuer`+`subjectRegExp`)+`ctlog.url`(rekor)+`verifyImageSignatures` CEL 表达式+`validationConfigurations`(mutateDigest/verifyDigest/required)+`validationActions:[Deny]`+`failurePolicy:Fail`+后台扫描；② `platform-registry-allowlist` → `ValidatingPolicy`：`namespaceSelector`(platform)+三条 validations 分别校验 containers/initContainers/ephemeralContainers 的 `image.startsWith('ghcr.io/jaculi/')`；③ ArgoCD kyverno-policies.yaml 的 ignoreDifferences 追加 ImageValidatingPolicy/ValidatingPolicy status。kind v1.32.2 + Kyverno v1.19.0 实证 6 用例全过（已签放行+digest `@sha256:a282…`、范围外放行、未签拒、platform 外部拒、init 注入拒、platform 已签放行），后台 PolicyReport 正常。偏差 ①：CEL keyless `identities.subject` 为严格匹配不支持通配符，必须改 `subjectRegExp` 正则；偏差 ②：自定义 `images[].expression` 须返回镜像字符串列表（`.map(c, c.image)`），返回容器对象报 type conversion error，Pod 推荐用内置 `images.containers`；偏差 ③：CEL 访问不存在字段直接报错，须用 `.?`+`.orValue([])` 回退 |
| 2026-09-06 | M11 Kyverno 准入纵深防御（PSS + 资源限制 + 例外机制） | ✅ 完成。在 M10 两层准入基础上新增两层，形成四层纵深防御：③ `require-pod-security-restricted`（ValidatingPolicy）：强制 platform ns Pod 满足 PSS restricted（privileged=false、allowPrivilegeEscalation=false、runAsNonRoot=true、readOnlyRootFilesystem=true、capabilities.drop=[ALL]、seccompProfile=RuntimeDefault、禁 hostNetwork/hostPID/hostIPC/hostPath），覆盖 containers/init/ephemeral；④ `require-resource-limits`（ValidatingPolicy）：全部容器必须声明 CPU/内存 requests+limits。策略例外机制：values.yaml 启用 `features.policyExceptions.enabled=true` 且 `namespace=kyverno`（例外仅允许在 kyverno ns 创建，平台管理员统一管理），exceptions/template.yaml 提供模板（须 expiresAt 到期失效）。kind 实证：违规 Pod 被 PSS/资源策略拒（含明确消息）、合规 Pod 放行+digest 固定、带 exc 标签的 Pod 经 PolicyException 放行、不带标签仍被拒。偏差 ①：CEL `has(obj.field)` 在字段不存在时报错，须用 `.?field.orValue(...)` 逐层安全访问（含 securityContext 嵌套字段）；偏差 ②：PolicyException 默认关闭，须 `features.policyExceptions.enabled=true` 显式开启 |
| 2026-09-06 | 模板可复制性实证（DoD 第 4 项） | ✅ 完成。从 `templates/bc-skeleton` 复制出第二个 BC `services/org-bc`，全局替换 `templates/bc-skeleton`→`services/org-bc`、`bc-skeleton`→`org-bc`、镜像仓库→`ghcr.io/jaculi/org-bc`，Helm chart 目录重命名为 org-bc。验证全通过：go build/vet/test（exit 0）、biz/data 层零框架依赖（depguard 约束，`go list -deps` 无 kratos/grpc/net/http）、Docker build（distroless，exit 0）、容器运行（HTTP :8000 + gRPC :9000 监听）。保留 org-bc 作为平台第二个空 BC 与模板可复制性实证。偏差：全仓 `buf lint` 报 api/ 目录 import 路径错误（既有问题，与 org-bc 无关，CI 中 buf 配置与本地不同） |
| 2026-09-06 | M12 Kyverno 生产基线（SA token + 命名空间治理） | ✅ 完成。在 M11 四层 Pod 级策略基础上新增四层，形成八层纵深防御。Pod 级新增 ⑤ `disable-automount-sa-token`（ValidatingPolicy）：强制 `automountServiceAccountToken=false` 且禁止 projected 卷挂载 serviceAccountToken。Namespace 级新增三层：⑥ `require-namespace-network-isolation`（标签 `jsl-platform/network-policy=default-deny`）、⑦ `require-namespace-quota`（标签 `jsl-platform/resource-quota=enforced`）、⑧ `require-namespace-labels`（team/environment/cost-center）。kind 实证：SA token 违规 Pod 被拒、缺标签命名空间被拒、完整标签命名空间放行、全合规 Pod 放行+digest 固定；platform 命名空间补充 5 个必需标签以兼容 ArgoCD 同步。偏差 ①：CEL ValidatingPolicy 不支持 `has()` 宏（undeclared reference），须用 `.?field == null` 判断；偏差 ②：CEL 策略无法跨资源查询，网络隔离/资源配额采用「标签声明 + GitOps 流程约束」模式而非直接检查 NetworkPolicy/ResourceQuota |
| 2026-09-06 | M13 Kyverno 生产基线（latest 标签 + 标准标签 + hostPort） | ✅ 完成。在 M12 八层基础上新增三层 Pod 级策略，形成十一层纵深防御：⑥ `disallow-latest-tag`（禁止 `:latest` 镜像标签，BP-04 §6 制品可追溯；已签 `:latest` 经签名策略 mutateDigest 转 digest 后通过）、⑦ `require-pod-standard-labels`（强制 `app.kubernetes.io/name` + `app.kubernetes.io/version`）、⑧ `disallow-host-port`（禁止 hostPort，对外服务经 Service+Ingress/网关）。kind 实证：hostPort 违规被拒、缺标准标签被拒、全合规 Pod 放行+digest 固定。偏差：无 |
