# deploy/ —— 部署与 GitOps 目录

> 依据：[BP-04 §6 部署与发布](../blueprint/04-technology-architecture.md)、[BP-07 §2.2 Cell 架构](../blueprint/07-deployment-and-operations.md)、[BP-05 §7.1.2 供应链安全](../blueprint/05-security-architecture.md)

## 目录结构

```
deploy/
├── dev/                          # 本地开发依赖栈（compose + helm dev values）
│   ├── docker-compose.yml        # PostgreSQL + Kafka（KRaft）+ Keycloak + APISIX + tenant-bc
│   ├── m18-auth-smoke.sh         # 认证链路冒烟脚本（compose 网络内运行）
│   └── tenant-bc-dev-values.yaml # kind 本地部署覆盖值（内存模式）
├── helm/
│   └── tenant-bc/                # BC Helm Chart（探针/HPA/资源/安全上下文）
├── kyverno/                      # 准入控制（M7-M17 二十三层纵深防御，CEL 策略）
│   ├── values.yaml               # Kyverno Helm 安装 values（HA + PolicyException）
│   ├── README.md                 # 策略说明与实证矩阵
│   ├── policies/                 # CEL 策略集
│   └── exceptions/               # 策略例外模板（PolicyException，须评审+过期）
├── identity/                     # 身份基础设施（M18，见 identity/README.md）
│   └── keycloak/                 # Keycloak realm 声明 + Helm values + ConfigMap
├── gateway/                      # API 网关（M18，见 gateway/README.md）
│   └── apisix/                   # APISIX standalone 路由/插件 + Helm values
└── gitops/
    ├── argocd/                   # ArgoCD Application 清单
    │   ├── kyverno.yaml          # 安装 Kyverno
    │   ├── kyverno-policies.yaml # 同步策略集
    │   ├── keycloak.yaml         # 安装 Keycloak（M18）
    │   ├── apisix.yaml           # 安装 APISIX（M18）
    │   └── tenant-bc.yaml        # 部署 tenant-bc
    └── namespaces/
        └── identity.yaml         # keycloak/apisix 命名空间（带 Kyverno 要求标签）
```

## 关键链路

- **制品**：CI 构建 distroless 镜像 → 推 GHCR → cosign keyless 签名（Fulcio + Rekor）
- **准入**：Kyverno CEL 策略校验 cosign 签名（issuer = GitHub Actions，subject = 本仓），通过后 pin digest
- **认证**：Keycloak 签发 OIDC 令牌 → APISIX openid-connect bearer 验签 + Lua 注入租户头 → BC JWKS 二次验签（M18）
- **GitOps**：ArgoCD 同步 Kyverno、策略集、身份/网关、各 BC

各 BC 的 Helm Chart 随服务放在 `services/<bc>/deploy/helm/<bc>/`，GitOps 目录只引用制品版本（Tag = Chart appVersion 策略，BP-04 §6）。
