# M19 租户门户镜像（Next.js standalone）。
# 多阶段构建：deps → build → runner（nonroot，distroless 风格）。
# dev 跑宿主 `npm run dev`；生产/K8s 用本镜像，经 Ingress 统一 hostname 对齐 Keycloak issuer。
FROM node:22-alpine AS deps
WORKDIR /app
COPY package.json package-lock.json* ./
RUN npm ci --omit=dev=false

FROM node:22-alpine AS builder
WORKDIR /app
COPY --from=deps /app/node_modules ./node_modules
COPY . .
# 构建期需要 AUTH_SECRET 占位（Next.js 编译时不读 env，仅运行时读；填占位避免告警）
ENV AUTH_SECRET=build-placeholder
ENV NEXT_TELEMETRY_DISABLED=1
RUN npm run build

FROM node:22-alpine AS runner
WORKDIR /app
ENV NODE_ENV=production
ENV NEXT_TELEMETRY_DISABLED=1
ENV PORT=3000
RUN addgroup -g 1001 -S nodejs && adduser -S nextjs -u 1001 -G nodejs
COPY --from=builder --chown=nextjs:nodejs /app/.next/standalone ./
COPY --from=builder --chown=nextjs:nodejs /app/.next/static ./.next/static
COPY --from=builder --chown=nextjs:nodejs /app/public ./public
USER nextjs
EXPOSE 3000
CMD ["node", "server.js"]
