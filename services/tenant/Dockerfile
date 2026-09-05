# tenant-bc 镜像 —— 多阶段构建，运行时 distroless（BP-05 攻击面最小化）
FROM golang:1.25.14-alpine AS builder
ARG SERVICE_PATH=./services/tenant
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -ldflags="-s -w" -o /bin/server ${SERVICE_PATH}

FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=builder /bin/server /server
EXPOSE 8000 9000
ENTRYPOINT ["/server"]
