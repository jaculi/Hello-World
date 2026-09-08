// Auth.js v5 配置（M19 租户门户）。
//
// 链路：浏览器 → Keycloak authorize（经 APISIX /realms/* 代理）→ code 回调
//       → 门户服务端换 token（access_token 存 httpOnly 加密 cookie）→ BFF 调 APISIX /api/*。
//
// 令牌不入前端 JS（BP-05 §3.1 工作负载身份预期）；门户 RSC + route handler 经 auth() 读 session。
import NextAuth from "next-auth";
import type { JWT } from "next-auth/jwt";
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

// M22：access token 过期自动续期（Keycloak refresh_token grant）。
// token 端点与 discovery 同 issuer 面（auth.localtest.me:8080，集群内经 CoreDNS→APISIX 可达）。
// 失败（refresh token 过期/被吊销/离线超时）→ 清空令牌，由 middleware 引导重登。
async function refreshAccessToken(token: JWT): Promise<JWT> {
  try {
    const res = await fetch(
      `${process.env.AUTH_KEYCLOAK_ISSUER}/protocol/openid-connect/token`,
      {
        method: "POST",
        headers: { "content-type": "application/x-www-form-urlencoded" },
        body: new URLSearchParams({
          grant_type: "refresh_token",
          client_id: process.env.AUTH_KEYCLOAK_ID ?? "",
          client_secret: process.env.AUTH_KEYCLOAK_SECRET ?? "",
          refresh_token: (token.refreshToken as string) ?? "",
        }),
        cache: "no-store",
      },
    );
    if (!res.ok) throw new Error(`token endpoint ${res.status}`);
    const t = (await res.json()) as {
      access_token: string;
      expires_in: number;
      refresh_token?: string; // Keycloak 每次轮换发新 refresh_token
    };
    // 重解码声明：租户/角色变更随新令牌生效。
    const claims = decodePayload(t.access_token);
    return {
      ...token,
      accessToken: t.access_token,
      refreshToken: t.refresh_token ?? (token.refreshToken as string),
      expiresAt: Math.floor(Date.now() / 1000) + t.expires_in,
      tenantId: (claims?.["tenant_id"] as string | undefined) ?? token.tenantId,
      subject: (claims?.["sub"] as string | undefined) ?? token.subject,
      roles:
        (claims?.["realm_access"] as { roles?: string[] } | undefined)?.roles ??
        token.roles,
      error: undefined,
    };
  } catch {
    return {
      ...token,
      accessToken: "",
      refreshToken: "",
      error: "RefreshTokenError",
    };
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
    // 初次登录：持久化 access/refresh 与过期时间；后续请求：过期前 30s 自动续期。
    async jwt({ token, account }) {
      if (account?.access_token) {
        token.accessToken = account.access_token;
        token.refreshToken = account.refresh_token;
        token.expiresAt = account.expires_at;
        const claims = decodePayload(account.access_token);
        if (claims) {
          token.tenantId = claims["tenant_id"] as string | undefined;
          token.subject = claims["sub"] as string | undefined;
          const realm = claims["realm_access"] as { roles?: string[] } | undefined;
          token.roles = realm?.roles ?? [];
        }
        return token;
      }
      // 未临近过期（提前 30s 预留网络与 APISIX 验签窗口）→ 复用现令牌。
      if (Date.now() / 1000 < ((token.expiresAt as number | undefined) ?? 0) - 30) {
        return token;
      }
      // 过期 → refresh_token 续期；失败时返回清空令牌的 token（middleware 引导重登）。
      return refreshAccessToken(token);
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
