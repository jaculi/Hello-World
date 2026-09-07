# M20 租户门户 K8s 部署（Helm + ArgoCD + APISIX hostname 路由）实施计划

## Context

M19 落地 Next.js 租户门户骨架，但仅跑宿主 `npm run dev` 对接 docker-compose 栈。M19 计划明确将 K8s 部署与 Ingress hostname 对齐留到 M20：「kind/K8s 部署门户（提供 Dockerfile；Ingress 处理 hostname 对齐留 M20）」。

**M20 目标**：门户镜像经 Helm Chart 部署到 kind 集群，经 APISIX hostname 路由暴露，浏览器 OIDC 登录流程在 K8s 内端到端跑通——Keycloak 令牌 `iss`、门户 Auth.js discovery、APISIX openid-connect 三方 issuer 对齐。

## 蓝图依据

- BP-04 §3：前端 `React + Next.js (TypeScript)`
- BP-04 §4.4：APISIX 插件链（认证→授权→限流→租户上下文注入→计量→i18n→路由）；BFF 按门户聚合 BC
- BP-04 §6：GitOps（ArgoCD）同步各 BC 与基础设施
- BP-05 §3.1：前端令牌不入浏览器存储（BFF 持 httpOnly cookie）
- ADR-0004 决策 3：GitOps 部署

## 范围

**本里程碑交付「门户上 K8s、hostname 对齐、浏览器登录跑通」**：
- `web/tenant-portal/` 产 standalone 镜像（M19 Dockerfile 已就绪）
- `deploy/helm/tenant-portal/` Helm Chart（仿 tenant-bc 模式，生产基线 values + dev override）
- `deploy/gitops/argocd/tenant-portal.yaml` ArgoCD Application
- APISIX standalone 路由增 hostname 分流 + portal upstream
- Keycloak `KC_HOSTNAME=http://auth.localtest.me` 锁定令牌 iss
- CoreDNS rewrite 使 pod 内 `auth.localtest.me` 解析到 APISIX ClusterIP
- kind 集群 `extraPortMappings` 映射 80→APISIX NodePort
- 门户 `/healthz` 探针端点 + middleware 排除

**范围外**：
- CI 构建 portal 镜像 + GHCR 推送 + cosign 签名（本地 `docker build` + `kind load` 替代，CI 随后续里程碑）
- kind 集群装 Kyverno（Helm chart 仍按生产基线编写 SA/securityContext/probes/labels，但不做准入强制）
- cert-manager + TLS 证书（dev 用 HTTP，生产 TLS 留后续）
- Go BFF 独立服务剥离（M19 范围外清单项）
- 平台运营门户 / 开发者门户

## 关键技术决策

### 1. hostname 策略：双 hostname 经 APISIX 路由

| hostname | 路由目标 | 用途 |
|---|---|---|
| `portal.localtest.me` | `tenant-portal.platform.svc:3000` | 浏览器访问门户 |
| `auth.localtest.me` | APISIX 内部路由（`/realms/*`→Keycloak，`/api/*`→tenant-bc） | 浏览器登录跳转 + 门户 BFF 服务端调 API |

**为何不用单 hostname 路径分流**：门户 BFF 用 `/api/proxy/*` 调 APISIX，APISIX 已有 `/api/*` 路由。单 hostname 下 `/api/*` 会被 APISIX 截获而非到达门户，BFF 链路断裂。双 hostname 干净分离。

**为何用 APISIX standalone 路由而非 K8s Ingress 资源**：
- 现有基础设施用 APISIX standalone YAML（`apisix-standalone-configmap.yaml`），不是 K8s Ingress controller
- 装 ingress-nginx 增加依赖
- 不创建 K8s Ingress 资源 → 规避 `require-ingress-tls` 策略对 dev 的 TLS 强制

### 2. CoreDNS rewrite（pod 内 hostname 对齐）

Auth.js v5 Keycloak provider 用 `AUTH_KEYCLOAK_ISSUER` 同时做浏览器 authorize 跳转和服务端 discovery/token 交换——无独立 backchannel 配置项（不像 BC 的 `JSL_AUTHZ_BACKCHANNEL_BASE`）。

