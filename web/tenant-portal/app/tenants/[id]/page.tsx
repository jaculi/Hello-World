// 租户详情页（M19）：GET /v1/tenants/{id}；越租户/不存在展示 BC not_found 消息。
import Link from "next/link";
import { apiFetch, type Tenant, type ListSitesResponse } from "@/lib/api";

export default async function TenantDetailPage({
  params,
}: {
  params: Promise<{ id: string }>;
}) {
  const { id } = await params;
  const t = await apiFetch<Tenant>(`/v1/tenants/${id}`);
  const sites = await apiFetch<ListSitesResponse>(`/v1/sites`);
  const tenant = t.data;
  const siteList = sites.data?.sites ?? [];

  return (
    <div className="space-y-6">
      <div>
        <Link href="/tenants" className="text-sm text-brand-600 hover:underline">
          ← 返回租户列表
        </Link>
        <h1 className="mt-2 text-2xl font-semibold text-slate-900">
          {tenant?.display_name ?? "租户"}
        </h1>
        {t.error && (
          <p className="mt-2 text-sm text-rose-600">
            查询失败：{t.error}（HTTP {t.status}；RLS 隔离或令牌租户不一致）
          </p>
        )}
      </div>

      {tenant && (
        <>
          <section className="rounded-lg border border-slate-200 bg-white p-6">
            <h2 className="text-sm font-medium text-slate-500">基本信息</h2>
            <dl className="mt-3 grid grid-cols-2 gap-x-6 gap-y-2 text-sm">
              <dt className="text-slate-500">租户 ID</dt>
              <dd className="font-mono text-slate-800">{tenant.tenant_id}</dd>
              <dt className="text-slate-500">外部名</dt>
              <dd className="text-slate-800">{tenant.name}</dd>
              <dt className="text-slate-500">状态</dt>
              <dd className="text-slate-800">{tenant.status}</dd>
              <dt className="text-slate-500">隔离级别</dt>
              <dd className="text-slate-800">{tenant.isolation}</dd>
              <dt className="text-slate-500">套餐</dt>
              <dd className="text-slate-800">{tenant.plan_id}</dd>
              <dt className="text-slate-500">归属区域</dt>
              <dd className="text-slate-800">{tenant.home_region}</dd>
              <dt className="text-slate-500">版本</dt>
              <dd className="text-slate-800">{tenant.audit?.version ?? "—"}</dd>
            </dl>
          </section>

          <section className="rounded-lg border border-slate-200 bg-white p-6">
            <div className="flex items-center justify-between">
              <h2 className="text-sm font-medium text-slate-500">站点</h2>
              <Link href="/sites" className="text-sm text-brand-600 hover:underline">
                管理全部 →
              </Link>
            </div>
            {sites.error ? (
              <p className="mt-2 text-sm text-rose-600">站点加载失败：{sites.error}</p>
            ) : siteList.length === 0 ? (
              <p className="mt-2 text-sm text-slate-400">该租户暂无站点</p>
            ) : (
              <ul className="mt-3 space-y-1 text-sm">
                {siteList.slice(0, 8).map((s) => (
                  <li key={s.site_id}>
                    <Link
                      href={`/sites/${s.site_id}`}
                      className="text-brand-600 hover:underline"
                    >
                      {s.display_name}
                    </Link>
                    <span className="ml-2 text-xs text-slate-400">{s.site_type}</span>
                  </li>
                ))}
              </ul>
            )}
          </section>
        </>
      )}
    </div>
  );
}
