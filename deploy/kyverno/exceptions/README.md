# deploy/kyverno/exceptions/ —— 策略例外（PolicyException）

> 依据：BP-05 默认拒绝 + 例外走评审原则

生产环境所有 Kyverno 策略默认 `Deny`。确需豁免特定资源时，**不得直接修改策略**，须经安全评审后在此目录提交 `PolicyException`（`policies.kyverno.io/v1`）。

## 例外流程

1. 提交例外申请（含业务理由、风险评估、过期时间）；
2. 安全评审通过后，复制 `template.yaml` 填写并提交 PR；
3. ArgoCD 同步后例外生效；
4. `expiresAt` 到期后例外自动失效，须重新评审续期。

## 模板字段

| 字段 | 说明 |
|---|---|
| `spec.policyRefs` | 豁免的策略列表（`kind` + `name`） |
| `spec.matchConditions` | CEL 表达式，返回 `true` 时应用例外 |
| `spec.expiresAt` | ISO 8601 过期时间，到期自动失效 |

例外为**命名空间作用域**，仅在其 `metadata.namespace` 内生效。
