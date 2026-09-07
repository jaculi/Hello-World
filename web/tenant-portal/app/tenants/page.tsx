// 租户列表页（M19）：RLS 下仅本租户可见，列表入口指向详情。
import Link from "next/link";
import { auth } from "@/auth";
import { apiFetch, type Tenant } from "@/lib/api";

export default async function TenantsPage() {
  const session = await auth();
  const tenantId = session?.tenantId;
  let tenant: Tenant | null = null;
  let error: string | null = null;
  if (tenantId) {
    const r = await apiFetch<Tenant>(`/v1/tenants/${tenantId}`);
    tenant = r.data;
    error = r.error;
  }

  return (
    <div className="space-y-6">
      <h1 className="text-2xl font-semibold text-slate-900">租户</h1>
      <p className="text-sm text-slate-500">
        行级安全（RLS）下仅当前令牌归属租户可见；越租户访问被伪装为不存在。
      </p>
      <div className="overflow-hidden rounded-lg border border-slate-200 bg-white">
        <table className="w-full text-sm">
          <thead className="bg-slate-50 text-left text-xs uppercase text-slate-500">
            <tr>
              <th className="px-6 py-3">租户 ID</th>
              <th className="px-6 py-3">显示名</th>
              <th className="px-6 py-3">状态</th>
              <th className="px-6 py-3">隔离</th>
              <th className="px-6 py-3"></th>
            </tr>
          </thead>
          <tbody className="divide-y divide-slate-100">
            {tenant ? (
              <tr>
                <td className="px-6 py-3 font-mono text-slate-800">{tenant.tenant_id}</td>
                <td className="px-6 py-3">{tenant.display_name}</td>
                <td className="px-6 py-3">{tenant.status}</td>
                <td className="px-6 py-3">{tenant.isolation}</td>
                <td className="px-6 py-3">
                  <Link
                    href={`/tenants/${tenant.tenant_id}`}
                    className="text-brand-600 hover:underline"
                  >
                    详情
                  </Link>
                </td>
              </tr>
            ) : (
              <tr>
                <td colSpan={5} className="px-6 py-6 text-center text-slate-400">
                  {error ? `加载失败：${error}` : "无可见租户"}
                </td>
              </tr>
            )}
          </tbody>
        </table>
      </div>
    </div>
  );
}
