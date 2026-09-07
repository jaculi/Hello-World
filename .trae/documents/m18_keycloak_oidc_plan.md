# M18 身份基础设施（Keycloak + APISIX OIDC + JWKS 验签）实施计划

## 仓库研究结论

**现状（已验证代码）：**

1. **认证桩已预留替换点**：[pkg/authz/authz.go](file:///C:/Users/jackl/.local/src/Hello-World/pkg/authz/authz.go) 定义 `Verifier` 接口（`Verify(ctx, rawToken) (Claims, error)`），`Claims{Subject, TenantID, Roles}`；当前唯一实现是 [stub.go](file:///C:/Users/jackl/.local/src/Hello-World/pkg/authz/stub.go) 的本地 HMAC-SHA256 验签（dev/测试用），注释明确「BC-P2 落地后替换为 JWKS 实现，Verifier 接口保持不变」。
2. **中间件链固定**（硬约束，不可调序）：recovery → tenantcontext → authz → audit → OTel（[server.go](file:///C:/Users/jackl/.local/src/Hello-World/services/tenant/internal/server/server.go#L50-L59)）。
3. **租户上下文 fail-closed**：[tenantcontext/middleware.go](file:///C:/Users/jackl/.local/src/Hello-World/pkg/tenantcontext/middleware.go) 要求每个请求带 `x-tenant-id` 头，缺失即 400 拒绝；[authz/middleware.go](file:///C:/Users/jackl/.local/src/Hello-World/pkg/authz/middleware.go#L22-L29) 还校验令牌 `tenant_id` 声明与头租户一致（不一致 403）。
   → **关键推论**：OIDC 登录后租户头必须由**网关注入**（BP-04 §4.4 插件链「租户上下文注入」），不能靠服务侧从令牌补——tenantcontext 在 authz 之前执行，且硬约束要求缺头即拒。因此 APISIX 是本里程碑关键路径，不可省略。
4. **网关/IdP 尚不存在**：deploy/ 下无 APISIX、无 Keycloak、无 Ingress 资源；dev 栈 [docker-compose.yml](file:///C:/Users/jackl/.local/src/Hello-World/deploy/dev/docker-compose.yml) 仅 PG + Kafka。
5. **蓝图选型已锁定**（BP-04）：身份服务 **Keycloak（自托管 OIDC）**；网关 **APISIX**，插件链为 认证(OIDC) → 粗授权 → 限流 → 租户上下文注入 → 计量 → i18n → 路由；前端 React+Next.js（**不在本里程碑**）。
6. **配置机制**：服务经 envFrom ConfigMap/Secret 读环境变量（[deployment.yaml](file:///C:/Users/jackl/.local/src/Hello-World/deploy/helm/tenant-bc/templates/deployment.yaml#L32-L36)），ConfigMap 由 `values.env` 渲染（[configmap.yaml](file:///C:/Users/jackl/.local/src/Hello-World/deploy/helm/tenant-bc/templates/configmap.yaml)）；当前 `JSL_AUTHZ_JWT_SECRET` 选 verifier。
7. **Kyverno 约束已就位**：新 namespace 必须带 `jsl-platform/team|environment|cost-center` 标签；platform ns 禁 NodePort/LB Service（网关 LB 放 apisix 自身命名空间即可规避）；Ingress TLS 策略仅作用 platform ns。
8. **go.mod 无任何 JWT/OIDC 库**，需新增依赖。

## 范围

**本里程碑（M18）交付「能认证」**：浏览器/API 经 Keycloak 登录拿令牌 → APISIX 验令牌并注入租户头 → BC 服务 JWKS 验签（RS256，替换 HMAC 桩）→ 租户一致性校验通过。

**范围外（M19+）**：Next.js 门户前端页面；租户开通→Keycloak 建用户的自动化（BC-P2 IAM）；企业 IdP 联邦登录；MFA 强制；APISIX 限流/计量/i18n 插件；gRPC 东西向 mTLS（BP-05 §3.1 工作负载身份，后续）。

## 变更文件与模块

**新增：**

* `deploy/identity/keycloak/jsl-realm.json` —— realm 引导声明（唯一事实源，compose 与 Helm 共用）

* `deploy/identity/keycloak/values.yaml` —— Keycloak 官方 Helm chart values

* `deploy/identity/README.md` —— 部署与 realm 说明

* `deploy/gateway/apisix/values.yaml` —— APISIX Helm values（standalone 模式）

* `deploy/gateway/apisix/standalone-routes.yaml` —— 路由 + OIDC 插件 + 租户头注入声明

* `deploy/gateway/apisix/pre-function.lua` —— 从访问令牌提取 tenant\_id 注入 `x-tenant-id` 的 Lua

* `deploy/gateway/README.md`

* `deploy/gitops/argocd/keycloak.yaml`、`deploy/gitops/argocd/apisix.yaml` —— ArgoCD Application

* `deploy/gitops/namespaces/keycloak.yaml`、`apisix.yaml` —— 带 Kyverno 必需标签的 Namespace

* `pkg/authz/jwks.go`、`pkg/authz/jwks_test.go` —— JWKS Verifier 实现与单测

**修改：**

* `pkg/authz/authz.go` —— 新增 `NewVerifierFromEnv()`（按 `JSL_AUTHZ_MODE=stub|jwks` 选择实现，默认 stub 保持向后兼容）

* [services/tenant/main.go](file:///C:/Users/jackl/.local/src/Hello-World/services/tenant/main.go) —— verifier 装配改走 `NewVerifierFromEnv()`

* `templates/bc-skeleton/main.go` —— 同步接线，保持模板可复制性

* [deploy/dev/docker-compose.yml](file:///C:/Users/jackl/.local/src/Hello-World/deploy/dev/docker-compose.yml) —— 增加 keycloak、apisix、etcd 三服务

* `deploy/dev/tenant-bc-dev-values.yaml` —— 增加 `JSL_AUTHZ_*` 环境变量示例

* `docs/tasks/bc-p1-skeleton-plan.md` —— M18 执行记录

* `go.mod` / `go.sum` —— 新增 `github.com/coreos/go-oidc/v3`

## 实施步骤（依赖序）

### 阶段 1：Keycloak 与 realm 声明

1. 编写 `deploy/identity/keycloak/jsl-realm.json`：

   * realm = `jsl`；开启 userManagedAccess 不需要；access token lifespan 默认；

   * client `jsl-gateway`：confidential，`standardFlowEnabled=true`（浏览器 code 流）、`directAccessGrantsEnabled=true`（dev 密码模式，供 curl 测试），redirect URIs = `http://localhost:9080/*`（compose/kind APISIX 入口）；

   * Protocol Mapper：User Attribute → `tenant_id`，加入 access token claim 与 userinfo；

   * demo 用户 `demo-admin`（密码 `dev`，user attribute `tenant_id=<预建租户 ULID>`），realm 角色 `TENANT_ADMIN`；

   * 客户端角色/audience 保持默认（aud = client\_id）。
2. compose 增加 keycloak：`quay.io/keycloak/keycloak:26.1`，`start-dev --import-realm`，realm JSON 挂载到 `/opt/keycloak/data/import/`；宿主端口 8081；env `KC_BOOTSTRAP_ADMIN_USERNAME/PASSWORD=admin/admin`、`KC_HOSTNAME_STRICT=false`、`KC_HTTP_ENABLED=true`。
3. 启动验证：`curl http://localhost:8081/realms/jsl/.well-known/openid-configuration` 返回发现文档；密码模式取 token 成功且 payload 含 `tenant_id`。

### 阶段 2：JWKS Verifier（pkg/authz）

1. `go get github.com/coreos/go-oidc/v3/oidc`（OIDC discovery + JWKS 自动缓存轮换）。
2. 实现 `JWKSVerifier`：

   * `oidc.NewProvider(ctx, issuer)` 发现 → `provider.Verifier(&oidc.Config{ClientID: audience})`；

   * 解析 claims：`sub`、自定义 `tenant_id` 声明、`realm_access.roles`；

   * 验签失败/过期/issuer 不符/audience 不符 → 返回 `CodeTokenInvalid`（401），**fail-closed**；

   * provider 初始化失败不 panic：后台重试，初始化完成前所有验证请求拒绝（记错误日志）。
3. `NewVerifierFromEnv()`：`JSL_AUTHZ_MODE`（空/stub → StubVerifier，读 `JSL_AUTHZ_JWT_SECRET`；jwks → JWKSVerifier，读 `JSL_AUTHZ_ISSUER`、`JSL_AUTHZ_JWKS_URL`（可选，默认 issuer 推导）、`JSL_AUTHZ_AUDIENCE`、`JSL_AUTHZ_TENANT_CLAIM`（默认 `tenant_id`））。
4. 单测（httptest 本地 OIDC：RSA 密钥对自签 JWKS/token）：合法通过；错签名/过期/错 issuer/错 audience 拒；tenant\_id 缺失拒；`NewVerifierFromEnv` 模式选择。
5. tenant main.go 与 bc-skeleton 模板接线改 `NewVerifierFromEnv()`；`go build ./...`、`go vet`、`go test ./pkg/... ./services/...` 全绿（stub 默认，既有测试零改动）。

### 阶段 3：APISIX 网关

1. compose 增加 etcd 3.5（单节点）+ `apache/apisix:3.11`；APISIX config 指向 etcd；宿主端口 9080。
2. 路由（先用 admin API/declarative 验证，最终落 standalone 声明文件）：

   * `/realms/*`、`/resources/*` → `keycloak:8080`（登录页、发现端点、JWKS 浏览器可达）；

   * `/api/*` → `tenant-bc:8000`（compose 网络内），插件链：

     * `openid-connect`：discovery=`http://keycloak:8080/realms/jsl/.well-known/openid-configuration`、client\_id/secret=jsl-gateway、`bearer_only=false`（无令牌浏览器走 code 流跳转登录）、`set_access_token_header=true`（X-Access-Token）；

     * `serverless-pre-function`：Lua 读取 `X-Access-Token`，base64 解码 JWT payload，取 `tenant_id` 设 `x-tenant-id` 头（缺则放行让 BC fail-closed 拒绝，网关不伪造）。
3. 集群侧：`deploy/gateway/apisix/values.yaml`（Helm chart `https://charts.apiseven.com`，standalone 模式挂载路由 ConfigMap，Gateway Service 用 NodePort）；ArgoCD Application + apisix namespace（带 Kyverno 标签）。
4. Keycloak 集群侧：官方 chart `https://keycloak.github.io/helm-charts`（chart `keycloak`，dev 用内置 PostgreSQL subchart、单副本），realm JSON 经 ConfigMap 挂载 + `--import-realm`；ArgoCD Application + keycloak namespace。

### 阶段 4：全链路验证与文档

1. **compose 全链路**：

   * 密码模式取 token → `curl -H "Authorization: Bearer <t>" http://localhost:9080/api/v1/tenants` → 200（网关注入 x-tenant-id，BC JWKS 验签通过）；

   * 无 token 访问 /api → 302 跳转 Keycloak 登录页；

   * 篡改签名 token → 401；他租户 token（tenant\_id 与头不一致场景由网关注入保证同源，另测伪造头 + 合法令牌 → 403 一致性）；

   * 浏览器手动走 code 流登录后访问 API 200。
2. **kind 实证**：ArgoCD sync keycloak/apisix/namespace；tenant-bc dev values 切 `JSL_AUTHZ_MODE=jwks`（ISSUER=`http://localhost:9080/realms/jsl` 浏览器面，JWKS\_URL=`http://keycloak.keycloak.svc:8080/realms/jsl/protocol/openid-connect/certs` 集群面）；port-forward 后重复 13 的用例。
3. 回归：stub 模式下既有全部单测与 e2e 不受影响；golangci-lint 通过；更新 deploy/identity、deploy/gateway README 与计划记录；提交 `feat(m18)`。

## 依赖与注意事项

* **新增 Go 依赖**：`github.com/coreos/go-oidc/v3`（含 `go-jose` 间接依赖），需过 depguard（pkg/authz 属壳层，允许第三方库；biz/data 零框架依赖约束不受影响）。

* **镜像**：Keycloak `quay.io/keycloak/keycloak:26.1`、APISIX `apache/apisix:3.11`、etcd `bitnami/etcd:3.5`（均非平台签名镜像，部署在 keycloak/apisix 命名空间，不受 platform ns 签名/允许清单策略约束；Kyverno 命名空间标签策略须满足）。

* **realm JSON 单一事实源**：compose 与 Helm 挂载同一份文件，避免两套配置漂移。

* **服务环境变量**（Helm values.env 透传）：`JSL_AUTHZ_MODE=jwks`、`JSL_AUTHZ_ISSUER`、`JSL_AUTHZ_AUDIENCE=jsl-gateway`、`JSL_AUTHZ_JWKS_URL`；client secret 仅 APISIX 侧需要（BC 只验签不持密）。

* **demo 租户 ULID**：realm 用户的 tenant\_id 须与 BC 内预建租户一致；compose 内存模式下每次重建需重新建租户，验证脚本里串起来（建租户 → 取 token → 调 API）。

## 验证清单

* [ ] `go build ./...` / `go vet ./...` / `go test ./...` 全绿（含新 JWKS 单测）

* [ ] golangci-lint 通过

* [ ] compose：Keycloak 发现端点可访问、密码模式取 token 含 tenant\_id

* [ ] compose：APISIX 带 token 200、无 token 302、坏 token 401、伪造租户头 403

* [ ] kind：ArgoCD 同步 keycloak/apisix Application 健康

* [ ] kind：浏览器 OIDC code 流登录后访问 /api 返回 200

* [ ] stub 模式回归：既有 e2e/单测零改动通过

## 风险与应对

1. **Keycloak issuer/hostname 三方对齐**（浏览器入口、APISIX 代理、BC 验签 issuer 必须一致）：dev 设 `KC_HOSTNAME_STRICT=false` + 固定 `JSL_AUTHZ_ISSUER=http://localhost:9080/realms/jsl`（浏览器面），JWKS 抓取走集群 DNS（host 与 issuer 允许不同，go-oidc 仅按 issuer 校验令牌声明）。若仍不对齐，退回 APISIX 同时代理 `/realms` 使内外 URL 同源。
2. **APISIX OIDC 插件 claim 注入差异**：优先 `X-Access-Token` + Lua 解码；若该版本插件不提供此头，退回 `X-Userinfo`（realm mapper 开启 Add to userinfo），Lua 改读 userinfo JSON。
3. **JWKS 端点不可用即全站 401**（fail-closed 副作用）：go-oidc 内置密钥缓存与后台刷新；Keycloak 短暂重启时缓存内密钥仍可验签。开发/CI 不受影响（默认 stub）。
4. **kind 资源占用**：Keycloak（+PostgreSQL）+ APISIX（+etcd）约 1.5GB 内存；kind 三节点应可承载，必要时 Keycloak/PostgreSQL 单副本并降低 limits。
5. **APISIX standalone 模式无动态路由**：M18 路由固定可接受；后续 BC 增多时升级 apisix-ingress-controller + ApisixRoute CRD（GitOps 动态路由）。
6. **realm 导入幂等**：Keycloak `--import-realm` 仅在 realm 不存在时导入；realm JSON 变更后需删 realm 重导或用 keycloak-config-cli，M18 在 README 注明此限制。

