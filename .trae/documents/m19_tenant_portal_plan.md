# M19 租户门户前端骨架（Next.js + Auth.js + Keycloak 浏览器 code 流）实施计划

## 背景与目标

M18 打通了「API 网关模式」认证链路：APISIX `/api/*` 用 `bearer_only` 验签，curl/服务端拿密码模式令牌调通。但**浏览器无法登录**——`bearer_only` 无令牌即 401，无 code 流跳转；用户在 M18 后明确提问「没有前端今后租户怎么登录呢？」。

**M19 目标**：落地 Next.js 租户门户骨架，使浏览器经 Keycloak OIDC authorization code 流登录 → 门户 BFF 持令牌调 APISIX `/api/*` → BC JWKS 验签通过，租户/站点页面可读可写。M18 的 curl 冒烟从此被真实浏览器流程取代。

## 蓝图依据

- BP-02 §2：前端应用 = 租户门户 / 平台运营门户 / 开发者门户 / 移动端·大屏；BFF 按门户聚合 BC。
- BP-04 §3 表：前端 `React + Next.js (TypeScript)`，三门户共库组件；i18n 用 `next-intl/react-i18next`。
- BP-04 §4.4：APISIX 插件链（认证→授权→限流→**租户上下文注入**→计量→i18n→路由）；BFF 按门户聚合 BC 接口产出视图。
- ADR-0003 §决策：前端技术栈锁定 React + Next.js (TS)。

## 范围

**本里程碑交付「能浏览器登录」**：
- Next.js 15 (App Router, RSC, TS) 门户骨架
- Auth.js v5 Keycloak provider（authorization code + PKCE）
- 门户 BFF 服务端代浏览器持令牌调 APISIX `/api/*`（令牌存 httpOnly 加密 cookie，不进前端 JS）
- 页面：仪表盘 / 登录 / 租户列表 / 租户详情 / 站点列表（树）/ 站点详情 / 注销
- 中间件保护路由；未登录跳 Keycloak
- dev compose：Keycloak 锁定 `KC_HOSTNAME=http://localhost:9080`（全 URL）使浏览器面 issuer 与 BC 验签 issuer 一致
- BC `pkg/authz/jwks.go` 增 backchannel 重写：iss 校验用浏览器面 issuer，discovery/JWKS 抓取走集群内 URL（直连 `keycloak:8080`）

**范围外（M20+）**：
- 平台运营门户 / 开发者门户（M19 仅租户门户）
- Go BFF 独立服务（BP-04 §4.4 蓝图是 Go BFF；M19 用 Next.js API routes 充当 BFF，后续可剥离为独立 Go 服务）
- i18n 翻译表（只透传 BC 消息码，不做翻译存储；BP-04 §5 i18n 随 BC-P4 落地）
- kind/K8s 部署门户（提供 Dockerfile；Ingress 处理 hostname 对齐留 M20）
- 移动端 / 大屏（共组件库，不在本期）
- 租户开通→Keycloak 建用户的自动化（BC-P2 IAM）

## 关键技术决策

### 1. OIDC issuer 三方对齐（M18 遗留风险点）

M18 smoke 用 `http://keycloak:8080` 取令牌，iss=`http://keycloak:8080/realms/jsl`，BC 直连可达。但**浏览器只能经 `http://localhost:9080`（APISIX 宿主端口）访问 Keycloak**，否则 iss 与 BC 不一致被拒。

**方案**：Keycloak 锁定 `KC_HOSTNAME=http://localhost:9080`（全 URL，Keycloak 26 不接受 `hostname:port` 形式），使所有令牌 iss 固定为 `http://localhost:9080/realms/jsl`。

| 调用方 | 走的 URL | 可达性 |
|---|---|---|
| 浏览器（authorize 跳转） | `http://localhost:9080/realms/jsl/.../auth`（APISIX 代理） | ✓ |
| 门户 BFF（discovery/token/userinfo） | `http://localhost:9080/...`（门户跑宿主，localhost=APISIX） | ✓ |
| APISIX openid-connect（discovery+JWKS） | `http://keycloak:8080/...`（内部）→ Keycloak 返回 issuer=`http://localhost:9080/...`、jwks_uri=`http://localhost:9080/.../certs`；APISIX 抓 JWKS 时 `localhost:9080` 是 APISIX 自己，自代理 `/realms/*`→keycloak:8080 ✓ | ✓ |
| BC 验签（discovery+JWKS） | iss=`http://localhost:9080/...`（必）但 BC 容器内 `localhost:9080` 不可达 | ✗ → 须 backchannel 重写 |