`KC_HOSTNAME=http://auth.localtest.me` 后，discovery 文档里所有 URL（issuer/token_endpoint/jwks_uri）都指向 `auth.localtest.me`。门户 pod 内必须能解析该 hostname → APISIX ClusterIP。

CoreDNS ConfigMap 增 `rewrite name auth.localtest.me apisix.apisix.svc.cluster.local`，pod 内 `auth.localtest.me` 解析到 APISIX。

`portal.localtest.me` 不需 rewrite——只有浏览器访问门户用，pod 内无需解析。

### 3. kind 端口映射

```yaml
kind: Cluster
nodes:
- role: control-plane
  extraPortMappings:
  - containerPort: 30080  # APISIX NodePort
    hostPort: 80           # 浏览器 http://*.localtest.me → kind → APISIX
```

APISIX Helm values 固定 HTTP NodePort=30080（`gateway.http.nodePort: 30080`），使 kind 映射可复现。

### 4. issuer 对齐矩阵

| 调用方 | env | 值 | 可达性 |
|---|---|---|---|
| 浏览器 authorize | `AUTH_KEYCLOAK_ISSUER` | `http://auth.localtest.me/realms/jsl` | localtest.me→127.0.0.1:80→kind→APISIX→Keycloak ✓ |
| 门户 Auth.js discovery/token | 同上 | 同上 | CoreDNS rewrite→APISIX ClusterIP→Keycloak ✓ |
| 门户 BFF 调 API | `APISIX_API_BASE` | `http://auth.localtest.me/api` | CoreDNS rewrite→APISIX `/api/*`→OIDC→tenant-bc ✓ |
| Keycloak 令牌 iss | `KC_HOSTNAME` | `http://auth.localtest.me` | 令牌 iss=`http://auth.localtest.me/realms/jsl` ✓ |
| APISIX openid-connect | discovery | `http://keycloak.keycloak.svc.../...configuration` | 集群内直连 Keycloak ✓ |
| BC 验签（jwks 模式时） | `JSL_AUTHZ_ISSUER` | `http://auth.localtest.me/realms/jsl` | iss 字符串校验 ✓ |
| BC backchannel | `JSL_AUTHZ_BACKCHANNEL_BASE` | `http://keycloak.keycloak.svc.../realms/jsl` | 集群内直连 Keycloak ✓ |

### 5. dev 不装 Kyverno

kind dev 不安装 Kyverno 与策略集。portal Helm chart 仍按生产基线编写（SA/securityContext/probes/labels/replicas=2），dev values override（replicas=1、pullPolicy=Never、本地镜像）。生产部署时 ArgoCD 同步 Kyverno 策略集后 chart 自动合规。

## 变更文件

### 新增（14）

**门户 Helm Chart（仿 `deploy/helm/tenant-bc/` 模式）：**
- `deploy/helm/tenant-portal/Chart.yaml` — name/version/appVersion
- `deploy/helm/tenant-portal/values.yaml` — 生产基线：replicaCount=2、GHCR 镜像、env（issuer 对齐值）、existingSecret、resources、autoscaling、probes、serviceAccount.create=true
- `deploy/helm/tenant-portal/templates/_helpers.tpl` — name/labels/selectors
- `deploy/helm/tenant-portal/templates/serviceaccount.yaml` — 专用 SA（Kyverno disallow-default-sa 合规改进）
- `deploy/helm/tenant-portal/templates/configmap.yaml` — 非敏感 env ConfigMap
- `deploy/helm/tenant-portal/templates/deployment.yaml` — Kyverno 全合规：securityContext(runAsNonRoot/runAsUser=1001/seccompProfile)、automountServiceAccountToken=false、probes(/healthz)、resources、emptyDir /tmp(sizeLimit)、labels(name+version)
- `deploy/helm/tenant-portal/templates/service.yaml` — ClusterIP，port 3000
- `deploy/helm/tenant-portal/templates/hpa.yaml` — HPA v2（.Values.autoscaling.enabled 守卫）

