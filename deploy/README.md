# deploy/ —— 部署与 GitOps 目录

> 依据：[BP-04 §6 部署与发布](../blueprint/04-technology-architecture.md)、[BP-07 §2.2 Cell 架构](../blueprint/07-deployment-and-operations.md)、[BP-05 §7.1.2 供应链安全](../blueprint/05-security-architecture.md)

## 目录结构

```
deploy/
├── dev/                          # 本地开发依赖栈（compose + helm dev values）
│   ├── docker-compose.yml        # PostgreSQL + Kafka（KRaft）
│   └── tenant-bc-dev-values.yaml # kind 本地部署覆盖值（内存模式）
├── helm/
│   └── tenant-bc/                # BC Helm Chart（探针/HPA/资源/安全上下文）
├── kyverno/                      # 镜像签名准入（M9 生产化，cosign keyless 消费端）
│   ├── values.yaml               # Kyverno Helm 安装 values（准入 3 副本 + 后台 2 副本 HA）
│   ├── README.md                 # 实证矩阵与注意点
│   └── policies/                 # ClusterPolicy：签名校验（全域通配）+ platform 允许清单
└── gitops/
    └── argocd/                   # ArgoCD Application 清单
        ├── kyverno.yaml          # 安装 Kyverno
        ├── kyverno-policies.yaml # 同步策略集
        └── tenant-bc.yaml        # 部署 tenant-bc
```

## 关键链路

- **制品**：CI 构建 distroless 镜像 → 推 GHCR → cosign keyless 签名（Fulcio + Rekor）
- **准入**：Kyverno `ImageVerification` 校验 cosign 签名（issuer = GitHub Actions，subject = 本仓），通过后 pin digest
- **GitOps**：ArgoCD 同步 Kyverno、策略集、各 BC

各 BC 的 Helm Chart 随服务放在 `services/<bc>/deploy/helm/<bc>/`，GitOps 目录只引用制品版本（Tag = Chart appVersion 策略，BP-04 §6）。
