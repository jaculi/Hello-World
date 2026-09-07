// 健康探针端点（M20）：K8s liveness/readiness probe 用，无 auth。
export function GET() {
  return new Response("ok\n", {
    status: 200,
    headers: { "content-type": "text/plain" },
  });
}
