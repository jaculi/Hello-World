# deploy/kyverno/ —— 镜像签名准入（M9 生产化）

> 依据：[BP-05 §7.1.2 供应链安全](../blueprint/05-security-architecture.md)（包签名验证、镜像扫描、SBOM）、BP-05 默认拒绝原则
> 这是 CI 侧 cosign keyless 签名（`.github/workflows/ci.yml` ⑤ 制品 job）的**运行时消费端**。

## 作用

两层准入（纵深防御，均为 Enforce + fail-closed）：

1. **签名校验**（`verify-image-signatures`）：`ghcr.io/jaculi/*:*` 全部平台镜像必须具备由 GitHub Actions OIDC 签发的 cosign keyless 签名（monorepo 内所有 BC 与后续插件行业服务**自动纳入**，新 BC 落地无需改策略）；验证通过后自动 pin 到 digest（`mutateDigest`），防标签重放。后台扫描对存量 Pod 生成 PolicyReport。
2. **允许清单**（`platform-registry-allowlist`）：`platform` 命名空间内全部容器（含 init/ephemeral）只允许 `ghcr.io/jaculi/*` 镜像——签名校验约束"平台镜像必须可信"，本策略反向收敛"platform 只跑平台镜像"，防止业务 Pod 夹带未纳入供应链治理的第三方镜像。第三方组件部署于独立命名空间，不受影响；确需引入时走例外评审。

## 目录结构

```
deploy/kyverno/
├── values.yaml                          # Kyverno Helm 安装 values（准入 3 副本 + 后台 2 副本 HA）
└── policies/
    ├── verify-image-signatures.yaml     # ClusterPolicy：cosign keyless 签名校验（平台全域通配）
    └── platform-registry-allowlist.yaml # ClusterPolicy：platform 命名空间镜像允许清单
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
kubectl apply -f deploy/kyverno/policies/
```

### 3. GitOps（ArgoCD）

集群装好 ArgoCD 后，apply 本仓：
- `deploy/gitops/argocd/kyverno.yaml` —— 安装 Kyverno（官方 Helm，chart 3.9.0 = v1.19.0，与实证版本对齐）
- `deploy/gitops/argocd/kyverno-policies.yaml` —— 同步本仓策略集

## 本地验证（kind）

```bash
kind create cluster --name m9
helm install kyverno kyverno/kyverno -n kyverno --create-namespace -f deploy/kyverno/values.yaml
kubectl apply -f deploy/kyverno/policies/
```

M9 实证矩阵（kind v1.32.2 + Kyverno v1.19.0）：

| 用例 | 结果 |
|---|---|
| 已签 `ghcr.io/jaculi/tenant-bc:latest`（default ns） | ✅ 放行，digest 固定 `@sha256:9d58…` |
| 未签 `nginx:alpine`（default ns，范围外） | ✅ 放行（不受策略影响） |
| 未签 `ghcr.io/jaculi/nonexistent:v0`（default ns） | ❌ 拒绝：`failed to verify image … DENIED`（通配 + fail-closed） |
| 未签 `nginx:alpine`（platform ns） | ❌ 拒绝：允许清单 |
| 已签 `tenant-bc:latest` + 伪造 init 容器 `nginx:alpine`（platform ns） | ❌ 拒绝：允许清单 foreach 逐容器拦截 |
| 已签 `tenant-bc:latest`（platform ns） | ✅ 放行 + digest 固定，双策略 PolicyReport 通过 |

## 策略约束

| 字段 | 值 | 与 CI 对应 |
|---|---|---|
| `imageReferences` | `ghcr.io/jaculi/*:*` | 平台全域通配（monorepo 全 BC） |
| `issuer` | `https://token.actions.githubusercontent.com` | CI OIDC 发行者 |
| `subject` | `https://github.com/jaculi/Hello-World/*` | 仅本仓 workflow 签名可信 |
| `rekor.url` | `https://rekor.sigstore.dev` | 公共透明日志（签名存在性校验） |
| `mutateDigest` | `true` | 验证通过后 pin 到 digest |
| `validationFailureAction` | `Enforce` | 未通过即拒绝（fail-closed） |
| `failurePolicy` | `Fail` | webhook 失效时拒绝（准入侧 fail-closed，故 admission 3 副本 HA） |
| `background` | `true` | 后台扫描存量 Pod 生成 PolicyReport |

## 已知注意点

- **Kyverno 归一化**：Docker Hub 镜像归一化为 `docker.io/<name>:<tag>`（不含 `library/`）；GHCR 保持 `ghcr.io/<org>/<repo>` 原样。
- **缺省列表误伤**：validate pattern 直接匹配 `initContainers` 等缺省列表会对普通 Pod 误判拒绝，必须用 `foreach` + `|| \`[]\`` 回退（策略内已按此写法）。
- **CEL 迁移**：Kyverno 已宣布 `ClusterPolicy`（kyverno.io）将来废弃，迁移至 `policies.kyverno.io` 的 ImageValidatingPolicy/ValidatingPolicy（CEL）；v1.19.0 仍完整支持，升级大版本时需回访本目录。
- **允许清单例外流程**：platform 命名空间引入第三方镜像（如 sidecar、采集器）需安全评审后在该策略追加镜像引用，禁止放宽签名校验。
