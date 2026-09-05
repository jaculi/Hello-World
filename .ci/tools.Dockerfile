# 平台工具链 CI 镜像 —— 工具版本唯一事实源（与根 Makefile 同步维护）
# 用途：CI 流水线与本地 `docker build -f .ci/tools.Dockerfile` 一键对齐工具链
# TODO(2026-Q4)：buf / golangci-lint 从 latest 固化为精确版本并纳入季度评审（BP-07 §5）

ARG GO_VERSION=1.25.14
FROM golang:${GO_VERSION}-alpine

ARG KRATOS_VERSION=v3.0.0
ARG BUF_VERSION=latest
ARG GOLANGCI_LINT_VERSION=latest

RUN apk add --no-cache git curl protobuf

# kratos CLI（脚手架/代码生成）
RUN go install github.com/go-kratos/kratos/cmd/kratos/${KRATOS_VERSION:+v}${KRATOS_VERSION#@} 2>/dev/null || \
    go install github.com/go-kratos/kratos/cmd/kratos/v3@${KRATOS_VERSION}

# proto 代码生成链（ADR-0005：契约单仓 + buf 门禁）
RUN go install google.golang.org/protobuf/cmd/protoc-gen-go@latest && \
    go install google.golang.org/grpc/cmd/protoc-gen-go-grpc@latest && \
    go install github.com/bufbuild/buf/cmd/buf@${BUF_VERSION}

# lint（含 biz 层禁框架自定义规则）
RUN curl -sSfL https://raw.githubusercontent.com/golangci/golangci-lint/HEAD/install.sh | \
    sh -s -- -b "$(go env GOPATH)/bin" ${GOLANGCI_LINT_VERSION}

ENV PATH="${PATH}:/root/go/bin"
WORKDIR /workspace
