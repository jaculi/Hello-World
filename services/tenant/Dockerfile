# tenant-bc 镜像（步 23）—— 多阶段构建，运行时 distroless nonroot（BP-05 攻击面最小化）
# 构建上下文为仓库根（monorepo）：
#   docker build -f services/tenant/Dockerfile -t jsl/tenant-bc:v0.0.1 .
FROM golang:1.25.14-alpine AS builder
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -ldflags="-s -w" -trimpath -o /bin/server ./services/tenant

# distroless 静态镜像 + nonroot 运行（无 shell/包管理器，CA 证书随基础镜像内置）
FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=builder /bin/server /server
EXPOSE 8000 9000
USER nonroot:nonroot
ENTRYPOINT ["/server"]
