import type { NextConfig } from "next";

// M19 租户门户配置：dev 跑宿主 :3000，后端经 APISIX :9080。
// 生产（kind/K8s）经 Ingress 统一 hostname，此处无需特殊配置。
const nextConfig: NextConfig = {
  reactStrictMode: true,
  // standalone：Dockerfile 多阶段构建输出自包含 server.js + 静态资产
  output: "standalone",
  productionBrowserSourceMaps: false,
};

export default nextConfig;
