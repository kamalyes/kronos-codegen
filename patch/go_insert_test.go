/*
 * @Author: kamalyes 501893067@qq.com
 * @Date: 2026-11-01 18:05:12
 * @LastEditors: kamalyes 501893067@qq.com
 * @LastEditTime: 2026-11-01 20:12:06
 * @FilePath: \kronos-codegen\patch\go_insert_test.go
 * @Description: Go 源码五类插入的表驱动测试 - 正常路径、幂等、边界报错与注释保全
 *
 * Copyright (c) 2026 by kamalyes, All Rights Reserved.
 */
package patch

import (
	"errors"
	"go/format"
	"go/parser"
	"go/token"
	"strings"
	"testing"
)

// mustApply 应用一条补丁并返回新内容
// [EN] mustApply applies one patch and returns the new content
func mustApply(t *testing.T, src string, p Patch, policy FormatPolicy) (string, *Plan) {
	t.Helper()
	out, pl, err := ApplyToSource([]byte(src), p, policy)
	if err != nil {
		t.Fatalf("ApplyToSource 失败: %v", err)
	}
	return string(out), pl
}

// commentTexts 收集源码里所有注释文本
// [EN] commentTexts collects every comment text in the source
func commentTexts(t *testing.T, src string) []string {
	t.Helper()
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, "x.go", src, parser.ParseComments|parser.SkipObjectResolution)
	if err != nil {
		t.Fatalf("解析失败: %v\n%s", err, src)
	}
	var out []string
	for _, cg := range f.Comments {
		for _, c := range cg.List {
			out = append(out, c.Text)
		}
	}
	return out
}

// assertCommentsKept 断言补丁没有吃掉任何原有注释，文件头注释块逐字节不变
// 这一条是「不走 go/printer 整文件回写」的核心收益，必须有专门断言守住
// [EN] assertCommentsKept asserts no original comment is lost and the header block is
// byte-identical, which is the whole point of splicing instead of reprinting
func assertCommentsKept(t *testing.T, before, after string) {
	t.Helper()
	if !strings.HasPrefix(after, headerBlock) {
		t.Errorf("文件头版权注释块被破坏\n实际开头:\n%s", firstLines(after, 10))
	}
	want := commentTexts(t, before)
	got := commentTexts(t, after)
	index := map[string]int{}
	for _, c := range got {
		index[c]++
	}
	for _, c := range want {
		if index[c] == 0 {
			t.Errorf("原有注释丢失: %q", c)
			continue
		}
		index[c]--
	}
}

// firstLines 截取前 n 行用于报错输出
// [EN] firstLines takes the first n lines for error output
func firstLines(s string, n int) string {
	lines := strings.Split(s, "\n")
	if len(lines) > n {
		lines = lines[:n]
	}
	return strings.Join(lines, "\n")
}

