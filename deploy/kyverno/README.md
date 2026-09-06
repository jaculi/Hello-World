# deploy/kyverno/ —— 镜像签名准入（M10 CEL 迁移）

> 依据：[BP-05 §7.1.2 供应链安全](../blueprint/05-security-architecture.md)（包签名验证、镜像扫描、SBOM）、BP-05 默认拒绝原则
> 这是 CI 侧 cosign keyless 签名（`.github/workflows/ci.yml` ⑤ 制品 job）的**运行时消费端**。

## 作用

两层准入（纵深防御，均为 Deny + fail-closed），M10 起全部迁移至 **CEL 策略**（`policies.kyverno.io/v1`），告别已废弃的 `ClusterPolicy`（kyverno.io）：

1. **签名校验**（`verify-image-signatures`，`ImageValidatingPolicy`）：`ghcr.io/jaculi/*:*` 全部平台镜像必须具备由 GitHub Actions OIDC 签发的 cosign keyless 签名（monorepo 内所有 BC 与后续插件行业服务**自动纳入**，新 BC 落地无需改策略）；验证通过后自动 pin 到 digest（`mutateDigest`），防标签重放。后台扫描对存量 Pod 生成 PolicyReport。
2. **允许清单**（`platform-registry-allowlist`，`ValidatingPolicy`）：`platform` 命名空间内全部容器（含 init/ephemeral）只允许 `ghcr.io/jaculi/*` 镜像——签名校验约束"平台镜像必须可信"，本策略反向收敛"platform 只跑平台镜像"，防止业务 Pod 夹带未纳入供应链治理的第三方镜像。第三方组件部署于独立命名空间，不受影响；确需引入时走例外评审。

## 目录结构

```
deploy/kyverno/
├── values.yaml                          # Kyverno Helm 安装 values（准入 3 副本 + 后台 2 副本 HA）
└── policies/
    ├── verify-image-signatures.yaml     # ImageValidatingPolicy（CEL）：cosign keyless 签名校验（平台全域通配）
    └── platform-registry-allowlist.yaml # ValidatingPolicy（CEL）：platform 命名空间镜像允许清单
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
- `deploy/gitops/argocd/kyverno-policies.yaml` —— 同步本仓策略集（已包含 CEL 策略类型的 ignoreDifferences）

## 本地验证（kind）

```bash
kind create cluster --name m10
helm install kyverno kyverno/kyverno -n kyverno --create-namespace -f deploy/kyverno/values.yaml
kubectl apply -f deploy/kyverno/policies/
```

M10 实证矩阵（kind v1.32.2 + Kyverno v1.19.0，CEL 策略）：

| 用例 | 结果 |
|---|---|
| 已签 `ghcr.io/jaculi/tenant-bc:latest`（default ns） | ✅ 放行，digest 固定 `@sha256:a282…` |
| 未签 `nginx:alpine`（default ns，范围外） | ✅ 放行（`matchImageReferences` 跳过非匹配镜像） |
| 未签 `ghcr.io/jaculi/nonexistent:v0`（default ns） | ❌ 拒绝：digest 解析失败（镜像不存在 + fail-closed） |
| 未签 `nginx:alpine`（platform ns） | ❌ 拒绝：允许清单 |
| 已签 `tenant-bc:latest` + 伪造 init 容器 `nginx:alpine`（platform ns） | ❌ 拒绝：允许清单 init 容器校验 |
| 已签 `tenant-bc:latest`（platform ns） | ✅ 放行 + digest 固定，双策略 PolicyReport 通过 |

## 策略约束（CEL）

| 字段 | 值 | 与 CI 对应 |
|---|---|---|
| `matchImageReferences[].glob` | `ghcr.io/jaculi/*:*` | 平台全域通配（monorepo 全 BC） |
| `keyless.identities[].issuer` | `https://token.actions.githubusercontent.com` | CI OIDC 发行者 |
| `keyless.identities[].subjectRegExp` | `^https://github\.com/jaculi/Hello-World/\.github/workflows/.*` | 仅本仓 workflow 签名可信（CEL subject 为严格匹配，必须用正则） |
| `ctlog.url` | `https://rekor.sigstore.dev` | 公共透明日志（签名存在性校验） |
| `validationConfigurations.mutateDigest` | `true` | 验证通过后 pin 到 digest |
| `validationConfigurations.required` | `true` | 匹配镜像必须通过签名校验 |
| `validationActions` | `[Deny]` | 未通过即拒绝（fail-closed） |
| `failurePolicy` | `Fail` | webhook 失效时拒绝（准入侧 fail-closed，故 admission 3 副本 HA） |
| `evaluation.background.enabled` | `true` | 后台扫描存量 Pod 生成 PolicyReport |

## 已知注意点（CEL 迁移踩坑）

- **`subject` 严格匹配**：CEL keyless `identities.subject` 为严格匹配（**不支持通配符**），必须改用 `subjectRegExp` 正则匹配 GitHub Actions 签发身份。经典 `ClusterPolicy` 的 `subject` 支持 glob，迁移时必须改写。
- **`images` 提取须返回字符串**：自定义 `images[].expression` 必须返回镜像**字符串列表**（如 `object.spec.containers.map(c, c.image)`），返回容器对象会报 `type conversion error from map to string`。Pod 资源推荐直接用内置 `images.containers`。
- **可选字段用 `.?`**：CEL 中访问不存在的字段直接报错，必须用 `.?` 操作符 + `.orValue([])` 回退（如 `object.spec.?initContainers.orValue([])`）。
- **Kyverno 归一化**：Docker Hub 镜像归一化为 `docker.io/<name>:<tag>`（不含 `library/`）；GHCR 保持 `ghcr.io/<org>/<repo>` 原样。
- **允许清单例外流程**：platform 命名空间引入第三方镜像（如 sidecar、采集器）需安全评审后在该策略追加镜像引用，禁止放宽签名校验。
