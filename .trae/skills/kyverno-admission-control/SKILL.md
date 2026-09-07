---
name: "kyverno-admission-control"
description: "Authors and validates Kyverno CEL admission policies (policies.kyverno.io/v1) for the JSL platform. Invoke when adding/modifying Kyverno policies, policy exceptions, or admission control hardening."
---

# Kyverno 准入控制策略编写与验证

本 skill 沉淀了 JSL 平台 Kyverno 准入控制器（M9–M12）的完整工作流，覆盖 CEL 策略编写、kind 实证、例外机制与提交推送。

## 适用场景

- 新增或修改 Kyverno 准入策略（ImageValidatingPolicy / ValidatingPolicy）
- 配置策略例外（PolicyException）
- Kyverno 生产侧准入硬化（PSS、资源、网络、标签治理等）

## 前置条件

- kind 集群已安装 Kyverno v1.19.0+（Helm chart 3.9.0）
- 策略文件位于 `deploy/kyverno/policies/`
- 例外模板位于 `deploy/kyverno/exceptions/template.yaml`

## CEL 策略编写要点（踩坑汇总）

### 1. 字段安全访问（最重要）

**禁止使用 `has()` 宏**（Kyverno CEL ValidatingPolicy 不支持，报 `undeclared reference to 'has'`）。

正确写法：
```yaml
# 错误：has(obj.field)
- expression: "has(object.spec.securityContext)"
# 正确：.?field.orValue(...)
- expression: "object.spec.?securityContext.orValue({}) != {}"
```

逐层安全访问嵌套字段：
```yaml
# securityContext 可能不存在，须逐层用 .?
- expression: >-
    (c.?securityContext.?capabilities.?drop.orValue([])).exists(cap, cap == 'ALL')
```

判断字段不存在：
```yaml
# 用 == null 判断
- expression: "v.?projected == null"
# 或 .orValue(默认值)
- expression: "(object.spec.?volumes.orValue([])).all(...)"
```

### 2. 可选列表字段

```yaml
# initContainers/ephemeralContainers 可能不存在，用 .orValue([])
- expression: "(object.spec.?initContainers.orValue([])).all(c, ...)"
```

### 3. cosign keyless 签名（ImageValidatingPolicy）

`identities.subject` 是**严格匹配**，不支持通配符。多仓库/多 workflow 须用 `subjectRegExp`：

```yaml
attestors:
  - name: gh-oidc
    entries:
      - keyless:
          issuer: https://token.actions.githubusercontent.com
          subjectRegExp: "^https://github.com/jaculi/Hello-World/.*"
          ctlog:
            url: https://rekor.sigstore.dev
```

### 4. 镜像引用匹配

用 `matchImageReferences` 的 glob 语法，无需正则：
```yaml
matchImageReferences:
  - glob: "ghcr.io/jaculi/*:*"
```

### 5. CEL 表达式返回类型

- `images[].expression` 必须返回**镜像字符串列表**，不能返回容器对象：
  ```yaml
  images:
    - expression: "object.spec.containers.map(c, c.image)"
  ```

## 策略矩阵（二十层纵深防御）