// TestGoInsertGolden 五类插入的正常路径，FormatNever 下输出必须逐字节等于预期
// [EN] TestGoInsertGolden covers the happy path of all five insertion kinds byte for byte
func TestGoInsertGolden(t *testing.T) {
	cases := []struct {
		name   string
		before string
		patch  Patch
		after  string
	}{
		{
			name:   "const 块按标记注释插入",
			before: constFixture,
			patch: &DeclInsert{
				Path:  "constants/table.go",
				Block: BlockRef{Marker: "kronos:consts=table-name"},
				Name:  "TableNameOrder",
				Value: `"orders"`,
				Doc:   "TableNameOrder 订单表",
			},
			after: headerBlock + `package constants

// 表名常量 kronos:consts=table-name
const (
	// TableNameExample 示例表
	TableNameExample = "examples"
	TableNameSecond  = "seconds" // 第二张表
	// TableNameOrder 订单表
	TableNameOrder = "orders"
)

// ServiceName 服务名
const ServiceName = "demo"
`,
		},
		{
			name:   "const 块免锚点命中唯一的括号块",
			before: constFixture,
			patch: &DeclInsert{
				Path:  "constants/table.go",
				Name:  "TableNameOrder",
				Value: `"orders"`,
				Doc:   "TableNameOrder 订单表",
			},
			after: headerBlock + `package constants

// 表名常量 kronos:consts=table-name
const (
	// TableNameExample 示例表
	TableNameExample = "examples"
	TableNameSecond  = "seconds" // 第二张表
	// TableNameOrder 订单表
	TableNameOrder = "orders"
)

// ServiceName 服务名
const ServiceName = "demo"
`,
		},
		{
			name:   "const 块按已有成员名插入",
			before: constFixture,
			patch: &DeclInsert{
				Path:  "constants/table.go",
				Block: BlockRef{Member: "TableNameSecond"},
				Name:  "TableNameOrder",
				Value: `"orders"`,
			},
			after: headerBlock + `package constants

// 表名常量 kronos:consts=table-name
const (
	// TableNameExample 示例表
	TableNameExample = "examples"
	TableNameSecond  = "seconds" // 第二张表
	TableNameOrder = "orders"
)

// ServiceName 服务名
const ServiceName = "demo"
`,
		},
		{
			name:   "struct 追加字段",
			before: factoryFixture,
			patch: &StructFieldInsert{
				Path:   "repository/factory.go",
				Struct: "RepositoryFactory",
				Name:   "Order",
				Type:   "*OrderRepository",
				Doc:    "Order 订单仓库",
			},
			after: headerBlock + `package repository

import "sync"

// RepositoryFactory 仓库工厂结构体
type RepositoryFactory struct {
	// Example 示例仓库
	Example *ExampleRepository
	Second  *SecondRepository
	// Order 订单仓库
	Order *OrderRepository
}

var (
	factoryInstance *RepositoryFactory
	factoryOnce     sync.Once
)

// NewRepositoryFactory 创建仓库工厂单例
func NewRepositoryFactory() *RepositoryFactory {
	factoryOnce.Do(func() {
		factoryInstance = &RepositoryFactory{
			Example: NewExampleRepository(),
			Second:  NewSecondRepository(),
		}
	})
	return factoryInstance
}
`,
		},
		{
			name:   "构造函数复合字面量追加初始化项",
			before: factoryFixture,
			patch: &CompositeFieldInsert{
				Path:  "repository/factory.go",
				Func:  "NewRepositoryFactory",
				Type:  "RepositoryFactory",
				Key:   "Order",
				Value: "NewOrderRepository()",
			},
			after: headerBlock + `package repository

import "sync"

// RepositoryFactory 仓库工厂结构体
type RepositoryFactory struct {
	// Example 示例仓库
	Example *ExampleRepository
	Second  *SecondRepository
}

var (
	factoryInstance *RepositoryFactory
	factoryOnce     sync.Once
)

// NewRepositoryFactory 创建仓库工厂单例
func NewRepositoryFactory() *RepositoryFactory {
	factoryOnce.Do(func() {
		factoryInstance = &RepositoryFactory{
			Example: NewExampleRepository(),
			Second:  NewSecondRepository(),
			Order: NewOrderRepository(),
		}
	})
	return factoryInstance
}
`,
		},
		{
			name:   "切片字面量追加元素",
			before: migrateFixture,
			patch: &SliceAppend{
				Path:    "migrations/auto_migrate.go",
				Func:    "GetAllModels",
				Element: "&models.OrderModel{}",
			},
			after: headerBlock + `package migrations

// GetAllModels 获取所有需要初始化的数据库模型
func GetAllModels() []interface{} {
	return []interface{}{
		&models.ExampleModel{},
		&models.SecondModel{}, // 第二个模型
		&models.OrderModel{},
	}
}

// AllTables 单行切片字面量，用来覆盖紧凑块的插入路径
// [EN] AllTables is a single-line slice literal covering the compact path
var AllTables = []string{"examples"}
`,
		},
		{
			name:   "单行切片字面量追加元素并补逗号",
			before: migrateFixture,
			patch: &SliceAppend{
				Path:    "migrations/auto_migrate.go",
				Var:     "AllTables",
				Element: `"orders"`,
			},
			after: headerBlock + `package migrations

// GetAllModels 获取所有需要初始化的数据库模型
func GetAllModels() []interface{} {
	return []interface{}{
		&models.ExampleModel{},
		&models.SecondModel{}, // 第二个模型
	}
}

// AllTables 单行切片字面量，用来覆盖紧凑块的插入路径
// [EN] AllTables is a single-line slice literal covering the compact path
var AllTables = []string{"examples",
	"orders",
}
`,
		},
		{
			name:   "闭包体按已有兄弟语句插入注册语句",
			before: bootstrapFixture,
			patch: &StmtInsert{
				Path: "bootstrap/services.go",
				Near: "pb.RegisterExampleServiceServer(srv, s.exampleSvc)",
				Stmt: "pb.RegisterOrderServiceServer(srv, s.orderSvc)",
			},
			after: headerBlock + `package bootstrap

// registerGRPCServices 注册 gRPC 服务
func (s *DemoGateway) registerGRPCServices() {
	s.gateway.RegisterService(func(srv *grpc.Server) {
		// kronos:register=grpc
		pb.RegisterExampleServiceServer(srv, s.exampleSvc)
		pb.RegisterOrderServiceServer(srv, s.orderSvc)
	})
	gwglobal.LOGGER.Info("gRPC services registered")
}

// initServices 初始化所有服务
func (s *DemoGateway) initServices() {
	s.repo = repository.NewRepositoryFactory()
}
`,
		},
		{
			name:   "切片按已有兄弟元素插入",
			before: migrateFixture,
			patch: &SliceAppend{
				Path:    "migrations/auto_migrate.go",
				Near:    "&models.SecondModel{}",
				Element: "&models.OrderModel{}",
			},
			after: headerBlock + `package migrations

// GetAllModels 获取所有需要初始化的数据库模型
func GetAllModels() []interface{} {
	return []interface{}{
		&models.ExampleModel{},
		&models.SecondModel{}, // 第二个模型
		&models.OrderModel{},
	}
}

// AllTables 单行切片字面量，用来覆盖紧凑块的插入路径
// [EN] AllTables is a single-line slice literal covering the compact path
var AllTables = []string{"examples"}
`,
		},
		{
			name:   "After 紧跟兄弟语句之后插入而不是追加到块尾",
			before: bootstrapFixture,
			patch: &StmtInsert{
				Path:  "bootstrap/services.go",
				Near:  "s.gateway.RegisterService(func(srv *grpc.Server) {\n\tpb.RegisterExampleServiceServer(srv, s.exampleSvc)\n})",
				After: true,
				Stmt:  "gwglobal.LOGGER.Info(\"order service wired\")",
			},
			after: headerBlock + `package bootstrap

// registerGRPCServices 注册 gRPC 服务
func (s *DemoGateway) registerGRPCServices() {
	s.gateway.RegisterService(func(srv *grpc.Server) {
		// kronos:register=grpc
		pb.RegisterExampleServiceServer(srv, s.exampleSvc)
	})
	gwglobal.LOGGER.Info("order service wired")
	gwglobal.LOGGER.Info("gRPC services registered")
}

// initServices 初始化所有服务
func (s *DemoGateway) initServices() {
	s.repo = repository.NewRepositoryFactory()
}
`,
		},
		{
			name:   "闭包体按标记注释插入注册语句",
			before: bootstrapFixture,
			patch: &StmtInsert{
				Path:   "bootstrap/services.go",
				Marker: "kronos:register=grpc",
				Stmt:   "pb.RegisterOrderServiceServer(srv, s.orderSvc)",
			},
			after: headerBlock + `package bootstrap

// registerGRPCServices 注册 gRPC 服务
func (s *DemoGateway) registerGRPCServices() {
	s.gateway.RegisterService(func(srv *grpc.Server) {
		// kronos:register=grpc
		pb.RegisterExampleServiceServer(srv, s.exampleSvc)
		pb.RegisterOrderServiceServer(srv, s.orderSvc)
	})
	gwglobal.LOGGER.Info("gRPC services registered")
}

// initServices 初始化所有服务
func (s *DemoGateway) initServices() {
	s.repo = repository.NewRepositoryFactory()
}
`,
		},
		{
			name:   "方法体按函数名插入语句",
			before: bootstrapFixture,
			patch: &StmtInsert{
				Path: "bootstrap/services.go",
				Func: "initServices",
				Recv: "DemoGateway",
				Stmt: "s.orderSvc = ordersvc.NewOrderService(s.repo.Order)",
			},
			after: headerBlock + `package bootstrap

// registerGRPCServices 注册 gRPC 服务
func (s *DemoGateway) registerGRPCServices() {
	s.gateway.RegisterService(func(srv *grpc.Server) {
		// kronos:register=grpc
		pb.RegisterExampleServiceServer(srv, s.exampleSvc)
	})
	gwglobal.LOGGER.Info("gRPC services registered")
}

// initServices 初始化所有服务
func (s *DemoGateway) initServices() {
	s.repo = repository.NewRepositoryFactory()
	s.orderSvc = ordersvc.NewOrderService(s.repo.Order)
}
`,
		},
		{
			name:   "空 struct 块沿用 TODO 注释的缩进",
			before: emptyFactoryFixture,
			patch: &StructFieldInsert{
				Path:   "repository/factory.go",
				Struct: "RepositoryFactory",
				Name:   "Order",
				Type:   "*OrderRepository",
			},
			after: headerBlock + `package repository

// RepositoryFactory 仓库工厂结构体
type RepositoryFactory struct {
	// TODO: 添加仓库字段
	Order *OrderRepository
}

// GetAllModels 获取所有需要初始化的数据库模型
func GetAllModels() []interface{} {
	return []interface{}{
		// TODO: 添加模型
	}
}
`,
		},
		{
			name:   "空切片块沿用 TODO 注释的缩进",
			before: emptyFactoryFixture,
			patch: &SliceAppend{
				Path:    "repository/factory.go",
				Func:    "GetAllModels",
				Element: "&models.OrderModel{}",
			},
			after: headerBlock + `package repository

// RepositoryFactory 仓库工厂结构体
type RepositoryFactory struct {
	// TODO: 添加仓库字段
}

// GetAllModels 获取所有需要初始化的数据库模型
func GetAllModels() []interface{} {
	return []interface{}{
		// TODO: 添加模型
		&models.OrderModel{},
	}
}
`,
		},
		{
			name:   "import 按 Near 落到标准库分组",
			before: exportsFixture,
			patch: &ImportInsert{
				Path:   "service/exports.go",
				Import: "errors",
				Near:   "context",
			},
			after: headerBlock + `package service

import (
	"context"
	"sync"
	"errors"

	"github.com/kamalyes/go-rpc-gateway/global"

	commonsvc "github.com/example/demo-service/service/common"
	notifysvc "github.com/example/demo-service/service/notify"
)
` + exportsTail,
		},
		{
			name:   "import 按 Near 落到内部分组并带别名",
			before: exportsFixture,
			patch: &ImportInsert{
				Path:   "service/exports.go",
				Import: "github.com/example/demo-service/service/template",
				Alias:  "templatesvc",
				Near:   "github.com/example/demo-service/service/common",
			},
			after: headerBlock + `package service

import (
	"context"
	"sync"

	"github.com/kamalyes/go-rpc-gateway/global"

	commonsvc "github.com/example/demo-service/service/common"
	notifysvc "github.com/example/demo-service/service/notify"
	templatesvc "github.com/example/demo-service/service/template"
)
` + exportsTail,
		},
		{
			name:   "import 不给 Near 追加到最后一组",
			before: exportsFixture,
			patch: &ImportInsert{
				Path:   "service/exports.go",
				Import: "github.com/example/demo-service/service/template",
				Alias:  "templatesvc",
			},
			after: headerBlock + `package service

import (
	"context"
	"sync"

	"github.com/kamalyes/go-rpc-gateway/global"

	commonsvc "github.com/example/demo-service/service/common"
	notifysvc "github.com/example/demo-service/service/notify"
	templatesvc "github.com/example/demo-service/service/template"
)
` + exportsTail,
		},
		{
			name:   "import 落到三方分组",
			before: exportsFixture,
			patch: &ImportInsert{
				Path:   "service/exports.go",
				Import: "github.com/kamalyes/go-sqlbuilder/repository",
				Alias:  "sqlrepo",
				Near:   "github.com/kamalyes/go-rpc-gateway/global",
			},
			after: headerBlock + `package service

import (
	"context"
	"sync"

	"github.com/kamalyes/go-rpc-gateway/global"
	sqlrepo "github.com/kamalyes/go-sqlbuilder/repository"

	commonsvc "github.com/example/demo-service/service/common"
	notifysvc "github.com/example/demo-service/service/notify"
)
` + exportsTail,
		},
		{
			name:   "type 块按已有成员插入别名",
			before: exportsFixture,
			patch: &DeclInsert{
				Path:  "service/exports.go",
				Decl:  DeclType,
				Block: BlockRef{Member: "NotifyServiceImpl"},
				Name:  "NotifyTemplateServiceImpl",
				Value: "templatesvc.NotifyTemplateServiceImpl",
			},
			after: headerBlock + `package service

import (
	"context"
	"sync"

	"github.com/kamalyes/go-rpc-gateway/global"

	commonsvc "github.com/example/demo-service/service/common"
	notifysvc "github.com/example/demo-service/service/notify"
)

// ============================================================
// notify 包 - 通知服务
// ============================================================
type (
	NotifyServiceImpl = notifysvc.NotifyServiceImpl // 通知服务实现
	CredentialCipher  = notifysvc.CredentialCipher
	NotifyTemplateServiceImpl = templatesvc.NotifyTemplateServiceImpl
)

var (
	NewNotifyService = notifysvc.NewNotifyService // 创建通知服务
)

// 保留引用，避免 fixture 出现未使用的导入
// [EN] keep the references so the fixture has no unused imports
var (
	_ = context.Background
	_ sync.Once
	_ = global.DB
)
`,
		},
		{
			name:   "type 块按标记注释插入类型定义",
			before: exportsFixture,
			patch: &DeclInsert{
				Path:  "service/exports.go",
				Decl:  DeclType,
				Block: BlockRef{Marker: "notify 包 - 通知服务"},
				Name:  "NotifyID",
				Type:  "string",
			},
			after: headerBlock + `package service

import (
	"context"
	"sync"

	"github.com/kamalyes/go-rpc-gateway/global"

	commonsvc "github.com/example/demo-service/service/common"
	notifysvc "github.com/example/demo-service/service/notify"
)

// ============================================================
// notify 包 - 通知服务
// ============================================================
type (
	NotifyServiceImpl = notifysvc.NotifyServiceImpl // 通知服务实现
	CredentialCipher  = notifysvc.CredentialCipher
	NotifyID string
)

var (
	NewNotifyService = notifysvc.NewNotifyService // 创建通知服务
)

// 保留引用，避免 fixture 出现未使用的导入
// [EN] keep the references so the fixture has no unused imports
var (
	_ = context.Background
	_ sync.Once
	_ = global.DB
)
`,
		},
		{
			name:   "var 块按已有成员插入别名",
			before: exportsFixture,
			patch: &DeclInsert{
				Path:  "service/exports.go",
				Decl:  DeclVar,
				Block: BlockRef{Member: "NewNotifyService"},
				Name:  "NewNotifyTemplateService",
				Value: "templatesvc.NewNotifyTemplateService",
			},
			after: headerBlock + `package service

import (
	"context"
	"sync"

	"github.com/kamalyes/go-rpc-gateway/global"

	commonsvc "github.com/example/demo-service/service/common"
	notifysvc "github.com/example/demo-service/service/notify"
)

// ============================================================
// notify 包 - 通知服务
// ============================================================
type (
	NotifyServiceImpl = notifysvc.NotifyServiceImpl // 通知服务实现
	CredentialCipher  = notifysvc.CredentialCipher
)

var (
	NewNotifyService = notifysvc.NewNotifyService // 创建通知服务
	NewNotifyTemplateService = templatesvc.NewNotifyTemplateService
)

// 保留引用，避免 fixture 出现未使用的导入
// [EN] keep the references so the fixture has no unused imports
var (
	_ = context.Background
	_ sync.Once
	_ = global.DB
)
`,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, pl := mustApply(t, tc.before, tc.patch, FormatNever)
			if pl.Present {
				t.Fatalf("首次插入不应判为已存在")
			}
			if got != tc.after {
				t.Errorf("输出与预期不一致\n--- got ---\n%s\n--- want ---\n%s", got, tc.after)
			}
			assertCommentsKept(t, tc.before, got)
			if _, err := parser.ParseFile(token.NewFileSet(), "x.go", got, parser.SkipObjectResolution); err != nil {
				t.Errorf("输出不是合法 Go 源码: %v", err)
			}
		})
	}
}

