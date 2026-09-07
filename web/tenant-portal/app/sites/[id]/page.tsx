// 站点详情页（M19）：GET /v1/sites/{id}；展示归属链 + 元数据。
import Link from "next/link";
import { apiFetch, type Site, type ListSitesResponse } from "@/lib/api";

export default async function SiteDetailPage({
  params,
}: {
  params: Promise<{ id: string }>;
}) {
  const { id } = await params;
  const r = await apiFetch<Site>(`/v1/sites/${id}`);
  const all = await apiFetch<ListSitesResponse>(`/v1/sites`);
  const site = r.data;
  const siteList = all.data?.sites ?? [];

  // 自下而上找祖先链
  const ancestors: Site[] = [];
  let cur = site;
  while (cur && cur.parent_site_id) {
    const parent = siteList.find((s) => s.site_id === cur!.parent_site_id);
    if (!parent) break;
    ancestors.unshift(parent);
    cur = parent;
  }
  const children = siteList.filter((s) => s.parent_site_id === id);

  return (
    <div className="space-y-6">
      <div>
        <Link href="/sites" className="text-sm text-brand-600 hover:underline">
          ← 返回站点树
        </Link>
        <h1 className="mt-2 text-2xl font-semibold text-slate-900">
          {site?.display_name ?? "站点"}
        </h1>
        {r.error && (
          <p className="mt-2 text-sm text-rose-600">
            查询失败：{r.error}（HTTP {r.status}；RLS 隔离或站点不存在）
          </p>
        )}
      </div>

      {site && (
        <>
          <section className="rounded-lg border border-slate-200 bg-white p-6">
            <h2 className="text-sm font-medium text-slate-500">归属链</h2>
            <nav className="mt-2 flex flex-wrap items-center gap-1 text-sm text-slate-500">
              <Link href="/sites" className="hover:text-brand-600">根</Link>
              {ancestors.map((a) => (
                <span key={a.site_id} className="flex items-center gap-1">
                  <span className="text-slate-300">/</span>
                  <Link href={`/sites/${a.site_id}`} className="hover:text-brand-600">
                    {a.display_name}
                  </Link>
                </span>
              ))}
              <span className="text-slate-300">/</span>
              <span className="font-medium text-slate-800">{site.display_name}</span>
            </nav>
          </section>

          <section className="rounded-lg border border-slate-200 bg-white p-6">
            <h2 className="text-sm font-medium text-slate-500">基本信息</h2>
            <dl className="mt-3 grid grid-cols-2 gap-x-6 gap-y-2 text-sm">
              <dt className="text-slate-500">站点 ID</dt>
              <dd className="font-mono text-slate-800">{site.site_id}</dd>
              <dt className="text-slate-500">名称</dt>
              <dd className="text-slate-800">{site.name}</dd>
              <dt className="text-slate-500">类型</dt>
              <dd className="text-slate-800">{site.site_type}</dd>
              <dt className="text-slate-500">父站点</dt>
              <dd className="font-mono text-slate-800">{site.parent_site_id || "（租户根）"}</dd>
            </dl>
          </section>

          <section className="rounded-lg border border-slate-200 bg-white p-6">
            <h2 className="text-sm font-medium text-slate-500">子站点</h2>
            {children.length === 0 ? (
              <p className="mt-2 text-sm text-slate-400">无子站点</p>
            ) : (
              <ul className="mt-3 space-y-1 text-sm">
                {children.map((c) => (
                  <li key={c.site_id}>
                    <Link
                      href={`/sites/${c.site_id}`}
                      className="text-brand-600 hover:underline"
                    >
                      {c.display_name}
                    </Link>
                    <span className="ml-2 text-xs text-slate-400">{c.site_type}</span>
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
