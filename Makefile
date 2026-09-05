# JSL 平台 monorepo 构建入口
# 工具链版本唯一事实源：.ci/tools.Dockerfile 与本文件同步维护（ADR-0004）
# Windows 开发者：将便携版 Go 加入 PATH 后可直接使用（docs/tasks/bc-p1-skeleton-plan.md）

GO      ?= go
BUF     ?= buf
LINT    ?= golangci-lint
MODULE  := github.com/jsl-aiot/platform

.DEFAULT_GOAL := help

## tidy: 整理 go.mod 依赖
.PHONY: tidy
tidy:
	$(GO) mod tidy

## build: 编译全部包
.PHONY: build
build:
	$(GO) build ./...

## vet: 静态检查（快速）
.PHONY: vet
vet:
	$(GO) vet ./...

## lint: golangci-lint 全量门禁（含 biz 层禁框架规则，ADR-0004）
.PHONY: lint
lint:
	$(LINT) run ./...

## buf-lint: 契约仓库 lint（ADR-0005）
.PHONY: buf-lint
buf-lint:
	cd api && $(BUF) lint

## buf-breaking: 契约向后兼容校验（基线默认 origin/main，ADR-0005 决策 2）
BUF_BASE ?= origin/main
.PHONY: buf-breaking
buf-breaking:
	cd api && $(BUF) breaking --against ".git#branch=$(BUF_BASE),subdir=api"

## test: 全部单元测试
.PHONY: test
test:
	$(GO) test ./...

## run-tenant: 本地运行 BC-P1 租户服务（HTTP :8000 / gRPC :9000）
.PHONY: run-tenant
run-tenant:
	$(GO) run ./services/tenant

## help: 显示可用目标
.PHONY: help
help:
	@grep -E '^## ' $(MAKEFILE_LIST) | sed 's/## /make /'