| 层 | 策略 | 类型 | 范围 | 作用 |
|---|---|---|---|---|
| 1 | verify-image-signatures | ImageValidatingPolicy | 全集群 Pod | cosign 签名校验 + digest 固定 |
| 2 | platform-registry-allowlist | ValidatingPolicy | platform ns | 仅允许平台仓库镜像 |
| 3 | require-pod-security-restricted | ValidatingPolicy | platform ns | PSS restricted |
| 4 | require-resource-limits | ValidatingPolicy | platform ns | 强制 CPU/内存 requests+limits |
| 5 | disable-automount-sa-token | ValidatingPolicy | platform ns | 禁止 SA token 自动挂载 |
| 6 | disallow-latest-tag | ValidatingPolicy | platform ns | 禁止 :latest 镜像标签 |
| 7 | require-pod-standard-labels | ValidatingPolicy | platform ns | Pod 标准标签（name/version） |
| 8 | disallow-host-port | ValidatingPolicy | platform ns | 禁止 hostPort |
| 9 | disallow-run-as-root | ValidatingPolicy | platform ns | 禁止 runAsUser=0 |
| 10 | require-default-proc-mount | ValidatingPolicy | platform ns | 禁止 procMount=Unmasked |
| 11 | disallow-host-aliases | ValidatingPolicy | platform ns | 禁止 hostAliases |
| 12 | disallow-node-port-lb-service | ValidatingPolicy | platform ns | 禁止 NodePort/LB Service |
| 13 | require-emptydir-size-limit | ValidatingPolicy | platform ns | 强制 emptyDir sizeLimit |
| 14 | require-always-pull-policy | ValidatingPolicy | platform ns | 强制 imagePullPolicy=Always |
| 15 | disallow-cluster-admin-binding | ValidatingPolicy | 全集群 RBAC | 禁止 cluster-admin 绑定 |
| 16 | disallow-wildcard-rbac | ValidatingPolicy | 全集群 RBAC | 禁止通配符 RBAC 权限 |
| 17 | disallow-default-service-account | ValidatingPolicy | platform ns | 禁止使用 default SA |
| 18 | require-namespace-network-isolation | ValidatingPolicy | 全集群 ns | 网络隔离声明标签 |
| 19 | require-namespace-quota | ValidatingPolicy | 全集群 ns | 资源配额声明标签 |
| 20 | require-namespace-labels | ValidatingPolicy | 全集群 ns | team/environment/cost-center |

## 策略例外机制（PolicyException）

1. values.yaml 须启用（默认关闭）：
```yaml
features:
  policyExceptions:
    enabled: true
    namespace: "kyverno"   # 例外仅允许在 kyverno ns 创建
```

2. 例外结构（policies.kyverno.io/v1）：
```yaml
apiVersion: policies.kyverno.io/v1
kind: PolicyException
metadata:
  name: <exception-name>
  namespace: kyverno
spec:
  policyRefs:
    - kind: ValidatingPolicy
      name: <policy-name>
  matchConditions:
    - name: <name>
      expression: "<CEL 返回 true 时应用例外>"
  expiresAt: "2026-12-31T23:59:59Z"   # 强制过期，定期复审
```

## 验证流程

### 1. 应用策略并确认 READY
```bash
kubectl apply -f deploy/kyverno/policies/<policy>.yaml
kubectl get validatingpolicies  # 或 imagevalidatingpolicies
```

### 2. 违规拒绝测试
创建违规资源，确认被拒且返回明确消息：
```bash
kubectl run <bad-pod> -n platform --image=...   # 预期 admission webhook denied
kubectl create ns <bad-ns>                       # 预期缺标签被拒
```

### 3. 合规放行测试
创建完全合规资源，确认放行：
```bash
# Pod 须同时满足：已签镜像 + PSS + 资源限制 + SA token=false
kubectl run <ok-pod> -n platform --image=ghcr.io/jaculi/<bc>:latest --overrides='...'
```

### 4. 例外机制测试
创建临时 PolicyException，确认带匹配条件的资源放行、不带的仍被拒。

## 提交规范

1. 更新 `deploy/kyverno/README.md` 策略矩阵
2. 更新 `docs/tasks/bc-p1-skeleton-plan.md` 执行记录表
3. 提交信息格式：`feat(m<N>): Kyverno ...`
4. 推送到 master 触发 CI

## 已知限制

- CEL ValidatingPolicy **无法跨资源查询**（不能在 namespace 创建时检查其下的 NetworkPolicy/ResourceQuota），网络隔离/资源配额采用「标签声明 + GitOps 流程约束」模式
- `has()` 宏不可用，一律用 `.?` + `.orValue()` / `== null`
