# deploy/identity/ —— 身份基础设施（M18）

> 依据：[BP-04 §4.4 身份服务选型](../docs/blueprint/04-technology-architecture.md)（Keycloak 自托管 OIDC）、[BP-05 §3 IAM](../docs/blueprint/05-security-architecture.md)

## 目录结构

```
deploy/identity/
├── README.md
└── keycloak/
    ├── jsl-realm.json          # realm 引导声明（唯一事实源；compose 直接挂载）
    ├── values.yaml             # 官方 Helm chart values（dev：start-dev + 内置 PG + 单副本）
    └── manifests/
        └── realm-configmap.yaml  # 集群用 ConfigMap（ArgoCD 同步，含 realm JSON）
```

## 本地开发（docker compose）

compose 栈（[deploy/dev/docker-compose.yml](../dev/docker-compose.yml)）已包含 Keycloak：

```bash
docker compose -f deploy/dev/docker-compose.yml up -d keycloak
# 管理台：http://localhost:8081 （admin/admin，仅 dev）
# realm jsl 由 --import-realm 自动导入
```

jsl realm 预置内容：

- client **jsl-gateway**（confidential，secret `dev-gateway-secret`；密码模式 + code 流）
- 协议映射：用户属性 `tenant_id` → 令牌 claim `tenant_id`（+ userinfo）；Audience 映射补 `aud=jsl-gateway`
- 用户 **demo-admin / dev**，属性 `tenant_id=01JSM18DEM0TENANT000000001`，角色 `TENANT_ADMIN`

> Keycloak 26 访问令牌默认**不含 `aud` 声明**，需 oidc-audience-mapper，否则服务端受众校验失败。

## 集群部署（GitOps）

- ArgoCD Application：[deploy/gitops/argocd/keycloak.yaml](../gitops/argocd/keycloak.yaml)
  多源同步：官方 chart（`https://keycloak.github.io/helm-charts`，26.1.*）+ 本仓 values + realm ConfigMap
- Namespace `keycloak` 由 [deploy/gitops/namespaces/identity.yaml](../gitops/namespaces/identity.yaml) 创建（带 Kyverno 要求标签）

## 限制（dev）

- `--import-realm` 策略 IGNORE_EXISTING：realm JSON 变更后需删除容器/重导（compose 数据在容器内，重建即重导；生产用 keycloak-config-cli 或 BC-P2 管理）
- admin/admin、client secret 均为 dev 明文；生产必须外部化 Secret + 启用 TLS（Kyverno 已有 Ingress TLS 策略）
- 租户开通 → Keycloak 用户自动化、企业 IdP 联邦、MFA 强制均为 BC-P2 IAM 范围
