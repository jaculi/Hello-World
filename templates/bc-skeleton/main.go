// bc-skeleton 服务入口 —— 壳层：仅装配传输服务器与生命周期（ADR-0004 决策 1：框架只进壳层）。
//
// 本目录是全平台 BC 骨架模板（templates/bc-skeleton），新 BC 的创建方式：
//   1. 复制本目录到 services/<bc-name>/
//   2. 全局替换 bc-skeleton → <bc-name>、模板模块路径 → services/<bc-name>
//   3. 修改 Name 变量与 Helm Chart 名
//   4. go build ./... && make lint 通过即为可运行空服务
package main

import (
	"github.com/go-kratos/kratos/v3"

	"github.com/jsl-aiot/platform/templates/bc-skeleton/internal/server"
)

var (
	Name    = "bc-skeleton"
	Version = "v0.0.1"
)

func main() {
	app := kratos.New(
		kratos.Name(Name),
		kratos.Version(Version),
		kratos.Server(server.NewHTTP(), server.NewGRPC()),
	)
	if err := app.Run(); err != nil {
		panic(err)
	}
}