// TestGoInsertIdempotent 同一条目插入两次，第二次必须是 no-op
// [EN] TestGoInsertIdempotent asserts the second application of a patch changes nothing
func TestGoInsertIdempotent(t *testing.T) {
	cases := []struct {
		name   string
		before string
		patch  Patch
	}{
		{"const", constFixture, &DeclInsert{
			Path: "a.go", Block: BlockRef{Member: "TableNameSecond"},
			Name: "TableNameOrder", Value: `"orders"`,
		}},
		{"struct 字段", factoryFixture, &StructFieldInsert{
			Path: "a.go", Struct: "RepositoryFactory", Name: "Order", Type: "*OrderRepository",
		}},
		{"复合字面量初始化项", factoryFixture, &CompositeFieldInsert{
			Path: "a.go", Func: "NewRepositoryFactory", Key: "Order", Value: "NewOrderRepository()",
		}},
		{"切片元素", migrateFixture, &SliceAppend{
			Path: "a.go", Func: "GetAllModels", Element: "&models.OrderModel{}",
		}},
		{"切片元素_排版不同也认得出", migrateFixture, &SliceAppend{
			Path: "a.go", Func: "GetAllModels", Element: "&models.SecondModel{}",
		}},
		{"注册语句", bootstrapFixture, &StmtInsert{
			Path: "a.go", Marker: "kronos:register=grpc",
			Stmt: "pb.RegisterOrderServiceServer(srv, s.orderSvc)",
		}},
		{"注册语句_空格不同也认得出", bootstrapFixture, &StmtInsert{
			Path: "a.go", Marker: "kronos:register=grpc",
			Stmt: "pb.RegisterExampleServiceServer(srv,   s.exampleSvc)",
		}},
		{"import", exportsFixture, &ImportInsert{
			Path: "a.go", Import: "github.com/example/demo-service/service/template",
			Alias: "templatesvc", Near: "github.com/example/demo-service/service/common",
		}},
		{"import_已有同路径同别名", exportsFixture, &ImportInsert{
			Path: "a.go", Import: "github.com/example/demo-service/service/notify",
			Alias: "notifysvc",
		}},
		{"type 别名", exportsFixture, &DeclInsert{
			Path: "a.go", Decl: DeclType, Block: BlockRef{Member: "NotifyServiceImpl"},
			Name: "NotifyTemplateServiceImpl", Value: "templatesvc.NotifyTemplateServiceImpl",
		}},
		{"type 定义", exportsFixture, &DeclInsert{
			Path: "a.go", Decl: DeclType, Block: BlockRef{Member: "CredentialCipher"},
			Name: "NotifyID", Type: "string",
		}},
		{"var 别名", exportsFixture, &DeclInsert{
			Path: "a.go", Decl: DeclVar, Block: BlockRef{Member: "NewNotifyService"},
			Name: "NewNotifyTemplateService", Value: "templatesvc.NewNotifyTemplateService",
		}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			once, pl1 := mustApply(t, tc.before, tc.patch, FormatNever)
			twice, pl2 := mustApply(t, once, tc.patch, FormatNever)
			if !pl2.Present {
				t.Errorf("第二次插入应判为已存在，实际 Present=false Insertions=%d", len(pl2.Insertions))
			}
			if !pl2.NoOp() {
				t.Errorf("第二次插入应为 no-op")
			}
			if twice != once {
				t.Errorf("重复执行产生了额外改动\n--- 第一次 ---\n%s\n--- 第二次 ---\n%s", once, twice)
			}
			if pl1.Present && once != tc.before {
				t.Errorf("已存在条目不应改动文件")
			}
		})
	}
}

