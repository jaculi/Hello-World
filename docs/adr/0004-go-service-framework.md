# ADR-0004：Go 服务框架选型——go-kratos v3

- 状态：**Accepted**
- 日期：2026-09-05
- 关联：[ADR-0003 技术栈基线](./0003-technology-stack-baseline.md)、[BP-02 应用架构](../blueprint/02-application-architecture.md)、[BP-04 技术架构](../blueprint/04-technology-architecture.md)

## 背景（Context）

ADR-0003 确定后端 Go 全栈、gRPC 通信、Protobuf 为契约单一事实源。首个业务 BC 开发前需在 BP-04 §9 清单中先定服务框架，避免 18 个限界上下文各自成型。

核心需求（来自既有决策）：

1. **契约单一**：proto 定义生成 HTTP + gRPC 双通道（BP-04 §3.1），不得引入第二契约源；
2. **DDD 分层工程结构**：与 BP-02 限界上下文划分对齐，领域层（biz）零框架依赖；
3. **统一中间件挂载**：租户上下文传播、IAM 鉴权、审计埋点、消息码错误封装、OTel 追踪（BP-02 §1/§2、BP-05 §4.2）；
4. **云中立 K8s 适配**：服务发现可基于 K8s Service/DNS，不强制外部注册中心；
5. **不与存储层绑定**：数据访问层自建（租户路由 + RLS，BP-04 §4.5），框架不得自带 ORM/缓存生态造成重复治理。

候选（2026-09 核实现状）：

| 候选 | 现状 | 特征 |
|---|---|---|
| **go-kratos v3** | v3.0.0（2026-06-26 发布，MIT，~25.8k star，活跃维护；要求 Go ≥1.25） | 以 Protobuf 为中心定义 API，生成 HTTP/gRPC 双通道；统一 Transport 抽象；可组合 Middleware；注册/配置/编码插件化；kratos-layout 模板参考 DDD/Clean Architecture |
| **go-zero** | v1.10.3（2026-08-01 发布，MIT，非常活跃） | goctl 代码生成（HTTP 侧以 .api DSL 为源、gRPC 用 proto）；自带 redis/sqlx/熔断/缓存等组件生态；开箱即用程度高 |
| **轻量组合** | stdlib + chi + grpc-go + 自建 | 最大自由度，但 18 个 BC 的横切模式全靠自建约定，一致性风险高 |

## 决策（Decision）

**采用 go-kratos v3 作为服务框架骨架**，并附加三条实施约束：

1. **框架只进"壳层"**：kratos 仅用于 Transport（HTTP/gRPC server）、生命周期与依赖注入；`internal/biz` 领域层与 `internal/data` 数据访问层为**纯 Go + 标准库**，不 import kratos——保证框架未来可替换（逃生舱）。
2. **统一 BC 脚手架模板**：基于 kratos-layout 定制平台模板，预置：租户上下文中间件（`tenant/org/site/trace` 注入）、IAM 鉴权中间件（对接 BC-P2）、审计埋点、消息码错误封装、OTel 初始化、RLS 会话绑定的数据访问基类。所有 BC 从模板生成，禁止自拼结构。
3. **不引入 kratos 注册中心依赖**：K8s 内服务发现直接使用 K8s Service/DNS；注册中心 contrib 仅在专有云非 K8s 场景按需启用。

### 选型理由（对照需求）

| 需求 | go-kratos v3 | go-zero | 轻量组合 |
|---|---|---|---|
| proto 单一契约源生成 HTTP+gRPC | **✅ 原生**（protoc 插件双生成） | ⚠️ HTTP 用 .api DSL，第二契约源（可互转但长期双维护） | ⚠️ 需自拼 grpc-gateway/Connect |
| DDD 分层结构 | **✅ layout 即 DDD/Clean 参考** | ⚠️ 自有目录约定，与 BP-02 映射需改造 | ❌ 全靠自建约定 |
| 中间件统一挂载 | ✅ 可组合 middleware 链 | ✅ 有，但自带生态倾向 | ❌ 自建 |
| 云中立 K8s（不强制注册中心） | **✅ 组件化可选** | ⚠️ 自带 etcd/discov 生态 | ✅ |
| 不绑存储层 | **✅ 不含 ORM/缓存** | ❌ 自带 sqlx/redis/cachex 生态，与自建数据访问层重复治理 | ✅ |
| 18 BC 一致性保障 | ✅ 脚手架 + 结构统一 | ✅ goctl 强约定 | ❌ 一致性风险最高 |
| 逃生舱（未来换框架成本） | ✅（决策 1 保证） | ⚠️ 深度绑定自带生态 | ✅ |

go-zero 的核心优势（goctl CRUD 生成、开箱组件）在本平台价值有限：数据层因租户路由/RLS 需自建（BP-04 §4.5），熔断限流由 APISIX 网关统一治理（BP-04 §4.4），其自带组件反而引入第二套治理口径。

### 重审触发条件

- go-kratos v3 维护停滞：连续 6 个月无安全补丁或社区实质停滞 → 重估 go-zero / 轻量组合；
- kratos 迁移成本由决策 1（壳层隔离）兜底，重估限于 Transport 层。

## 后果（Consequences）

### 正面

- 契约链彻底单一：proto → HTTP/gRPC/OpenAPI/SDK 一路生成（BP-06 §3.5 的 TS/Go SDK 同源）；
- 18 个 BC 工程结构、中间件行为、可观测埋点天然一致，脚手架模板成为准入门禁；
- 领域层零框架依赖，框架升级/替换被限制在壳层，长期演进风险可控；
- 与云中立 K8s、自建数据访问层、网关治理的既有决策零冲突。

### 负面 / 约束

- v3 为 2026-06 新发布大版本（自 v2 有破坏性变更），contrib 集成（OTel 扩展等）需在首个 BC 中验证；选 v3 起步是为避免 v2→v3 迁移；
- go-zero 式 CRUD 代码生成缺失，简单 BC 的样板代码略多——由平台模板缓解；
- 租户上下文、审计、消息码等中间件需平台自建（本为既有计划，非额外成本，但需在模板中一次做对）；
- 要求构建环境 Go ≥ 1.25，CI 镜像与开发者环境需统一。

### 后续决策依赖

- 首个 BC（建议 BC-P1 租户与站点）按模板落地后，回归验证：中间件链、OTel 上报、RLS 绑定与 proto 生成链路；
- BP-04 §9 表中该项已闭环，其余落地 ADR（事件总线产品、时序规模预案等）按其触发时机推进。
