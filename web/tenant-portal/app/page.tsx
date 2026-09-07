// 仪表盘（M19）：登录后落地页，显示当前租户摘要 + 快速入口。
import Link from "next/link";
import { auth } from "@/auth";
import { apiFetch, type Tenant } from "@/lib/api";

export default async function DashboardPage() {
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
    <div className="space-y-8">
      <div>
        <h1 className="text-2xl font-semibold text-slate-900">仪表盘</h1>
        <p className="mt-1 text-sm text-slate-500">
          {session?.subject ? `欢迎，${session.subject}` : "已登录"}
        </p>
      </div>

      <section className="rounded-lg border border-slate-200 bg-white p-6">
        <h2 className="text-sm font-medium text-slate-500">当前租户</h2>
        {error && (
          <p className="mt-2 text-sm text-rose-600">
            获取失败：{error}（HTTP 链路已通，业务 4xx 证明认证链正确）
          </p>
        )}
        {tenant ? (
          <dl className="mt-3 grid grid-cols-2 gap-x-6 gap-y-2 text-sm">
            <dt className="text-slate-500">租户 ID</dt>
            <dd className="font-mono text-slate-800">{tenant.tenant_id}</dd>
            <dt className="text-slate-500">显示名</dt>
            <dd className="text-slate-800">{tenant.display_name}</dd>
            <dt className="text-slate-500">状态</dt>
            <dd className="text-slate-800">{tenant.status}</dd>
            <dt className="text-slate-500">隔离级别</dt>
            <dd className="text-slate-800">{tenant.isolation}</dd>
            <dt className="text-slate-500">套餐</dt>
            <dd className="text-slate-800">{tenant.plan_id}</dd>
            <dt className="text-slate-500">归属区域</dt>
            <dd className="text-slate-800">{tenant.home_region}</dd>
          </dl>
        ) : (
          !error && <p className="mt-2 text-sm text-slate-400">令牌无 tenant_id 声明</p>
        )}
        {tenant && (
          <Link
            href={`/tenants/${tenant.tenant_id}`}
            className="mt-4 inline-block text-sm text-brand-600 hover:underline"
          >
            查看详情 →
          </Link>
        )}
      </section>

      <section className="grid grid-cols-2 gap-4">
        <Link
          href="/sites"
          className="rounded-lg border border-slate-200 bg-white p-6 hover:border-brand-500"
        >
          <h3 className="font-medium text-slate-900">站点管理</h3>
          <p className="mt-1 text-sm text-slate-500">站点树与子站点移动</p>
        </Link>
        <Link
          href="/tenants"
          className="rounded-lg border border-slate-200 bg-white p-6 hover:border-brand-500"
        >
          <h3 className="font-medium text-slate-900">租户信息</h3>
          <p className="mt-1 text-sm text-slate-500">本租户详情与状态</p>
        </Link>
      </section>
    </div>
  );
}
