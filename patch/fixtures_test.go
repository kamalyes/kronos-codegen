/*
 * @Author: kamalyes 501893067@qq.com
 * @Date: 2026-11-01 18:05:12
 * @LastEditors: kamalyes 501893067@qq.com
 * @LastEditTime: 2026-11-01 20:12:06
 * @FilePath: \kronos-codegen\patch\fixtures_test.go
 * @Description: 测试用的 before 源码 fixture - 形态对齐 kronos-service-template 的真实文件
 *
 * Copyright (c) 2026 by kamalyes, All Rights Reserved.
 */
package patch

// headerBlock 文件头版权注释块，所有 Go fixture 共用
// 专门用来验证补丁不会动文件头
// [EN] headerBlock is the shared copyright header used to assert it stays untouched
const headerBlock = `/*
 * @Author: kamalyes 501893067@qq.com
 * @Date: 2026-11-01 18:05:12
 * @Description: fixture
 *
 * Copyright (c) 2026 by kamalyes, All Rights Reserved.
 */
`

// constFixture 带标记注释的 const 块，外加一个单行 const 声明
// [EN] constFixture holds a marked const block plus a standalone const declaration
const constFixture = headerBlock + `package constants

// 表名常量 kronos:consts=table-name
const (
	// TableNameExample 示例表
	TableNameExample = "examples"
	TableNameSecond  = "seconds" // 第二张表
)

// ServiceName 服务名
const ServiceName = "demo"
`

// twoConstBlocksFixture 同一文件里两个带括号的 const 块
// 用来钉住「免锚点只在唯一块时成立」：有两个时必须报歧义而不是挑一个
//
// [EN] Two parenthesized const blocks in one file, pinning that anchorless resolution
// holds only for a sole block: with two of them an ambiguity error is required rather
// than a guess
const twoConstBlocksFixture = headerBlock + `package constants

const (
	TableNameExample = "examples"
)

const (
	CacheKeyPrefix = "demo:"
)
`

// factoryFixture 仓库工厂：struct 字段与构造函数初始化项成对出现
// [EN] factoryFixture is a repository factory: struct fields paired with constructor items
const factoryFixture = headerBlock + `package repository

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
		}
	})
	return factoryInstance
}
`

// emptyFactoryFixture 还没有任何字段的工厂，块里只剩一行 TODO 注释
// [EN] emptyFactoryFixture is a factory with no members yet, only a TODO comment
const emptyFactoryFixture = headerBlock + `package repository

// RepositoryFactory 仓库工厂结构体
type RepositoryFactory struct {
	// TODO: 添加仓库字段
}

// GetAllModels 获取所有需要初始化的数据库模型
func GetAllModels() []interface{} {
	return []interface{}{
		// TODO: 添加模型
	}
}
`

// migrateFixture 迁移模型清单
// [EN] migrateFixture is the migration model list
const migrateFixture = headerBlock + `package migrations

// GetAllModels 获取所有需要初始化的数据库模型
func GetAllModels() []interface{} {
	return []interface{}{
		&models.ExampleModel{},
		&models.SecondModel{}, // 第二个模型
	}
}

// AllTables 单行切片字面量，用来覆盖紧凑块的插入路径
// [EN] AllTables is a single-line slice literal covering the compact path
var AllTables = []string{"examples"}
`

// bootstrapFixture 服务注册：目标块是闭包体，只能靠标记注释钻进去
// [EN] bootstrapFixture registers services inside a closure reachable only by marker
const bootstrapFixture = headerBlock + `package bootstrap

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
}
`

// localeFixture 两空格缩进的 locale 文件
// [EN] localeFixture is a two-space indented locale file
const localeFixture = `{
  "welcome": "Welcome",
  "success": "Success",
  "user": {
    "not_found": "User not found"
  },
  "goodbye": "Goodbye"
}
`

// localeCompactFixture 紧凑单行 JSON
// [EN] localeCompactFixture is a compact single-line JSON document
const localeCompactFixture = `{"welcome":"Welcome","success":"Success"}`

// localeEmptyFixture 空对象 JSON
// [EN] localeEmptyFixture is an empty JSON object
const localeEmptyFixture = `{
}
`

// exportsFixture 服务统一导出文件，形态对齐线上服务的 service/exports.go
// import 块按标准库 / 三方 / 内部分成三组，type 与 var 块做跨包别名
// [EN] exportsFixture mirrors a production service's service/exports.go: a three-group import
// block plus type and var blocks holding cross-package aliases
const exportsFixture = headerBlock + `package service

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
)

// 保留引用，避免 fixture 出现未使用的导入
// [EN] keep the references so the fixture has no unused imports
var (
	_ = context.Background
	_ sync.Once
	_ = global.DB
)
`

// singleImportFixture 单行 import 形式，用来覆盖无法追加时的报错路径
// [EN] singleImportFixture is the non-parenthesized form covering the failure path
const singleImportFixture = headerBlock + `package service

import "context"

var _ = context.Background
`

// noImportFixture 完全没有 import 声明
// [EN] noImportFixture has no import declaration at all
const noImportFixture = headerBlock + `package service

type Marker struct{}
`

// exportsTail exportsFixture 里 import 块之后的部分，golden 用例复用它避免大段重复
// [EN] exportsTail is everything after the import block, reused by the golden cases
const exportsTail = `
// ============================================================
// notify 包 - 通知服务
// ============================================================
type (
	NotifyServiceImpl = notifysvc.NotifyServiceImpl // 通知服务实现
	CredentialCipher  = notifysvc.CredentialCipher
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
`
