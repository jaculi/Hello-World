# M23 ArgoCD GitOps 落地（kind）实施计划

## Context

M20/M22 租户门户与 BC 均以 `helm install` 直装到 kind，deploy/gitops/argocd 下的 Application
清单只是骨架、从未被真实 ArgoCD 同步。BP-04 §6 要求 GitOps：集群状态以 Git 为唯一事实源，
漂移自动收敛。

**M23 目标**：kind 集群真实安装 ArgoCD，从 GitHub 同步 tenant-bc / tenant-portal 两个
Application，验证 Sync + selfHeal（漂移自动回滚）。

## 前置结论（实证）

- 仓库 **public**（未认证可读 Actions API）→ ArgoCD 无需凭据即可 clone
- GHCR 包 **private**（未认证拉 manifest 返回 401）→ kind 节点无 imagePullSecrets 不能拉
  `ghcr.io/jaculi/*`；dev（kind）同步走 deploy/dev values 声明的本地镜像（kind load 导入）
- GHCR + cosign 验签链属生产 overlay：后续独立 *-prod Application，`tag=sha-<commit>`、
  `pullPolicy=Always`、imagePullSecrets（PAT read:packages）、Kyverno cosign 策略

## Bootstrap（kind 可复现命令）

```bash
# 1. 安装 ArgoCD（非 HA，单节点 kind）
kubectl create namespace argocd
kubectl apply --server-side -n argocd --force-conflicts \
  -f https://raw.githubusercontent.com/argoproj/argo-cd/stable/manifests/install.yaml
#   注意：必须 server-side——application-set CRD 注解超 262144 字节，客户端 apply 会被拒
kubectl -n argocd wait --for=condition=available --timeout=300s \
  deploy/argocd-server deploy/argocd-repo-server deploy/argocd-applicationset-controller

# 2. 注册 Application（dev）
kubectl apply -f deploy/gitops/argocd/tenant-bc.yaml -f deploy/gitops/argocd/tenant-portal.yaml

# 3. 观察
kubectl -n argocd get applications
```

## 变更（3）

- `deploy/gitops/argocd/tenant-portal.yaml` / `tenant-bc.yaml`：移除 GHCR+Always 硬覆盖
  （private 包 dev 拉不动），仅保留跨目录 valueFiles 指向 dev values；生产 overlay 路径写入注释
- `deploy/dev/tenant-portal-dev-values.yaml`：tag v0.0.1 → v0.0.2（含 M22 refresh 轮换）

## 验证清单

- [x] ArgoCD v3.5.2 7 组件全 Running（application-controller / repo-server / server / redis / dex / applicationset / notifications）
- [x] server-side apply 解决 ApplicationSet CRD 注解超长（>262144 bytes）
- [x] 两 Application 自动同步：SYNC STATUS=Synced，HEALTH STATUS=Healthy
- [x] 同步修订 = master HEAD 88e4038（Git 事实源确认）；跨目录 valueFiles（../../../deploy/dev/...）ArgoCD 支持
- [x] ArgoCD 纳管既有 helm 直装资源（apply 采纳，无冲突）
- [x] selfHeal 实证：`kubectl scale deploy/tenant-portal --replicas=3` 制造漂移
      → hard refresh → 副本自动回滚 1，App 恢复 Synced/Healthy

## 执行记录（2026-09-08）

- ArgoCD 镜像走 quay.io（quay.io/argoproj/argocd:v3.5.2），kind 节点可直连拉取
- 现有 helm release secret 保留（孤儿但不冲突）；资源经 ArgoCD tracking 注解纳管
- selfHeal 默认 3 分钟重扫周期；实证用 `argocd.argoproj.io/refresh=hard` 注解即时触发

## 范围外

- 生产 overlay（*-prod Application）：GHCR sha pin + Always + imagePullSecrets + Kyverno 验签
- GitOps bootstrap 自举：把 ArgoCD 自身安装也做成 GitOps（app-of-apps / ApplicationSet）
- ArgoCD UI/SSO：admin 密码与 Keycloak 联邦登录（本里程碑仅用 kubectl 声明式验证）
- TLS + Ingress：cert-manager 与外部 DNS 前置，继续挂起
