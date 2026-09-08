# M22 门户令牌续期 + ArgoCD 镜像切 GHCR 实施计划

## Context

M21 完成门户 CI 制品链（GHCR + cosign）。遗留两项 M20/M21 范围外候选：
1. Auth.js 无 refresh token 轮换——realm `accessTokenLifespan=300`，过期后 APISIX 返回 401，须重登
2. ArgoCD Application 骨架（tenant-portal）仍指向本地镜像，未切 CI 产出的 GHCR 镜像

**M22 目标**：以上两项全落地并浏览器端到端实证；TLS+Ingress 继续挂起（依赖 cert-manager 与外部 DNS 前置）。

## 变更

### 修改（3）

- `web/tenant-portal/auth.ts` — `refreshAccessToken()`（Keycloak refresh_token grant，token 端点
  与 discovery 同 issuer 面可达）；jwt callback 初次登录持久化 `refreshToken/expiresAt`，
  过期前 30s 静默续期并重解码声明（租户/角色变更随新令牌生效）；刷新失败清空令牌置
  `RefreshTokenError`
- `web/tenant-portal/middleware.ts` — `auth()` 包装器：无 session 或令牌被清空（刷新失败）→
  重定向 /login（Keycloak SSO 会话存活时静默回跳，无需重输密码）
- `deploy/gitops/argocd/tenant-portal.yaml` — 仿 tenant-bc：helm parameters 覆盖
  `image.repository=ghcr.io/jaculi/tenant-portal`、`tag=latest`、`pullPolicy=Always`（生产应 pin sha-*）

### 修正（1）

- `deploy/identity/keycloak/manifests/realm-configmap.yaml` — demo-admin 补 `firstName/lastName`
  （实证中发现精简版导入触发 Keycloak VERIFY_PROFILE 必填补全页，首登多一步）

## 验证清单

- [x] `npm run build` 通过（next-auth/jwt 导入 JWT 类型；v5 beta 根模块不导出）
- [x] docker build v0.0.2 + kind load + 滚动升级 m20 集群
- [x] realm accessTokenLifespan 临时调 60s（kcadm 实测）
- [x] 旧会话（无 refresh 字段）→ 刷新失败 → middleware 引导 /login（优雅降级路径实证）
- [x] 新登录后 95s+（原始 60s 令牌彻底过期）/sites 免重登渲染站点树（链式续期实证）
- [x] 刷新失败路径（Keycloak 重启清空内存会话后旧 refresh token 失效）→ 清空令牌 → /login 引导
- [x] realm 寿命恢复 300s，/tenants 与站点详情页回归通过

## 执行记录（2026-09-08）

- kind m20 集群宿主机 Docker 引擎中途重启：keycloak Quarkus 增强耗 306s 重启（dev-file H2
  增强慢为已知教训），tenant-bc discovery 依赖 keycloak 就绪时序、出现暂时 CrashLoopBackOff；
  keycloak 就绪后自动恢复，无需人工干预（最终 delete pod 立即拉起验证）
- Keycloak 重启清空内存会话（Infinispan 不持久）→ 旧 refresh token 失效——恰好实证了
  刷新失败 → 清空令牌 → /login 引导重登的降级链路

## 范围外

- 生产 TLS：cert-manager + K8s Ingress 替代 APISIX standalone 路由（外部 DNS/证书前置）
- GHCR 包 private→kind 拉取需 imagePullSecrets 或 public 包（ArgoCD 真正同步前处理）
