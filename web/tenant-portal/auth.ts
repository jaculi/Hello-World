// Auth.js v5 配置（M19 租户门户）。
//
// 链路：浏览器 → Keycloak authorize（经 APISIX /realms/* 代理）→ code 回调
//       → 门户服务端换 token（access_token 存 httpOnly 加密 cookie）→ BFF 调 APISIX /api/*。
//
// 令牌不入前端 JS（BP-05 §3.1 工作负载身份预期）；门户 RSC + route handler 经 auth() 读 session。
import NextAuth from "next-auth";
import Keycloak from "next-auth/providers/keycloak";

// 解码 JWT payload（BFF 仅读声明，签名由 APISIX 验，不二次校验）。
function decodePayload(token: string): Record<string, unknown> | null {
  try {
    const payload = token.split(".")[1];
    const json = Buffer.from(payload, "base64url").toString("utf8");
    return JSON.parse(json) as Record<string, unknown>;
  } catch {
    return null;
  }
}

declare module "next-auth" {
  // 扩展 Session：暴露 access_token（服务端用）、tenant_id、roles。
  interface Session {
    accessToken: string;
    tenantId?: string;
    subject?: string;
    roles: string[];
  }
}

export const { handlers, auth, signIn, signOut } = NextAuth({
  providers: [Keycloak],
  session: { strategy: "jwt" },
  // 路径与 Auth.js v5 默认一致：/api/auth/*（callback/signin/signout）
  pages: {
    signIn: "/login",
  },
  callbacks: {
    // 首次登录：把 access_token 与 tenant_id/sub/roles 持久化进 JWT。
    async jwt({ token, account }) {
      if (account?.access_token) {
        token.accessToken = account.access_token;
        const claims = decodePayload(account.access_token);
        if (claims) {
          token.tenantId = claims["tenant_id"] as string | undefined;
          token.subject = claims["sub"] as string | undefined;
          const realm = claims["realm_access"] as { roles?: string[] } | undefined;
          token.roles = realm?.roles ?? [];
        }
      }
      return token;
    },
    // 每次请求：把 JWT 投射到 session 供 RSC/route handler 读。
    async session({ session, token }) {
      session.accessToken = (token.accessToken as string) ?? "";
      session.tenantId = token.tenantId as string | undefined;
      session.subject = token.subject as string | undefined;
      session.roles = (token.roles as string[]) ?? [];
      return session;
    },
  },
});
