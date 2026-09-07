// BFF 透传代理（M19）：浏览器 fetch /api/proxy/v1/tenants/... → 门户服务端
// 持 session.access_token 调 APISIX /api/*。令牌不入浏览器 JS。
//
// 仅放行 GET/POST/PATCH（与 BC HTTP 契约对齐；DELETE 等后续随 BC 增）。
import { proxyFetch } from "@/lib/api";

export async function GET(
  _req: Request,
  { params }: { params: Promise<{ path: string[] }> }
) {
  const { path } = await params;
  return proxyFetch(`/${path.join("/")}`, { method: "GET" });
}

export async function POST(
  req: Request,
  { params }: { params: Promise<{ path: string[] }> }
) {
  const { path } = await params;
  const body = await req.text();
  return proxyFetch(`/${path.join("/")}`, {
    method: "POST",
    headers: { "content-type": req.headers.get("content-type") ?? "application/json" },
    body,
  });
}

export async function PATCH(
  req: Request,
  { params }: { params: Promise<{ path: string[] }> }
) {
  const { path } = await params;
  const body = await req.text();
  return proxyFetch(`/${path.join("/")}`, {
    method: "PATCH",
    headers: { "content-type": req.headers.get("content-type") ?? "application/json" },
    body,
  });
}
