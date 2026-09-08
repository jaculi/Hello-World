# M21 门户 CI 构建 + cosign 签名 实施计划

## Context

M20 完成门户 K8s 部署，浏览器登录端到端跑通。但门户镜像的 CI 构建、GHCR 推送、cosign 签名仍为
预留项（M20 计划范围外）。生产基线 `deploy/helm/tenant-portal/values.yaml` 已指向
`ghcr.io/jaculi/tenant-portal`，但 CI 尚未产出该镜像。

**M21 目标**：在 `.github/workflows/ci.yml` 增加门户质量门禁与镜像构建 job，镜像推 GHCR 并
cosign keyless 签名，与 tenant-bc 制品链对齐。

## 蓝图依据

- BP-04 §6：GitOps（ArgoCD）同步各 BC 与基础设施；镜像须有来源可追溯
- BP-05 §3.1：前端令牌不入浏览器存储（已在 M19 实现 httpOnly cookie）
- BP-07：供应链安全——cosign keyless 签名 + Fulcio 证书验证
- ADR-0003 决策：Go 后端 + Python 推理；前端 React/Next.js，CI 独立构建

## 变更

### 新增（1）

- `web/tenant-portal/.dockerignore` — 排除 node_modules、.next、.env 等，缩小构建上下文

### 修改（1）

- `.github/workflows/ci.yml` — 增两个 job：
  - `portal-build`（⑦）：`npm ci` + `npm run build`（next build 含 TypeScript 编译），
    所有 push/PR 触发，作门户质量门禁
  - `portal-image-build`（⑧）：仿 tenant-bc `image-build`——
    - docker build（context=web/tenant-portal，file=web/tenant-portal/Dockerfile）
    - master push 推 GHCR `ghcr.io/<owner>/tenant-portal`（tag: sha-* + latest）
    - PR 仅本地 load 不推
    - master push 时 cosign keyless 签名（digest）+ verify（Fulcio 证书 + GitHub Actions 发行者正则）
    - 制品冒烟：容器启动后 `curl /healthz` 返回 `ok`

## 与 tenant-bc 制品链对齐

| 维度 | tenant-bc | tenant-portal |
|---|---|---|
| 质量门禁 | golangci-lint + go test | npm ci + next build |
| 镜像构建 | services/tenant/Dockerfile（distroless） | web/tenant-portal/Dockerfile（node:22-alpine） |
| GHCR 镜像 | ghcr.io/<owner>/tenant-bc | ghcr.io/<owner>/tenant-portal |
| 标签 | sha-<short> + latest（master） | 同 |
| 签名 | cosign keyless（master） | 同 |
| 验证 | Fulcio + GitHub Actions issuer | 同 |
| 冒烟 | /healthz 返回 SERVING | /healthz 返回 ok |

## 验证

- [x] ci.yml YAML 语法正确（GitHub Actions 解析通过，run 34219114415）
- [x] `npm run build` 本地通过（Next.js 15.5.25，8 路由编译 + TypeScript 类型检查）
- [x] 镜像可本地构建并启动，/healthz 返回 200 ok（node:22-alpine，非 root nextjs 用户，679ms ready）
- [x] master push（c22c1c1）后 CI 8 job 全绿：⑧ portal-image-build 十步全过
      （构建 → GHCR 推 sha-*/latest → cosign keyless 签名 → Fulcio 证书验证 → /healthz 冒烟）

## 执行记录（2026-09-08）

- 本地验证：npm run build 90s 通过；docker build 成功；容器冒烟 /healthz=ok HTTP 200
- 推送 master 后 CI run 34219114415 全部 8 job success，新增 ⑦⑧ 两 job 首跑即绿
- 门户制品链与 tenant-bc 对齐完成：GHCR `ghcr.io/jaculi/tenant-portal`（sha-<short> + latest），
  cosign keyless 签名（Fulcio + GitHub Actions OIDC），Kyverno 镜像验证策略可直接沿用

## 范围外

- ArgoCD Application 切换 image 为 GHCR + Always（dev 仍用本地镜像；生产 values 已指向 GHCR）
- 门户 Auth.js refresh token 轮换（另立里程碑）
- 生产 TLS + Ingress（另立里程碑）
