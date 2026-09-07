// 顶栏（M19）：平台标识 + 租户信息 + 注销。
// 服务端组件；注销走 logout 服务端 action（包 signOut）。
import Link from "next/link";
import { logout } from "@/lib/actions";

export interface NavbarProps {
  subject?: string;
  tenantId?: string;
  roles?: string[];
}

export function Navbar({ subject, tenantId, roles }: NavbarProps) {
  return (
    <header className="border-b border-slate-200 bg-white">
      <div className="mx-auto flex max-w-6xl items-center justify-between px-6 py-3">
        <div className="flex items-center gap-6">
          <Link href="/" className="text-lg font-semibold text-brand-700">
            JSL 租户门户
          </Link>
          <nav className="flex gap-4 text-sm text-slate-600">
            <Link href="/tenants" className="hover:text-brand-700">租户</Link>
            <Link href="/sites" className="hover:text-brand-700">站点</Link>
          </nav>
        </div>
        <div className="flex items-center gap-4 text-sm">
          {tenantId && (
            <span className="text-slate-500">
              租户 <code className="text-slate-700">{tenantId}</code>
            </span>
          )}
          {subject && <span className="text-slate-500">{subject}</span>}
          {roles && roles.length > 0 && (
            <span className="text-xs text-slate-400">{roles.join(", ")}</span>
          )}
          <form action={logout}>
            <button type="submit" className="text-slate-500 underline hover:text-brand-700">
              注销
            </button>
          </form>
        </div>
      </div>
    </header>
  );
}