// TestIdempotentIgnoresComments 注释和字符串里出现同名文本不应被误判为已存在
// 幂等判定走 AST 查询而不是 strings.Contains，这一条守住这个承诺
// [EN] TestIdempotentIgnoresComments asserts a same-named token inside a comment or a
// string literal is not mistaken for an existing entry
func TestIdempotentIgnoresComments(t *testing.T) {
	src := headerBlock + `package constants

// TableNameOrder 这里只是注释里提到了 TableNameOrder 这个名字
const (
	TableNameExample = "examples"
	// 下面这个字符串里也有 TableNameOrder
	TableNameHint = "TableNameOrder"
)
`
	p := &DeclInsert{
		Path:  "a.go",
		Block: BlockRef{Member: "TableNameExample"},
		Name:  "TableNameOrder",
		Value: `"orders"`,
	}
	got, pl := mustApply(t, src, p, FormatNever)
	if pl.Present {
		t.Fatalf("注释与字符串里的同名文本被误判为已声明")
	}
	if !strings.Contains(got, "\tTableNameOrder = \"orders\"\n") {
		t.Errorf("常量没有被插入\n%s", got)
	}
}

// TestFormatIfCleanKeepsHeader gofmt 归一化只补齐块内对齐，不碰文件头与其他注释
// [EN] TestFormatIfCleanKeepsHeader asserts the gofmt pass only fixes in-block alignment
func TestFormatIfCleanKeepsHeader(t *testing.T) {
	for _, src := range []string{constFixture, factoryFixture, migrateFixture, bootstrapFixture, exportsFixture} {
		if formatted, err := format.Source([]byte(src)); err != nil || string(formatted) != src {
			t.Fatalf("fixture 本身不是 gofmt 干净的，FormatIfClean 路径无法被覆盖")
		}
	}
	p := &DeclInsert{
		Path:  "constants/table.go",
		Block: BlockRef{Member: "TableNameSecond"},
		Name:  "TableNameOrderDetail",
		Value: `"order_details"`,
	}
	got, _ := mustApply(t, constFixture, p, FormatIfClean)
	assertCommentsKept(t, constFixture, got)
	if formatted, err := format.Source([]byte(got)); err != nil || string(formatted) != got {
		t.Errorf("FormatIfClean 的输出应当是 gofmt 干净的\n%s", got)
	}
	// 新常量名比原有成员更长，归一化应当把整块对齐重新补齐
	// [EN] the new name is longer, so the block alignment must be recomputed
	if !strings.Contains(got, "TableNameExample     = \"examples\"") {
		t.Errorf("归一化没有补齐 const 块对齐\n%s", got)
	}
}

