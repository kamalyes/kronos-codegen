/*
 * @Author: kamalyes 501893067@qq.com
 * @Date: 2026-11-01 18:05:12
 * @LastEditors: kamalyes 501893067@qq.com
 * @LastEditTime: 2026-11-01 20:12:06
 * @FilePath: \kronos-codegen\cli_test.go
 * @Description: CLI 端到端测试 - dry-run 不写盘、确认流程、幂等重跑与 check 漂移报告
 *
 * Copyright (c) 2026 by kamalyes, All Rights Reserved.
 */
package main

import (
	"bytes"
	"go/format"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// 测试项目的文件内容，形态对齐 kronos-service-template 的真实文件
// [EN] the test project files, shaped after the real kronos-service-template files
const (
	tableGo = `package constants

const (
	TableNameExample = "examples"
)
`
	factoryGo = `package repository

// RepositoryFactory 仓库工厂
type RepositoryFactory struct {
	Example *ExampleRepository
}

var factoryInstance *RepositoryFactory

// NewRepositoryFactory 创建工厂
func NewRepositoryFactory() *RepositoryFactory {
	factoryInstance = &RepositoryFactory{
		Example: NewExampleRepository(),
	}
	return factoryInstance
}
`
	migrateGo = `package migrations

// GetAllModels 模型清单
func GetAllModels() []interface{} {
	return []interface{}{
		&models.ExampleModel{},
	}
}
`
	servicesGo = `package bootstrap

// registerGRPCServices 注册 gRPC 服务
func (s *DemoGateway) registerGRPCServices() {
	s.gateway.RegisterService(func(srv *grpc.Server) {
		// kronos:register=grpc
		pb.RegisterExampleServiceServer(srv, s.exampleSvc)
	})
}
`
	exportsGo = `package service

import (
	commonsvc "github.com/kamalyes/demo/service/common"
	notifysvc "github.com/kamalyes/demo/service/notify"
)

type (
	NotifyServiceImpl = notifysvc.NotifyServiceImpl
)

var (
	NewNotifyService = notifysvc.NewNotifyService
	_                = commonsvc.Base
)
`
	modelTmpl = `/*
 * @Description: {{.Name}} 数据模型
 *
 * Copyright (c) 2026 by kamalyes, All Rights Reserved.
 */
package models

// {{.Name}}Model {{.Name}} 数据模型
type {{.Name}}Model struct {
Id string
TenantId string
}

// TableName 指定表名
func ({{.Name}}Model) TableName() string { return "{{.Snake}}s" }
`
	localeJSON = `{
  "welcome": "Welcome"
}
`
	manifestJSON = `{
  "version": 1,
  "sections": {
    "entity": [
      {
        "kind": "const",
        "file": "constants/table.go",
        "member": "TableNameExample",
        "name": "TableName{{.Name}}",
        "value": "\"{{.Snake}}s\"",
        "doc": "TableName{{.Name}} {{.Name}} 表名"
      },
      {
        "kind": "struct_field",
        "file": "repository/factory.go",
        "struct": "RepositoryFactory",
        "name": "{{.Name}}",
        "type": "*{{.Name}}Repository"
      },
      {
        "kind": "composite_field",
        "file": "repository/factory.go",
        "func": "NewRepositoryFactory",
        "type": "RepositoryFactory",
        "key": "{{.Name}}",
        "value": "New{{.Name}}Repository()"
      },
      {
        "kind": "slice_append",
        "file": "migrations/auto_migrate.go",
        "func": "GetAllModels",
        "element": "&models.{{.Name}}Model{}"
      },
      {
        "kind": "stmt",
        "file": "bootstrap/services.go",
        "marker": "kronos:register=grpc",
        "stmt": "pb.Register{{.Name}}ServiceServer(srv, s.{{.Lower}}Svc)"
      },
      {
        "kind": "import",
        "file": "service/exports.go",
        "import": "github.com/kamalyes/demo/service/{{.Snake}}",
        "alias": "{{.Flat}}svc",
        "near": "github.com/kamalyes/demo/service/notify"
      },
      {
        "kind": "type",
        "file": "service/exports.go",
        "member": "NotifyServiceImpl",
        "name": "{{.Name}}ServiceImpl",
        "value": "{{.Flat}}svc.{{.Name}}ServiceImpl"
      },
      {
        "kind": "create_file",
        "file": "models/{{.Snake}}_model.go",
        "template": ".kronos-codegen/templates/model.go.tmpl"
      },
      {
        "kind": "json_key",
        "files": ["locales/*.json"],
        "key": "{{.Delimited}}.created",
        "values": {"zh": "订单已创建"},
        "fallback": "{{.Name}} created"
      }
    ]
  }
}
`
)

// newProject 在临时目录里铺出一个待打补丁的项目
// [EN] newProject lays out a patchable project in a temp directory
func newProject(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	files := map[string]string{
		"constants/table.go":                      tableGo,
		"repository/factory.go":                   factoryGo,
		"migrations/auto_migrate.go":              migrateGo,
		"bootstrap/services.go":                   servicesGo,
		"service/exports.go":                      exportsGo,
		".kronos-codegen/templates/model.go.tmpl": modelTmpl,
		"locales/en.json":                         localeJSON,
		"locales/zh.json":                         localeJSON,
		".kronos-codegen.json":                    manifestJSON,
	}
	for rel, content := range files {
		abs := filepath.Join(root, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(abs), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(abs, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return root
}

// invoke 跑一次 CLI，返回退出码与合并后的输出
// [EN] invoke runs the CLI once and returns the exit code plus the merged output
func invoke(t *testing.T, stdin string, args ...string) (int, string) {
	t.Helper()
	var out, errBuf bytes.Buffer
	code := Run(args, &out, &errBuf, strings.NewReader(stdin))
	return code, out.String() + errBuf.String()
}

// readFile 读取项目里的文件
// [EN] readFile reads one project file
func readFile(t *testing.T, root, rel string) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(rel)))
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

// TestNewEntityDryRunThenApply dry-run 只给预览，确认之后才写盘，重跑是 no-op
// [EN] TestNewEntityDryRunThenApply covers preview, write-after-confirm and the no-op rerun
func TestNewEntityDryRunThenApply(t *testing.T) {
	root := newProject(t)

	code, out := invoke(t, "", "new", "entity", "Order", "--root", root, "--dry-run")
	if code != exitOK {
		t.Fatalf("dry-run 退出码应为 0，实际 %d\n%s", code, out)
	}
	for _, want := range []string{
		"--- a/constants/table.go",
		"+\tTableNameOrder",
		"+\tOrder   *OrderRepository",
		"+\t\tOrder:   NewOrderRepository(),",
		"+\t\t&models.OrderModel{},",
		"+\t\tpb.RegisterOrderServiceServer(srv, s.orderSvc)",
		"+\tordersvc \"github.com/kamalyes/demo/service/order\"",
		"+\tOrderServiceImpl  = ordersvc.OrderServiceImpl",
		`+  "order.created": "订单已创建"`,
		`+  "order.created": "Order created"`,
		"create models/order_model.go  (",
		"--- /dev/null",
		"+++ b/models/order_model.go",
		"新建文件，预览前 8 行",
		"dry-run 模式，未写盘",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("预览里缺少 %q\n--- 实际输出 ---\n%s", want, out)
		}
	}
	if readFile(t, root, "constants/table.go") != tableGo {
		t.Errorf("dry-run 不应写盘")
	}
	if _, err := os.Stat(filepath.Join(root, "models", "order_model.go")); err == nil {
		t.Errorf("dry-run 不应创建新文件")
	}

	// 不加 --yes 且回答 n，应当取消
	// [EN] answering n without --yes cancels the run
	code, out = invoke(t, "n\n", "new", "entity", "Order", "--root", root)
	if code != exitOK || !strings.Contains(out, "已取消") {
		t.Fatalf("回答 n 应当取消，code=%d\n%s", code, out)
	}
	if readFile(t, root, "constants/table.go") != tableGo {
		t.Errorf("取消后不应写盘")
	}

	// 回答 y，写盘
	// [EN] answering y writes the changes
	code, out = invoke(t, "y\n", "new", "entity", "Order", "--root", root)
	if code != exitOK {
		t.Fatalf("写入退出码应为 0，实际 %d\n%s", code, out)
	}
	if !strings.Contains(out, "共写入 8 个文件") {
		t.Errorf("应当写入 8 个文件\n%s", out)
	}
	if got := readFile(t, root, "constants/table.go"); !strings.Contains(got, "\tTableNameOrder = \"orders\"\n") {
		t.Errorf("const 没写进去\n%s", got)
	}
	if got := readFile(t, root, "locales/zh.json"); !strings.Contains(got, `"order.created": "订单已创建"`) {
		t.Errorf("中文译文没写进去\n%s", got)
	}
	if got := readFile(t, root, "locales/en.json"); !strings.Contains(got, `"order.created": "Order created"`) {
		t.Errorf("兜底译文没写进去\n%s", got)
	}
	// 新建文件必须存在、必须 gofmt 干净、注释块必须保留
	// [EN] the new file must exist, be gofmt-clean and keep its header comment
	newFile := readFile(t, root, "models/order_model.go")
	if formatted, err := format.Source([]byte(newFile)); err != nil || string(formatted) != newFile {
		t.Errorf("新建的 Go 文件必须是 gofmt 干净的\n%s", newFile)
	}
	if !strings.Contains(newFile, "\tId       string\n") {
		t.Errorf("新建文件的字段没有被对齐\n%s", newFile)
	}
	if !strings.HasPrefix(newFile, "/*\n * @Description: Order 数据模型") {
		t.Errorf("新建文件的头注释块不符\n%s", newFile)
	}
	if !strings.Contains(newFile, `return "orders"`) {
		t.Errorf("模板变量没有渲染\n%s", newFile)
	}

	// 幂等重跑：所有条目已存在
	// [EN] idempotent rerun: every entry is already present
	before := readFile(t, root, "repository/factory.go")
	code, out = invoke(t, "", "new", "entity", "Order", "--root", root, "--yes")
	if code != exitOK {
		t.Fatalf("重跑退出码应为 0，实际 %d\n%s", code, out)
	}
	if !strings.Contains(out, "所有条目都已存在，无需改动") {
		t.Errorf("重跑应当报告全部已存在\n%s", out)
	}
	if readFile(t, root, "repository/factory.go") != before {
		t.Errorf("重跑不应改动文件")
	}
}

// TestCheck check 命令要能报出漂移，并在全部落地后通过
// [EN] TestCheck asserts check reports drift and passes once everything is in place
func TestCheck(t *testing.T) {
	root := newProject(t)

	code, out := invoke(t, "", "check", "--root", root)
	if code != exitOK {
		t.Fatalf("清单自检应当通过，实际 %d\n%s", code, out)
	}
	if !strings.Contains(out, "补丁计划自检通过") {
		t.Errorf("自检输出不符\n%s", out)
	}

	code, out = invoke(t, "", "check", "--entity", "Order", "--root", root)
	if code != exitDrift {
		t.Fatalf("尚未落地时应返回漂移退出码 %d，实际 %d\n%s", exitDrift, code, out)
	}
	if !strings.Contains(out, "miss   models/order_model.go") || !strings.Contains(out, "(文件不存在)") {
		t.Errorf("该建但没建的文件应当算漂移\n%s", out)
	}
	if !strings.Contains(out, "miss ") || !strings.Contains(out, "10 条缺失") {
		t.Errorf("漂移报告不符\n%s", out)
	}

	if code, out = invoke(t, "", "new", "entity", "Order", "--root", root, "--yes"); code != exitOK {
		t.Fatalf("写入失败 %d\n%s", code, out)
	}
	code, out = invoke(t, "", "check", "--entity", "Order", "--root", root)
	if code != exitOK {
		t.Fatalf("全部落地后 check 应当通过，实际 %d\n%s", code, out)
	}
	if !strings.Contains(out, "10 条已落地，0 条缺失") {
		t.Errorf("check 输出不符\n%s", out)
	}
}

// TestUsageAndErrors 用法错误与缺少计划清单的提示
// [EN] TestUsageAndErrors covers usage errors and the missing manifest hint
func TestUsageAndErrors(t *testing.T) {
	root := newProject(t)
	empty := t.TempDir()

	cases := []struct {
		name     string
		args     []string
		wantCode int
		wantMsg  string
	}{
		{"无参数", nil, exitUsage, "用法 / Usage"},
		{"未知命令", []string{"nope"}, exitUsage, "未知命令"},
		{"version", []string{"version"}, exitOK, "kronos-codegen " + Version},
		{"help", []string{"help"}, exitOK, "new entity <Name>"},
		{"new 缺子目标", []string{"new"}, exitUsage, "目前支持: entity"},
		{"new 未知子目标", []string{"new", "field"}, exitUsage, "未知的 new 子目标"},
		{"缺实体名", []string{"new", "entity"}, exitUsage, "用法: kronos-codegen new entity"},
		{"实体名小写开头", []string{"new", "entity", "order", "--root", root}, exitUsage, "必须以大写字母开头"},
		{"实体名不合法", []string{"new", "entity", "Order-X", "--root", root}, exitUsage, "不是合法的 Go 标识符"},
		{"根目录不存在", []string{"new", "entity", "Order", "--root", filepath.Join(root, "nope")}, exitUsage, "不存在或不是目录"},
		{"缺少补丁计划", []string{"new", "entity", "Order", "--root", empty}, exitFailure, "找不到补丁计划清单"},
		{"格式化策略不合法", []string{"new", "entity", "Order", "--root", root, "--format", "wat"}, exitUsage, "未知的格式化策略"},
		{"多余参数", []string{"new", "entity", "Order", "extra", "--root", root}, exitUsage, "多余的参数"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			code, out := invoke(t, "", tc.args...)
			if code != tc.wantCode {
				t.Errorf("退出码 got %d want %d\n%s", code, tc.wantCode, out)
			}
			if !strings.Contains(out, tc.wantMsg) {
				t.Errorf("输出里缺少 %q\n%s", tc.wantMsg, out)
			}
		})
	}
}

