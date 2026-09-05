# deploy/ —— 部署与 GitOps 目录

> 依据：[BP-04 §6 部署与发布](../blueprint/04-technology-architecture.md)、[BP-07 §2.2 Cell 架构](../blueprint/07-deployment-and-operations.md)

## 规划结构（阶段 6 / 步 23-24 填充）

```
deploy/
├── gitops/
│   └── dev/                  # dev 环境（k3d/kind）
│       └── apps/             # ArgoCD Application 清单（每个 BC 一个）
└── compose/
    └── docker-compose.yaml   # 本地开发依赖栈（PostgreSQL/Kafka/OTel Collector）
```

各 BC 的 Helm Chart 随服务放在 `services/<bc>/deploy/helm/<bc>/`，GitOps 目录只引用制品版本（Tag = Chart appVersion 策略，BP-04 §6）。