// TestFormatNeverLeavesDirtyFileUntouched 原文件不是 gofmt 干净时，默认策略不整体重排
// [EN] TestFormatNeverLeavesDirtyFileUntouched asserts a non-gofmt-clean file is not reflowed
func TestFormatNeverLeavesDirtyFileUntouched(t *testing.T) {
	src := headerBlock + `package constants

const (
	TableNameExample     = "examples"
	TableNameSecond = "seconds"
)
`
	p := &DeclInsert{
		Path: "a.go", Block: BlockRef{Member: "TableNameSecond"},
		Name: "TableNameOrder", Value: `"orders"`,
	}
	got, _ := mustApply(t, src, p, FormatIfClean)
	if !strings.Contains(got, "\tTableNameSecond = \"seconds\"\n") {
		t.Errorf("原有的不规范排版被动了\n%s", got)
	}
}

// TestImportFormatIfCleanSortsWithinGroup gofmt 会在**组内**重排 import，但从不合并分组
// 所以 Near 只要把新条目放进正确的那一组，排序交给归一化即可
// [EN] TestImportFormatIfCleanSortsWithinGroup documents that gofmt reorders imports inside
// a group but never merges groups, so Near only has to pick the right group
func TestImportFormatIfCleanSortsWithinGroup(t *testing.T) {
	p := &ImportInsert{
		Path:   "service/exports.go",
		Import: "errors",
		Near:   "context",
	}
	got, _ := mustApply(t, exportsFixture, p, FormatIfClean)
	assertCommentsKept(t, exportsFixture, got)
	if formatted, err := format.Source([]byte(got)); err != nil || string(formatted) != got {
		t.Errorf("FormatIfClean 的输出应当是 gofmt 干净的\n%s", got)
	}
	want := `import (
	"context"
	"errors"
	"sync"

	"github.com/kamalyes/go-rpc-gateway/global"

	commonsvc "github.com/example/demo-service/service/common"
	notifysvc "github.com/example/demo-service/service/notify"
)
`
	if !strings.Contains(got, want) {
		t.Errorf("归一化后 import 块不符\n--- got ---\n%s\n--- want 片段 ---\n%s", got, want)
	}
}