// TestGlobalFlagBeforeCommand 全局 flag 写在子命令前面也要能生效
// [EN] TestGlobalFlagBeforeCommand asserts global flags work before the sub-command too
func TestGlobalFlagBeforeCommand(t *testing.T) {
	root := newProject(t)
	code, out := invoke(t, "", "--root", root, "--dry-run", "new", "entity", "Order")
	if code != exitOK {
		t.Fatalf("退出码应为 0，实际 %d\n%s", code, out)
	}
	if !strings.Contains(out, "dry-run 模式，未写盘") {
		t.Errorf("--dry-run 没生效\n%s", out)
	}
}

// TestVarOverride --var 要能覆盖派生出来的命名变量
// [EN] TestVarOverride asserts --var overrides the derived naming variables
func TestVarOverride(t *testing.T) {
	root := newProject(t)
	code, out := invoke(t, "", "new", "entity", "Order", "--root", root, "--dry-run", "--var", "Snake=order_item")
	if code != exitOK {
		t.Fatalf("退出码应为 0，实际 %d\n%s", code, out)
	}
	if !strings.Contains(out, `TableNameOrder = "order_items"`) {
		t.Errorf("--var 没生效\n%s", out)
	}
}

// TestFormatNeverKeepsBytes --format never 下插入点之外逐字节不变
// [EN] TestFormatNeverKeepsBytes asserts byte stability outside the insertion point
func TestFormatNeverKeepsBytes(t *testing.T) {
	root := newProject(t)
	if code, out := invoke(t, "", "new", "entity", "Order", "--root", root, "--yes", "--format", "never"); code != exitOK {
		t.Fatalf("写入失败 %d\n%s", code, out)
	}
	got := readFile(t, root, "constants/table.go")
	want := `package constants

const (
	TableNameExample = "examples"
	// TableNameOrder Order 表名
	TableNameOrder = "orders"
)
`
	if got != want {
		t.Errorf("输出不符\n--- got ---\n%s\n--- want ---\n%s", got, want)
	}
}