**dev values + GitOps + kind：**
- `deploy/dev/tenant-portal-dev-values.yaml` — replicaCount=1、image=jsl/tenant-portal:v0.0.1、pullPolicy=Never、HPA off、env（同基线 issuer 对齐值）、existingSecret
- `deploy/gitops/argocd/tenant-portal.yaml` — ArgoCD Application：source=deploy/helm/tenant-portal、valueFiles=dev values、namespace=platform
- `deploy/dev/kind-cluster.yaml` — kind config：extraPortMappings 80→30080
- `deploy/dev/coredns-rewrite-patch.yaml` — CoreDNS ConfigMap 全量 Corefile + rewrite 规则

**门户 app：**
- `web/tenant-portal/app/healthz/route.ts` — `GET() → Response("ok", 200)`，无 auth

### 修改（5）

- `web/tenant-portal/middleware.ts` — matcher 增 `healthz` 排除，探针返回 200 而非 302
- `deploy/gateway/apisix/manifests/apisix-standalone-configmap.yaml` — 现有 3 路由增 `hosts: ["auth.localtest.me"]`；新增 portal 路由（`hosts: ["portal.localtest.me"]`, uri=`/*`, upstream=`tenant-portal.platform.svc:3000`）
- `deploy/gateway/apisix/values.yaml` — 固定 HTTP NodePort=30080
- `deploy/identity/keycloak/values.yaml` — extraEnvVars 增 `KC_HOSTNAME=http://auth.localtest.me`
- `deploy/identity/keycloak/manifests/realm-configmap.yaml` — jsl-gateway client `redirectUris` 增 `http://portal.localtest.me/*`
- `deploy/identity/keycloak/jsl-realm.json` — 同步增 redirectUri（compose 一致性）
- `deploy/dev/tenant-bc-dev-values.yaml` — JWKS 注释更新为 K8s issuer 值（参考用，仍 stub 模式）
- `deploy/README.md` — 目录树补 `helm/tenant-portal/`

## 实施顺序

1. 门户 `/healthz` 端点 + middleware 排除（app 改动先行，独立可测）
2. 门户 Helm Chart（Chart.yaml → values → _helpers → configmap → serviceaccount → deployment → service → hpa）
3. dev values + kind-cluster.yaml + coredns-rewrite-patch.yaml
4. APISIX standalone configmap 增 hostname 路由 + portal 路由
5. APISIX values 固定 NodePort
6. Keycloak values 增 KC_HOSTNAME + realm redirectUri
7. ArgoCD Application
8. 本地 `docker build` + `kind create` + `kind load`
9. ArgoCD 同步 → 全 Deployment ready
10. 浏览器 `http://portal.localtest.me/login` → Keycloak → demo-admin/dev → 回门户 → 见租户/站点
11. 更新计划记录 + commit

## 验证清单

- [x] `docker build -t jsl/tenant-portal:v0.0.1 web/tenant-portal` 成功（Next.js 15.5.25 standalone，8 路由全部编译）
- [x] `kind create cluster --config deploy/dev/kind-cluster.yaml` + `kind load docker-image` 成功（集群名 m20）
- [x] CoreDNS rewrite 生效：pod 内 `auth.localtest.me` → apisix.apisix.svc（portal pod 内 wget discovery 200）
- [x] Helm 直装 4 个 release（keycloak、apisix、tenant-bc、tenant-portal；m20 dev 未装 ArgoCD/Kyverno）
- [x] 全 Deployment ready：keycloak / apisix / tenant-bc / tenant-portal 均 Running
- [x] 门户健康探针：`http://portal.localtest.me:8080/healthz` 返回 200
- [x] 浏览器 `http://portal.localtest.me:8080` 返回门户 HTML
- [x] `http://auth.localtest.me:8080/realms/jsl/.well-known/openid-configuration` 返回 200，issuer=`http://auth.localtest.me:8080/realms/jsl`
- [x] 浏览器 OIDC 登录：portal.localtest.me/login → Keycloak（client_id=jsl-gateway、PKCE S256）→ demo-admin/dev → 回门户仪表盘
- [x] 仪表盘/租户列表/租户详情/站点树/站点详情 全部渲染 Demo 数据
- [x] tenant-bc 审计日志：audit.tenant_id=01JSM18DEM0TENANT000000001、audit.subject=Keycloak sub，GetTenant/ListSites/GetSite 全 success