// TestLocateErrors 目标锚点不存在时要给出可读报错，绝不 panic
// [EN] TestLocateErrors asserts missing anchors produce readable errors and never panic
func TestLocateErrors(t *testing.T) {
	cases := []struct {
		name    string
		before  string
		patch   Patch
		wantErr error
		wantMsg string
	}{
		{
			name:   "const 块标记不存在",
			before: constFixture,
			patch: &DeclInsert{Path: "a.go", Block: BlockRef{Marker: "kronos:consts=nope"},
				Name: "TableNameOrder", Value: `"orders"`},
			wantErr: ErrNotFound,
			wantMsg: "kronos:consts=nope",
		},
		{
			name:   "const 锚点命中单行声明",
			before: constFixture,
			patch: &DeclInsert{Path: "a.go", Block: BlockRef{Member: "ServiceName"},
				Name: "TableNameOrder", Value: `"orders"`},
			wantErr: ErrNotFound,
			wantMsg: "单行声明",
		},
		{
			// 免锚点只在唯一块时成立；两个块时猜哪个都是错的，必须报歧义
			// [EN] Anchorless resolution holds only for a sole block; with two of them any
			// guess would be wrong, so an ambiguity error is required
			name:    "const 有两个块时锚点为空",
			before:  twoConstBlocksFixture,
			patch:   &DeclInsert{Path: "a.go", Name: "TableNameOrder", Value: `"orders"`},
			wantErr: ErrInvalidPatch,
			wantMsg: "Marker 或 Member",
		},
		{
			name:    "常量名不合法",
			before:  constFixture,
			patch:   &DeclInsert{Path: "a.go", Block: BlockRef{Member: "TableNameSecond"}, Name: "1Bad", Value: `"x"`},
			wantErr: ErrInvalidPatch,
			wantMsg: "合法标识符",
		},
		{
			name:    "常量值不是合法表达式",
			before:  constFixture,
			patch:   &DeclInsert{Path: "a.go", Block: BlockRef{Member: "TableNameSecond"}, Name: "X", Value: "func("},
			wantErr: ErrInvalidPatch,
			wantMsg: "合法表达式",
		},
		{
			name:    "结构体不存在",
			before:  factoryFixture,
			patch:   &StructFieldInsert{Path: "a.go", Struct: "NoSuchFactory", Name: "Order", Type: "*OrderRepository"},
			wantErr: ErrNotFound,
			wantMsg: "NoSuchFactory",
		},
		{
			name:    "目标类型不是结构体",
			before:  headerBlock + "package constants\n\ntype TableName string\n",
			patch:   &StructFieldInsert{Path: "a.go", Struct: "TableName", Name: "Order", Type: "*OrderRepository"},
			wantErr: ErrNotFound,
			wantMsg: "不是结构体",
		},
		{
			name:    "构造函数不存在",
			before:  factoryFixture,
			patch:   &CompositeFieldInsert{Path: "a.go", Func: "NoSuchCtor", Key: "Order", Value: "NewOrderRepository()"},
			wantErr: ErrNotFound,
			wantMsg: "NoSuchCtor",
		},
		{
			name:    "复合字面量类型不匹配",
			before:  factoryFixture,
			patch:   &CompositeFieldInsert{Path: "a.go", Func: "NewRepositoryFactory", Type: "OtherFactory", Key: "Order", Value: "New()"},
			wantErr: ErrNotFound,
			wantMsg: "OtherFactory",
		},
		{
			name:    "切片所在函数不存在",
			before:  migrateFixture,
			patch:   &SliceAppend{Path: "a.go", Func: "GetNothing", Element: "&models.OrderModel{}"},
			wantErr: ErrNotFound,
			wantMsg: "GetNothing",
		},
		{
			name:    "切片包级变量不存在",
			before:  migrateFixture,
			patch:   &SliceAppend{Path: "a.go", Var: "NoSuchVar", Element: `"orders"`},
			wantErr: ErrNotFound,
			wantMsg: "NoSuchVar",
		},
		{
			name:    "切片锚点为空",
			before:  migrateFixture,
			patch:   &SliceAppend{Path: "a.go", Element: `"orders"`},
			wantErr: ErrInvalidPatch,
			wantMsg: "Marker、Near、Var 或 Func",
		},
		{
			name:    "切片元素不是合法表达式",
			before:  migrateFixture,
			patch:   &SliceAppend{Path: "a.go", Func: "GetAllModels", Element: "&models.OrderModel{"},
			wantErr: ErrInvalidPatch,
			wantMsg: "Element",
		},
		{
			name:    "兄弟语句锚点找不到",
			before:  bootstrapFixture,
			patch:   &StmtInsert{Path: "a.go", Near: "pb.RegisterNothing(srv)", Stmt: "f()"},
			wantErr: ErrNotFound,
			wantMsg: "pb.RegisterNothing(srv)",
		},
		{
			name:    "兄弟元素锚点找不到",
			before:  migrateFixture,
			patch:   &SliceAppend{Path: "a.go", Near: "&models.NothingModel{}", Element: "&models.OrderModel{}"},
			wantErr: ErrNotFound,
			wantMsg: "&models.NothingModel{}",
		},
		{
			name:    "After 缺少 Near 锚点",
			before:  bootstrapFixture,
			patch:   &StmtInsert{Path: "a.go", Func: "initServices", After: true, Stmt: "f()"},
			wantErr: ErrInvalidPatch,
			wantMsg: "After 需要配合 Near 使用",
		},
		{
			name:    "标记注释不存在",
			before:  bootstrapFixture,
			patch:   &StmtInsert{Path: "a.go", Marker: "kronos:register=nope", Stmt: "f()"},
			wantErr: ErrNotFound,
			wantMsg: "kronos:register=nope",
		},
		{
			name:    "语句片段不合法",
			before:  bootstrapFixture,
			patch:   &StmtInsert{Path: "a.go", Func: "initServices", Stmt: "if {"},
			wantErr: ErrInvalidPatch,
			wantMsg: "Stmt",
		},
		{
			name:   "import 同路径不同别名要报冲突",
			before: exportsFixture,
			patch: &ImportInsert{Path: "a.go", Alias: "notify",
				Import: "github.com/example/demo-service/service/notify"},
			wantErr: ErrConflict,
			wantMsg: `已经以别名 "notifysvc" 导入过`,
		},
		{
			name:   "import 同路径从有别名变无别名也报冲突",
			before: exportsFixture,
			patch: &ImportInsert{Path: "a.go",
				Import: "github.com/example/demo-service/service/notify"},
			wantErr: ErrConflict,
			wantMsg: "与期望的别名 <无别名> 冲突",
		},
		{
			name:    "import 锚点路径不在 import 块里",
			before:  exportsFixture,
			patch:   &ImportInsert{Path: "a.go", Import: "errors", Near: "nope/not/here"},
			wantErr: ErrNotFound,
			wantMsg: "nope/not/here",
		},
		{
			name:    "import 单行声明无法追加",
			before:  singleImportFixture,
			patch:   &ImportInsert{Path: "a.go", Import: "errors"},
			wantErr: ErrNotFound,
			wantMsg: "只有单行 import 声明",
		},
		{
			name:    "import 单行声明命中 Near 也要报错",
			before:  singleImportFixture,
			patch:   &ImportInsert{Path: "a.go", Import: "errors", Near: "context"},
			wantErr: ErrNotFound,
			wantMsg: "命中的是单行 import 声明",
		},
		{
			name:    "文件里没有 import 声明",
			before:  noImportFixture,
			patch:   &ImportInsert{Path: "a.go", Import: "errors"},
			wantErr: ErrNotFound,
			wantMsg: "没有 import 声明",
		},
		{
			name:    "import 路径为空",
			before:  exportsFixture,
			patch:   &ImportInsert{Path: "a.go"},
			wantErr: ErrInvalidPatch,
			wantMsg: "Import 不能为空",
		},
		{
			name:    "import 别名不合法",
			before:  exportsFixture,
			patch:   &ImportInsert{Path: "a.go", Import: "errors", Alias: "1bad"},
			wantErr: ErrInvalidPatch,
			wantMsg: "别名",
		},
		{
			name:    "type 块不存在",
			before:  constFixture,
			patch:   &DeclInsert{Path: "a.go", Decl: DeclType, Block: BlockRef{Member: "Nope"}, Name: "X", Value: "Y"},
			wantErr: ErrNotFound,
			wantMsg: "没有任何 type 声明",
		},
		{
			name:    "type 同时给了 Value 与 Type",
			before:  exportsFixture,
			patch:   &DeclInsert{Path: "a.go", Decl: DeclType, Block: BlockRef{Member: "NotifyServiceImpl"}, Name: "X", Value: "Y", Type: "string"},
			wantErr: ErrInvalidPatch,
			wantMsg: "只能二选一",
		},
		{
			name:    "type 既没给 Value 也没给 Type",
			before:  exportsFixture,
			patch:   &DeclInsert{Path: "a.go", Decl: DeclType, Block: BlockRef{Member: "NotifyServiceImpl"}, Name: "X"},
			wantErr: ErrInvalidPatch,
			wantMsg: "需要 Value（别名）或 Type（类型定义）之一",
		},
		{
			name:    "未知的 Decl",
			before:  exportsFixture,
			patch:   &DeclInsert{Path: "a.go", Decl: "func", Block: BlockRef{Member: "X"}, Name: "Y", Value: "1"},
			wantErr: ErrInvalidPatch,
			wantMsg: "未知的 Decl",
		},
		{
			name:    "目标文件不是合法 Go 源码",
			before:  "package ???",
			patch:   &DeclInsert{Path: "a.go", Block: BlockRef{Member: "X"}, Name: "Y", Value: "1"},
			wantErr: ErrParse,
			wantMsg: "a.go",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, _, err := ApplyToSource([]byte(tc.before), tc.patch, FormatNever)
			if err == nil {
				t.Fatalf("预期报错，实际成功")
			}
			if !errors.Is(err, tc.wantErr) {
				t.Errorf("错误类型不符: got %v, want %v", err, tc.wantErr)
			}
			if !strings.Contains(err.Error(), tc.wantMsg) {
				t.Errorf("错误信息里应当含有 %q，实际为 %q", tc.wantMsg, err.Error())
			}
		})
	}
}