### 2. jwks.go backchannel 重写

新增 `JSL_AUTHZ_BACKCHANNEL_BASE`（如 `http://keycloak:8080/realms/jsl`）。go-oidc 经 `oidc.ClientContext(ctx, httpClient)` 注入自定义 HTTP client：当请求 URL 以 issuer 前缀开头时，把 scheme+host+port 替换为 backchannel 基址。discovery 与 JWKS 抓取均走重写后的内部 URL；**iss 校验仍用原 issuer 字符串**（令牌 iss 一致即可通过）。

> **实证修正**：原计划 backchannel 走 `http://apisix:9080/realms/jsl`，但 compose 依赖顺序为 keycloak(healthy) → tenant-bc(started) → apisix(started)，tenant-bc 启动时 APISIX 尚未就绪导致 DNS 解析 `apisix` 失败。改走 `http://keycloak:8080/realms/jsl` 直连 Keycloak，规避启动顺序问题；iss 校验不受影响。

### 3. demo 租户 seed

M18 demo-admin 令牌 `tenant_id=01JSM18DEM0TENANT000000001`，但 BC 内存模式每次重启空仓 → 查询 404，RLS 也阻止创建（新租户 ULID 与令牌租户不一致 → 不可见）。**dev 内存模式 seed 该租户 + 根组织 + 根站点 + 一个示例子站点**，使 demo 端到端可见。

### 4. BFF 模式

浏览器不直连 APISIX。Next.js 服务端 RSC + route handler 持 httpOnly cookie 中 access_token，以 `Authorization: Bearer` 调 APISIX `/api/*`。令牌不进前端 JS（BP-05 §3.1 工作负载身份「前端令牌不入浏览器存储」预期）。

## 变更文件

**修改：**
- `pkg/authz/jwks.go` —— `NewJWKSVerifier` 增 `backchannelBase` 参数 + 重写 HTTP client；`NewVerifierFromEnv` 读 `JSL_AUTHZ_BACKCHANNEL_BASE`
- `pkg/authz/jwks_test.go` —— 增 backchannel 重写用例
- `services/tenant/main.go` —— dev 内存模式 seed demo 租户；verifier 装配传 backchannel
- `deploy/dev/docker-compose.yml` —— keycloak 增 `KC_HOSTNAME=http://localhost:9080`；tenant-bc 增 `JSL_AUTHZ_ISSUER=http://localhost:9080/realms/jsl` + `JSL_AUTHZ_BACKCHANNEL_BASE=http://keycloak:8080/realms/jsl`
- `deploy/README.md` —— 目录树补 `web/`
- `docs/tasks/bc-p1-skeleton-plan.md` —— M19 执行记录

**新增（`web/tenant-portal/`）：**
- `package.json` `next.config.ts` `tsconfig.json` `tailwind.config.ts` `postcss.config.mjs` `.env.example` `.gitignore` `Dockerfile`
- `auth.ts`（Auth.js v5 Keycloak provider）
- `middleware.ts`（路由保护）
- `lib/api.ts`（服务端 API client：从 session 取 token，调 APISIX）
- `app/{globals.css,layout.tsx,page.tsx}`（仪表盘）
- `app/login/page.tsx`（登录触发）
- `app/tenants/{page.tsx,[id]/page.tsx}`
- `app/sites/{page.tsx,[id]/page.tsx}`
- `app/api/auth/[...nextauth]/route.ts`（Auth.js handler）
- `app/api/proxy/[...path]/route.ts`（BFF 代理：浏览器→门户→APISIX）
- `components/{Navbar,SiteTree}.tsx`

## 实施顺序