## 实施记录（2026-09-07 冒烟修正）

浏览器冒烟发现并修复 3 个链路问题（均已落盘）：

1. **Server Actions 被 APISIX 头篡改中止**：APISIX 默认 `X-Forwarded-Host` 取 nginx `$host`（剥端口），
   与浏览器 `Origin: portal.localtest.me:8080` 不匹配，Next.js 以
   `Invalid Server Actions request` 拒绝登录表单。
   修复：portal 路由加 `proxy-rewrite.headers.set.X-Forwarded-Host: "$http_host"`（保留端口）。
2. **集群内 issuer 端口不可达**：issuer 面为 `auth.localtest.me:8080`（宿主 80 被 Docker Desktop
   占用，kind 映射 8080→30080），但 APISIX Service port=9080，CoreDNS rewrite 后门户 pod 访问
   `:8080` 失败，Auth.js discovery `fetch failed` 回退默认 client_id（`portal`）→
   callback `?error=Configuration`。
   修复：APISIX Service port 改 8080（targetPort=9080、nodePort=30080 不变），
   即 `gateway.http.servicePort: 8080`。
3. **tenant-bc 仍为 stub 验签**：门户令牌（RS256 JWT）过 APISIX 后被 BC stub HMAC 拒绝
   （`platform.token_invalid`）。
   修复：`deploy/dev/tenant-bc-dev-values.yaml` 切 `JSL_AUTHZ_MODE: "jwks"`（issuer 与浏览器面
   一致，backchannel 直连 `keycloak.keycloak.svc:80`），`helm upgrade` + rollout restart。

**已知限制（后续里程碑）**：realm `accessTokenLifespan=300`（5 分钟），Auth.js jwt callback 未实现
refresh token 轮换，过期后 APISIX 返回 401（openresty HTML），需重新登录。后续按 Auth.js Keycloak
refresh 模式在 `auth.ts` jwt callback 增 `refreshAccessToken()`。

## 风险与应对

1. **APISIX NodePort 固定失败**：apache/apisix chart 2.15 的 nodePort 键名不确定。应对：`helm show values` 确认键名；若不支持固定，用动态 NodePort 并相应调整 kind 映射。
2. **CoreDNS ConfigMap 覆盖丢 kind 定制**：kind 默认 Corefile 跨版本稳定。应对：先 `kubectl get cm coredns -o yaml` 备份，patch 后验证。
3. **Keycloak realm 重导入幂等**：`--import-realm` 用 IGNORE_EXISTING，realm 已存在时 redirectUri 变更不生效。应对：删 keycloak-0 pod 强制重导，或先经 admin CLI 删 realm。
4. **Next.js readOnlyRootFilesystem**：standalone 可能写 `.next/cache`（ISR）或 `/tmp`（server actions）。应对：emptyDir 挂 `/tmp`（sizeLimit=100Mi）；当前门户页面动态无 ISR，`/tmp` 足够。
5. **门户镜像未签名**：dev 本地构建无 cosign 签名。应对：kind 不装 Kyverno，无签名校验；生产前补 CI 构建+签名（后续里程碑）。

## 范围外后续项

- `ci.yml` 增 portal-image-build job（GHCR 推送 + cosign 签名，仿 tenant-bc 模式）
- ArgoCD Application 切换 image 为 GHCR + Always
- ~~BC dev values 切 jwks 模式~~（冒烟阶段已提前完成，见实施记录 3）
- 门户 Auth.js refresh token 轮换（access token 5 分钟过期后自动续期，见已知限制）
- 生产 TLS：cert-manager + K8s Ingress 资源替代 APISIX standalone 路由
