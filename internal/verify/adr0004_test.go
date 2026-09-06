// ADR-0004 回归测试（步 21 项 5）：领域层与数据层零框架依赖。
//
// go-kratos 仅允许出现在壳层（internal/server、main）；internal/biz 与 internal/data
// 必须为纯 Go + 标准库 + 平台 pkg，保证框架未来可替换（ADR-0004 决策 1 逃生舱）。
//
// 用法：go test ./internal/verify -run TestADR0004 -v
package verify

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// forbiddenModules 禁止在 biz/data 层出现的框架/框架生态依赖。
var forbiddenModules = []string{
	"github.com/go-kratos/kratos",
}

// layerDirs 受约束的层目录（相对仓库根）。
var layerDirs = []string{
	"services/tenant/internal/biz",
	"services/tenant/internal/data",
}

func TestADR0004_NoFrameworkInDomainLayers(t *testing.T) {
	root := repoRoot(t)
	for _, dir := range layerDirs {
		abs := filepath.Join(root, filepath.FromSlash(dir))
		fset := token.NewFileSet()
		pkgs, err := parser.ParseDir(fset, abs, func(fi os.FileInfo) bool {
			// 排除测试文件（测试可引用框架做 mock）
			return !strings.HasSuffix(fi.Name(), "_test.go")
		}, parser.ImportsOnly)
		if err != nil {
			t.Fatalf("parse %s: %v", dir, err)
		}
		for _, pkg := range pkgs {
			ast.Inspect(pkg, func(n ast.Node) bool {
				if imp, ok := n.(*ast.ImportSpec); ok {
					path := strings.Trim(imp.Path.Value, `"`)
					for _, mod := range forbiddenModules {
						if strings.HasPrefix(path, mod) {
							t.Errorf("ADR-0004 violation: %s imports framework %q (file %s)",
								dir, mod, fset.Position(imp.Pos()).Filename)
						}
					}
				}
				return true
			})
		}
	}
}

// repoRoot 向上查找 go.mod 定位仓库根。
func repoRoot(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("go.mod not found")
		}
		dir = parent
	}
}
