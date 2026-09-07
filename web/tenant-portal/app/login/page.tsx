// 登录触发页（M19）：点按钮 → loginWithKeycloak 服务端 action → signIn 跳 Keycloak code 流。
import { loginWithKeycloak } from "@/lib/actions";

export default function LoginPage() {
  return (
    <div className="mx-auto flex max-w-sm flex-col items-center gap-6 py-24">
      <h1 className="text-2xl font-semibold text-brand-700">JSL 租户门户</h1>
      <p className="text-center text-sm text-slate-500">
        经 Keycloak OpenID Connect 登录；令牌存 httpOnly cookie，由门户 BFF 调 APISIX。
      </p>
      <form action={loginWithKeycloak}>
        <button
          type="submit"
          className="rounded-md bg-brand-600 px-6 py-2.5 text-sm font-medium text-white hover:bg-brand-700"
        >
          使用 Keycloak 登录
        </button>
      </form>
      <p className="text-xs text-slate-400">
        demo 账号：demo-admin / dev
      </p>
    </div>
  );
}
