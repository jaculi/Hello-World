// 路由保护（M19/M22）：除 /login、/api/auth、静态资源外的路径均要求有效令牌。
// 未登录（无 session）或令牌不可用（access/refresh 均失效被清空）→ 跳 /login
//（Keycloak SSO 会话若仍存活则静默回跳，无需重输密码）。
import { auth } from "@/auth";

export default auth((req) => {
  if (!req.auth?.accessToken) {
    return Response.redirect(new URL("/login", req.url));
  }
});

export const config = {
  // 匹配所有路径，排除 Auth.js 自身路由、登录页、健康探针、Next 静态资产。
  matcher: ["/((?!login|api/auth|healthz|_next/static|_next/image|favicon.ico).*)"],
};
