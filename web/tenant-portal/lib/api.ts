// 门户 BFF → APISIX /api/* 服务端 API 客户端（M19）。
//
// 令牌来自 Auth.js session（httpOnly cookie），以 Bearer 头透传给 APISIX。
// APISIX openid-connect 验签 + Lua 注入 X-Tenant-Id → BC RLS。
import { auth } from "@/auth";

const APISIX_BASE = process.env.APISIX_API_BASE || "http://localhost:9080/api";

export interface ApiResult<T> {
  status: number;
  data: T | null;
  // 错误体（BC 消息码 + i18n_key + params；前端按消息码做 i18n，M19 仅透传原文）
  error: string | null;
}

// apiFetch 服务端调用 APISIX；未登录返回 401。
export async function apiFetch<T>(
  path: string,
  init?: RequestInit
): Promise<ApiResult<T>> {
  const session = await auth();
  if (!session?.accessToken) {
    return { status: 401, data: null, error: "未登录" };
  }
  const res = await fetch(`${APISIX_BASE}${path}`, {
    ...init,
    headers: {
      ...(init?.headers ?? {}),
      Authorization: `Bearer ${session.accessToken}`,
    },
    // 透传 trace（APISIX 生成 X-Tenant-Id，BC 在中间件读）
    cache: "no-store",
  });
  const text = await res.text();
  if (!res.ok) {
    return { status: res.status, data: null, error: text || `HTTP ${res.status}` };
  }
  if (!text) return { status: res.status, data: null, error: null };
  try {
    return { status: res.status, data: JSON.parse(text) as T, error: null };
  } catch {
    return { status: res.status, data: null, error: text };
  }
}

// 透传代理：route handler 用，不解析 JSON，直接回传 APISIX 响应。
export async function proxyFetch(
  path: string,
  init: RequestInit
): Promise<Response> {
  const session = await auth();
  if (!session?.accessToken) {
    return new Response("Unauthorized", { status: 401 });
  }
  const res = await fetch(`${APISIX_BASE}${path}`, {
    ...init,
    headers: {
      ...(init.headers ?? {}),
      Authorization: `Bearer ${session.accessToken}`,
    },
    cache: "no-store",
  });
  const body = await res.arrayBuffer();
  const headers = new Headers();
  const ct = res.headers.get("content-type");
  if (ct) headers.set("content-type", ct);
  return new Response(body, { status: res.status, headers });
}

// BC 契约类型（与 api/tenant/v1/tenant.proto、api/site/v1/site.proto 对齐）。
export interface Tenant {
  tenant_id: string;
  name: string;
  display_name: string;
  status: string;
  isolation: string;
  plan_id: string;
  home_region: string;
  audit?: { created_at: string; updated_at: string; version: string };
}

export interface Site {
  site_id: string;
  tenant_id: string;
  parent_site_id: string;
  name: string;
  display_name: string;
  site_type: string;
}

export interface ListSitesResponse {
  sites: Site[];
}
