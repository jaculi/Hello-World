// 路由保护（M19）：除 /login、/api/auth、静态资源外的路径均要求 session。
// 未登录 → 跳 /login（由 Auth.js 触发 Keycloak code 流）。
export { auth as middleware } from "@/auth";

export const config = {
  // 匹配所有路径，排除 Auth.js 自身路由、登录页、Next 静态资产。
  matcher: ["/((?!login|api/auth|_next/static|_next/image|favicon.ico).*)"],
};
