// 站点列表页（M19）：GET /v1/sites；展示站点树。
import { apiFetch, type ListSitesResponse } from "@/lib/api";
import { SiteTree } from "@/components/SiteTree";

export default async function SitesPage() {
  const r = await apiFetch<ListSitesResponse>(`/v1/sites`);
  const sites = r.data?.sites ?? [];

  return (
    <div className="space-y-6">
      <h1 className="text-2xl font-semibold text-slate-900">站点</h1>
      <p className="text-sm text-slate-500">
        站点是行业中性的核心空间概念（BP-01 §2.2）；租户下站点树组织设备与业务对象。
      </p>
      <section className="rounded-lg border border-slate-200 bg-white p-6">
        <h2 className="text-sm font-medium text-slate-500">站点树</h2>
        {r.error ? (
          <p className="mt-2 text-sm text-rose-600">加载失败：{r.error}</p>
        ) : (
          <div className="mt-3">
            <SiteTree sites={sites} />
          </div>
        )}
      </section>
    </div>
  );
}