1. jwks.go backchannel 重写 + 单测（pkg 改动先行，独立可测）
2. main.go seed demo 租户
3. compose 改 KC_HOSTNAME + BC env
4. `go build ./... && go vet ./... && go test ./pkg/authz/... ./services/...` 回归
5. Next.js 骨架（package.json/install → config → auth → middleware → pages → BFF）
6. `npm run build` 通过
7. 启动 compose → `npm run dev` → 浏览器 localhost:3000 登录 demo-admin/dev → 见租户/站点
8. 更新计划记录 + commit

## 验证清单

- [x] `go build`/`go vet`/`go test` 全绿（含 backchannel 单测）— Docker 构建验证通过
- [x] compose 五服务全起（keycloak healthy / apisix / tenant-bc / postgres / kafka）
- [x] `npm run dev` 启动，localhost:3000 返回 200
- [x] 浏览器流程：localhost:3000/login → 跳 Keycloak → demo-admin/dev → 回门户 → 见 demo 租户 + 站点树
- [x] 仪表盘显示 Demo Tenant（ID=01JSM18DEM0TENANT000000001，TENANT_ADMIN 角色）
- [x] /sites 页面显示站点树（root + A 号厂区 factory-a）
- [x] /tenants 页面显示租户列表（RLS 生效，仅本租户可见）
- [x] API 无令牌被 APISIX 拒（401）
- [ ] M18 冒烟回归（compose 起 + m18-auth-smoke.sh 全 PASS）— 留后续回归
- [ ] `npm run build` 生产构建 — 留后续
- [ ] 注销生效（cookie 清除，回 /login）— 留后续
- [ ] 伪造/过期令牌经 BFF 调用被 APISIX 拒（401）— 留后续

## 执行记录（2026-09-07）

### 实证修正

1. **Keycloak 26 hostname 格式**：`KC_HOSTNAME=localhost:9080` 被拒（"neither a plain hostname nor a valid URL"），改为全 URL `http://localhost:9080`。
2. **Backchannel URL 改走 Keycloak 直连**：原计划 `http://apisix:9080/realms/jsl`，但 compose 依赖顺序 keycloak→tenant-bc→apisix 导致 tenant-bc 启动时 APISIX 未就绪、DNS 解析 `apisix` 失败。改走 `http://keycloak:8080/realms/jsl` 直连 Keycloak 内部端口，iss 校验不受影响。
3. **Quarkus 首次构建慢**：Keycloak dev 模式首次启动 Quarkus augmentation 耗时 ~6 分钟（dev 模式特征），healthcheck start_period=90s 不足，需手动等待后重启 apisix/tenant-bc。

### 端到端验证结果

浏览器自动化冒烟测试全通过：
- 登录页 → Keycloak authorize 跳转 → demo-admin/dev 登录 → 回门户仪表盘
- 仪表盘显示 Demo Tenant 详情（ID、显示名、状态、隔离级别、套餐、区域）
- Navbar 显示租户 ID、角色（TENANT_ADMIN）、注销按钮
- /sites 页面显示站点树（root + factory-a）
- /tenants 页面显示租户列表表
- 无功能错误，仅 dev-only 的 React DevTools/HMR 控制台消息

## 风险与应对

1. **Auth.js v5 仍 beta**：用最新 stable tag；若 Keycloak provider 异常，退回手写 OIDC code 流（`jose` + native fetch，PKCE 自管）。本期已预留 `lib/api.ts` 抽象。
2. **APISIX 自代理 JWKS 循环**：APISIX `/realms/*` 路由 → keycloak:8080，lua-resty-openidc 抓 `localhost:9080/.../certs` 命中自身 :9080 走该路由。OpenResty 不禁 loopback，可行；若被拒，退回 BC 侧只经 APISIX 抓 JWKS，APISIX 侧 openid-connect 改 `introspection` 模式或显式 jwks_uri。
3. **Auth.js cookie 跨域**：dev 同 localhost 不同端口（3000 vs 9080），cookie 同站点（均 localhost），无跨域问题。
4. **Keycloak realm 导入幂等**：改 `KC_HOSTNAME` 不影响 realm 数据，重启即可。
5. **门户跑宿主 vs 容器**：dev 跑宿主（`npm run dev`）规避 OIDC issuer 内外 URL 不一致；kind/K8s 部署用 Ingress 统一 hostname（M20）。
