# pkg/ —— 平台壳层公共组件

> 依据：[ADR-0004 决策 1/2](../adr/0004-go-service-framework.md)、[BP-03 §3.2/§4](../blueprint/03-data-architecture.md)、[BP-05 §3/§4/§9](../blueprint/05-security-architecture.md)
> 定位：全平台复用资产。所有 BC 的 `internal/` 只允许经这些组件访问横切能力；修改需架构评审（BP-07 §6）。

| 组件 | 职责 | 关键硬约束 |
|---|---|---|
| `errors` | 消息码 + 渲染参数统一错误（BP-03 §4.2） | 对外永不携带渲染文本；Cause 不外传 |
| `tenantcontext` | 租户上下文注入/提取 + HTTP/gRPC 中间件 | 租户头缺失即拒（fail-closed） |
| `authz` | 令牌验证 + 主体-租户一致性校验 | 跨租户令牌即拒；Verifier 接口为 BC-P2 对接点 |
| `observability` | OTel 初始化 + JSON 日志 | 日志强制租户/链路字段；S3/S4 禁入日志 |
| `dataaccess` | RLS 数据访问基类 + 租户路由表 | 无租户上下文禁触数据库；GUC 事务级绑定 |
| `audit` | 审计事件发射（日志 Sink，事件 Sink 预留） | 审计失败不阻断业务；S3/S4 禁入 |

## 标准中间件链（顺序不可调整，ADR-0004）

```
recovery → tenantcontext → authz → audit → OTel(tracing)
```

## 开发约定

- 新增组件先在本 README 登记，再写代码；
- 组件之间允许的依赖方向：`audit/authz/dataaccess/observability → tenantcontext/errors`，禁止反向与成环；
- 消息码统一前缀 `platform.*`（平台级），BC 业务码用 `<bc>.<...>`。
