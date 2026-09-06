# deploy/kyverno/ —— 镜像签名准入（M7）

> 依据：[BP-05 §7.1.2 供应链安全](../blueprint/05-security-architecture.md)（包签名验证、镜像扫描、SBOM）
> 这是 CI 侧 cosign keyless 签名（`.github/workflows/ci.yml` ⑤ 制品 job）的**运行时消费端**。

## 作用

强制 Pod 镜像必须具备由 GitHub Actions OIDC 签发的 cosign keyless 签名，防止未签名或被篡改的镜像进入集群。验证通过后自动将镜像标签替换为 digest（`mutateDigest: true`），防止标签重放攻击。

## 目录结构

```
deploy/kyverno/
├── values.yaml                    # Kyverno Helm 安装 values（镜像验证 Enforce）
└── policies/
    └── verify-image-signatures.yaml   # ClusterPolicy：cosign keyless 签名校验
```

## 安装（生产）

### 1. 安装 Kyverno

```bash
helm repo add kyverno https://kyverno.github.io/kyverno
helm repo update kyverno
helm install kyverno kyverno/kyverno -n kyverno --create-namespace \
  -f deploy/kyverno/values.yaml
```

Kyverno 需能访问目标镜像仓库（GHCR）与 Rekor 透明日志（`https://rekor.sigstore.dev`）。

### 2. 应用策略

```bash
kubectl apply -f deploy/kyverno/policies/verify-image-signatures.yaml
```

### 3. GitOps（ArgoCD）

集群装好 ArgoCD 后，apply 本仓：
- `deploy/gitops/argocd/kyverno.yaml` —— 安装 Kyverno（官方 Helm）
- `deploy/gitops/argocd/kyverno-policies.yaml` —— 同步本仓策略集

## 本地验证（kind）

```bash
kind create cluster --name m7
helm install kyverno kyverno/kyverno -n kyverno --create-namespace -f deploy/kyverno/values.yaml
kubectl apply -f deploy/kyverno/policies/verify-image-signatures.yaml

# ✅ 已签镜像：放行 + digest 固定
kubectl run ok --image=ghcr.io/jaculi/tenant-bc:latest --command -- /server

# ❌ 未签镜像：拒绝
#   admission webhook ... failed to verify image docker.io/nginx:alpine: no signatures found
kubectl run bad --image=nginx:alpine
```

## 策略约束

| 字段 | 值 | 与 CI 对应 |
|---|---|---|
| `imageReferences` | `ghcr.io/jaculi/tenant-bc:*` | 首个 BC；后续扩为 `ghcr.io/jaculi/*:*` |
| `issuer` | `https://token.actions.githubusercontent.com` | CI OIDC 发行者 |
| `subject` | `https://github.com/jaculi/Hello-World/*` | 仅本仓 workflow 签名可信 |
| `rekor.url` | `https://rekor.sigstore.dev` | 公共透明日志（签名存在性校验） |
| `mutateDigest` | `true` | 验证通过后 pin 到 digest |
| `validationFailureAction` | `Enforce` | 未通过即拒绝（fail-closed） |

> 注意：Kyverno 归一化 Docker Hub 镜像为 `docker.io/<name>:<tag>`（不含 `library/`），配置 `imageReferences` 时不要加 `library/` 前缀。
