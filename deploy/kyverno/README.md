# deploy/kyverno/ —— 准入控制（M11 纵深防御）

> 依据：[BP-05 §7.1.2 供应链安全](../blueprint/05-security-architecture.md)（包签名验证、镜像扫描、SBOM）、BP-05 默认拒绝 + 纵深防御原则
> 这是 CI 侧 cosign keyless 签名（`.github/workflows/ci.yml` ⑤ 制品 job）的**运行时消费端**。

## 作用

四层准入（纵深防御，均为 Deny + fail-closed），全部使用 **CEL 策略**（`policies.kyverno.io/v1`），覆盖 platform 命名空间：

1. **签名校验**（`verify-image-signatures`，`ImageValidatingPolicy`）：`ghcr.io/jaculi/*:*` 全部平台镜像必须具备由 GitHub Actions OIDC 签发的 cosign keyless 签名；验证通过后自动 pin 到 digest（`mutateDigest`），防标签重放。后台扫描对存量 Pod 生成 PolicyReport。
2. **允许清单**（`platform-registry-allowlist`，`ValidatingPolicy`）：platform 命名空间内全部容器（含 init/ephemeral）只允许 `ghcr.io/jaculi/*` 镜像，防止业务 Pod 夹带未纳入供应链治理的第三方镜像。
3. **Pod 安全标准 restricted**（`require-pod-security-restricted`，`ValidatingPolicy`）：强制 privileged=false、allowPrivilegeEscalation=false、runAsNonRoot=true、readOnlyRootFilesystem=true、capabilities.drop=[ALL]、seccompProfile=RuntimeDefault、禁止 hostNetwork/hostPID/hostIPC/hostPath。防容器逃逸与权限提升。
4. **资源限制强制**（`require-resource-limits`，`ValidatingPolicy`）：全部容器必须声明 CPU/内存的 requests 与 limits，防止资源耗尽与调度失衡。

**策略例外**：默认拒绝，确需豁免时经安全评审后创建 `PolicyException`（仅允许在 kyverno 命名空间创建，由平台管理员统一管理），须设 `expiresAt` 到期自动失效。

## 目录结构

```
deploy/kyverno/
├── values.yaml                          # Kyverno Helm 安装 values（HA + PolicyException 启用）
├── README.md
├── policies/                            # 策略集（CEL，ArgoCD 同步）
│   ├── verify-image-signatures.yaml     # ImageValidatingPolicy：cosign keyless 签名校验
│   ├── platform-registry-allowlist.yaml # ValidatingPolicy：platform 镜像允许清单
│   ├── require-pod-security-restricted.yaml  # ValidatingPolicy：PSS restricted
│   └── require-resource-limits.yaml     # ValidatingPolicy：资源 requests/limits 强制
└── exceptions/                          # 策略例外模板（PolicyException）
    ├── README.md                        # 例外流程说明
    └── template.yaml                    # 例外模板（namespace=kyverno，须 expiresAt）
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