// TestNormalizeCodeIgnoresLineBreaksAndKeepsLiterals 归一化必须与换行无关，又不能改写字符串字面量
// 多行链式调用是真实代码里最常见的形状，清单作者只会写成一行
// [EN] TestNormalizeCodeIgnoresLineBreaksAndKeepsLiterals asserts normalization is newline
// agnostic yet never rewrites string literals
func TestNormalizeCodeIgnoresLineBreaksAndKeepsLiterals(t *testing.T) {
	multi := "cachex.NewModelKV[models.WarehouseModel](base, db).\n\tFields(\"Name\", \"Code\").\n\tRegister()"
	single := `cachex.NewModelKV[models.WarehouseModel](base, db).Fields("Name", "Code").Register()`
	if normalizeCode(multi) != normalizeCode(single) {
		t.Errorf("多行与单行写法应当归一化成同一个 token 流\n%q\n%q",
			normalizeCode(multi), normalizeCode(single))
	}
	// 字面量里的空格属于内容，不能被当成排版抹掉
	// [EN] spaces inside a literal are content, not layout
	if normalizeCode(`f("a. b")`) == normalizeCode(`f("a.b")`) {
		t.Errorf("字符串字面量里的空格被当成排版抹掉了")
	}
}

// TestStmtNearAcceptsSingleLineForm 多行链式语句用单行形式当 near 锚点就该命中
// [EN] TestStmtNearAcceptsSingleLineForm matches a multi-line chained statement by its
// single-line form, which is how a manifest author naturally writes it
func TestStmtNearAcceptsSingleLineForm(t *testing.T) {
	src := headerBlock + `package bootstrap

// registerCaches 注册缓存
func registerCaches() {
	cachex.NewModelKV[models.ExampleModel](base, db).
		Fields("Name", "Code").
		Register()
}
`
	p := &StmtInsert{
		Path:  "a.go",
		Near:  `cachex.NewModelKV[models.ExampleModel](base, db).Fields("Name", "Code").Register()`,
		After: true,
		Stmt:  "cachex.NewModelCache[models.OrderModel](base, db).\n\tKeyFields(\"TenantId\", \"Id\").\n\tRegister()",
	}
	got, pl := mustApply(t, src, p, FormatNever)
	if pl.Present {
		t.Fatalf("不应判为已存在")
	}
	want := `	cachex.NewModelKV[models.ExampleModel](base, db).
		Fields("Name", "Code").
		Register()
	cachex.NewModelCache[models.OrderModel](base, db).
		KeyFields("TenantId", "Id").
		Register()
}
`
	if !strings.Contains(got, want) {
		t.Errorf("输出不符\n--- got ---\n%s", got)
	}
	// 幂等：同一条多行语句再插一次应为 no-op
	// [EN] idempotency: inserting the same multi-line statement again is a no-op
	twice, pl2 := mustApply(t, got, p, FormatNever)
	if !pl2.Present || twice != got {
		t.Errorf("多行语句的幂等判定失效")
	}
}
