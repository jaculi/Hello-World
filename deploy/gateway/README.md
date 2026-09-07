# deploy/gateway/ —— API 网关（M18）

> 依据：[BP-04 §4.4 API 网关与 BFF](../docs/blueprint/04-technology-architecture.md)（APISIX；插件链：认证 → 授权 → 限流 → **租户上下文注入** → 计量 → i18n → 路由）

## 目录结构

```
deploy/gateway/
├── README.md
└── apisix/
    ├── config.yaml               # standalone 配置（compose 直接挂载）
    ├── apisix.yaml               # 路由声明（compose；compose 网络短名 upstream）
    ├── values.yaml               # Helm chart values（standalone 模式挂载 ConfigMap）
    └── manifests/
        └── apisix-standalone-configmap.yaml  # 集群用 ConfigMap（upstream 为集群 DNS）
```

## 路由与插件链

| 路由 | 插件 | upstream |
|---|---|---|
| `/realms/*`、`/resources/*` | 无（透传） | Keycloak（登录页/发现端点/令牌） |
| `/api/*` | proxy-rewrite（去 `/api` 前缀）→ openid-connect（bearer 验签）→ serverless-pre-function（注入 `x-tenant-id`） | tenant-bc:8000 |

关键设计：

- **bearer_only 模式**：无令牌即 401（API 网关）。浏览器 code 流跳转由 M19 门户 BFF 承接，前端获令牌后以 Bearer 调 `/api`。
- **租户上下文注入**：Lua 解码 JWT payload 的 `tenant_id` claim 写入 `x-tenant-id`（签名已由 openid-connect 校验，网关只解码；客户端伪造的同名头被 `set_header` 覆盖）。tenantcontext 中间件 fail-closed 缺头即拒，因此本插件是认证链路的必需环节。
- **BC 侧仍二次验签**：[pkg/authz JWKSVerifier](../../pkg/authz/jwks.go) 经 OIDC discovery + JWKS 做 RS256 验签（iss/aud/exp/tenant_id），构成 BP-05 纵深防御（网关粗校验 + BC 细校验 + RLS）。

## 本地开发

```bash
docker compose -f deploy/dev/docker-compose.yml up -d   # keycloak + apisix + tenant-bc
# 全链路冒烟：
docker run --rm -i --network jsl-tenant-dev_default \
  --entrypoint sh curlimages/curl -s < deploy/dev/m18-auth-smoke.sh
```

冒烟覆盖：无令牌 401 / 合法令牌通过（业务 not_found 404 即认证链通过）/ 篡改签名 401 / Keycloak 经网关 200 / 伪造租户头被覆盖。

## 集群部署（GitOps）

- ArgoCD Application：[deploy/gitops/argocd/apisix.yaml](../gitops/argocd/apisix.yaml)
  多源同步：chart（`https://charts.apiseven.com`，2.15.*）+ values + standalone ConfigMap
- Namespace `apisix` 由 [deploy/gitops/namespaces/identity.yaml](../gitops/namespaces/identity.yaml) 创建
- APISIX Gateway Service 为 NodePort（位于 apisix 命名空间，不受 platform ns 禁止 NodePort 策略约束）

## 限制 / 后续

- standalone 模式路由静态（M18 路由固定可接受）；BC 增多后升级 apisix-ingress-controller + ApisixRoute CRD
- 限流/计量/i18n 渲染/粗授权插件尚未启用（后续里程碑）
- gRPC 东西向不经本网关，走 mTLS 工作负载身份（BP-05 §3.1，后续）
