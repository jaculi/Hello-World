# BC 骨架镜像 —— 多阶段构建，运行时 distroless（BP-05 攻击面最小化）
# SERVICE_PATH 由复制后的服务 Dockerfile 覆盖（模板默认指向模板自身）
FROM golang:1.25.14-alpine AS builder
ARG SERVICE_PATH=./templates/bc-skeleton
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -ldflags="-s -w" -o /bin/server ${SERVICE_PATH}

FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=builder /bin/server /server
EXPOSE 8000 9000
ENTRYPOINT ["/server"]
