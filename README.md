# Kronos Code Generator

VSCode 插件，用于从 Proto 定义、Go Model 结构体或数据库表结构自动生成 Go 微服务 CRUD 代码

## 功能

### 1. 从 Proto 生成代码
在 `.proto` 文件中右键 → `Kronos: 从 Proto 生成代码`，自动识别 `XxxInfo` Message 并生成：
- **Model** - GORM 结构体 + TableName + TableComment + PBMO 转换（含字段中文注释）
- **Repository** - CRUD 方法 + 条件查询 + 分页（含方法中文注释）
- **Service** - 业务逻辑（Create/Get/List/Update/Delete）（含方法中文注释）
- **Error Codes** - BizErrCode 错误常量
- **Factory 注册** - RepositoryFactory 注册片段

### 2. 从 Model 生成代码
在 `.go` 文件中选中 Model 结构体 → 右键 → `Kronos: 从 Model 生成代码`，自动解析并生成 Repository/Service/Errors/Factory

> 不选中时自动弹出 Model 选择器，支持搜索，默认选中第一个

### 3. 从数据库表生成代码
命令面板 → `Kronos: 从数据库表生成代码`，支持：
- **MySQL** - 内置 mysql2 驱动直连，无需安装 CLI
- **PostgreSQL** - 内置 pg 驱动直连，无需安装 CLI
- 多表批量选择
- 连接配置保存
- **包名可配置** - 可自定义 models/repository/service/errors 的包名和目录名
- **直接写入文件** - 自动检测 go.mod 项目，一键写入对应目录

### 4. 从数据库表生成 Proto
命令面板 → `Kronos: 从数据库表生成 Proto`，自动：
- 读取数据库表结构
- 生成 Proto Message 定义（XxxInfo）
- 生成 CRUD 请求/响应 Message
- 生成 Service RPC 定义
- 可选同时生成 Go 代码

## 环境要求

| 依赖 | 版本要求 | 说明 |
|------|---------|------|
| Node.js | >= 18.0.0 | 运行时环境 |
| Yarn | >= 1.22.0 | 包管理器 |
| VSCode | >= 1.85.0 | 插件宿主 |
| TypeScript | 5.5.3 | 编译器（固定版本） |

## 快速开始

### 安装依赖

```bash
# 使用 yarn（推荐）
yarn install

# 或使用 npm
npm install
```

### 编译

```bash
yarn compile
```

### 开发调试

1. 在 VSCode 中打开 `kronos-codegen` 目录
2. 按 `F5` 启动 Extension Development Host
3. 在调试窗口中打开你的项目进行测试

### 打包 VSIX

```bash
# 安装 vsce 工具（仅需一次）
yarn global add @vscode/vsce

# 打包
yarn package

# 产物：kronos-codegen-0.2.0.vsix
# 打包 VSIX（包含依赖）
vsce package --yarn --allow-missing-repository
```

### 安装 VSIX

**VSCode：**
```bash
# 命令行安装
code --install-extension kronos-codegen-0.2.0.vsix

# 或在 VSCode 中：扩展 → ... → 从 VSIX 安装
```

**Trae IDE：**
```bash
# 命令行安装
trae --install-extension kronos-codegen-0.2.0.vsix

# 或在 Trae 中：扩展面板 → ... → 从 VSIX 安装
```

> Trae IDE 基于 VSCode 架构，完全兼容 VSIX 插件格式，安装方式与 VSCode 一致

## 配置

### VSCode 设置

插件在 `settings.json` 中提供以下配置项：

```jsonc
{
  // ========== 头部注释配置 ==========

  // 是否在生成的代码中添加文件头部注释（@Author/@Date/@Description 等）
  "kronos-codegen.enableFileHeader": true,

  // 生成代码头部注释中的作者名
  "kronos-codegen.author": "kamalyes 501893067@qq.com",

  // 生成代码头部注释中的版权归属方
  "kronos-codegen.publisher": "kamalyes",

  // ========== 包名配置 ==========

  // Model 层的 Go 包名（目录名），生成时输入框的默认值
  "kronos-codegen.modelsPackageName": "models",

  // Repository 层的 Go 包名（目录名），生成时输入框的默认值
  "kronos-codegen.repositoryPackageName": "repository",

  // Service 层的 Go 包名（目录名），生成时输入框的默认值
  "kronos-codegen.servicePackageName": "service",

  // 错误码的 Go 包名（目录名），生成时输入框的默认值
  "kronos-codegen.errorsPackageName": "errors",

  // ========== 数据库连接配置 ==========

  // 数据库连接配置（自动管理，无需手动编辑）
  "kronos-codegen.databaseConnections": [
    {
      "type": "mysql",
      "host": "127.0.0.1",
      "port": 3306,
      "username": "root",
      "password": "your_password",
      "database": "your_database",
      "name": "MySQL-你的数据库"
    },
    {
      "type": "postgresql",
      "host": "127.0.0.1",
      "port": 5432,
      "username": "postgres",
      "password": "your_password",
      "database": "your_database",
      "name": "PostgreSQL-你的数据库"
    }
  ]
}
```

### 配置项一览

| 配置项 | 类型 | 默认值 | 说明 |
|--------|------|--------|------|
| `enableFileHeader` | `boolean` | `true` | 是否启用头部注释，设为 `false` 则不生成 |
| `author` | `string` | `"kronos-codegen"` | `@Author` 和 `@LastEditors` 字段的值 |
| `publisher` | `string` | `"kronos-team"` | `Copyright` 行的归属方 |
| `modelsPackageName` | `string` | `"models"` | Model 层的 Go 包名（目录名） |
| `repositoryPackageName` | `string` | `"repository"` | Repository 层的 Go 包名（目录名） |
| `servicePackageName` | `string` | `"service"` | Service 层的 Go 包名（目录名） |
| `errorsPackageName` | `string` | `"errors"` | 错误码的 Go 包名（目录名） |
| `databaseConnections` | `array` | `[]` | 数据库连接配置列表 |

### 头部注释说明

启用头部注释后，生成的代码文件顶部会自动添加与项目风格一致的注释块：

```go
/*
 * @Author: kamalyes 501893067@qq.com
 * @Date: 2026-04-25 10:30:00
 * @LastEditors: kamalyes 501893067@qq.com
 * @LastEditTime: 2026-04-25 10:30:00
 * @FilePath: \kronos-service\models\user_model.go
 * @Description: 用户模型定义 - 由 Kronos Code Generator 自动生成
 *
 * Copyright (c) 2026 by kamalyes, All Rights Reserved.
 */
```

### .npmrc 配置

项目已配置国内镜像加速：

```ini
registry=https://registry.npmmirror.com
```

## 使用方式

### 从 Proto 生成
1. 打开 `.proto` 文件
2. 右键 → `Kronos: 从 Proto 生成代码`
3. 选择要生成的实体
4. 选择 Service 风格
5. 确认/修改自动检测的 Module 和 PB 包路径
6. 在 Webview 面板中预览和复制代码

### 从 Model 生成
1. 打开 `.go` 文件
2. 选中 `type XxxModel struct { ... }` 定义（或不选中，弹出选择器）
3. 右键 → `Kronos: 从 Model 生成代码`
4. 后续步骤同上

### 从数据库生成
1. `Ctrl+Shift+P` 打开命令面板
2. 输入 `Kronos: 从数据库表生成代码`
3. 选择/新建数据库连接
4. 选择要生成的表（支持多选）
5. 选择 Service 风格
6. 确认/修改 Module 和 PB 包路径
7. **配置包名** - 设置 models/repository/service/errors 的包名（目录名）
8. **选择输出方式**：
   - **预览代码** - 在 Webview 面板中查看和复制
   - **直接写入文件** - 自动检测 go.mod 项目，写入对应目录

## 包名配置

从数据库生成代码时，可以自定义各层的包名和目录名，适配不同项目结构：

| 配置项 | 默认值 | 说明 |
|--------|--------|------|
| Models 包名 | `models` | Model 层的 Go 包名和目录名 |
| Repository 包名 | `repository` | Repository 层的 Go 包名和目录名 |
| Service 包名 | `service` | Service 层的 Go 包名和目录名 |
| Errors 包名 | `errors` | 错误码的 Go 包名和目录名 |

生成的代码会自动使用配置的包名：
- `package` 声明使用配置的包名
- `import` 路径自动拼接 `github.com/{module}/{packageName}`
- 文件路径使用 `{packageName}/{entity}_model.go` 格式

### 示例

假设 Module 为 `github.com/kamalyes/apex-service`，配置包名：
- Models: `models`
- Repository: `repository`
- Service: `service`
- Errors: `errors`

生成的文件结构：
```
apex-service/
├── models/
│   └── alarm_model.go          # package models
├── repository/
│   └── alarm_repository.go     # package repository, import "github.com/kamalyes/apex-service/models"
├── service/
│   └── alarm_service.go        # package service, import models & errors
└── errors/
    └── codes.go                # 错误码常量
```

## 直接写入文件

选择"直接写入文件"输出方式时：

1. 插件自动扫描工作区中包含 `go.mod` 的项目目录
2. 如果找到多个项目，弹出选择器让你选择目标项目
3. 如果只有一个项目，直接使用
4. 如果没有找到，弹出文件夹选择器手动选择
5. 代码自动写入到项目的对应包目录下
6. 已存在的文件会提示是否覆盖

## 表名前缀处理

数据库表名常见前缀会自动去除，确保生成的实体名正确：

| 表名 | 实体名 | 说明 |
|------|--------|------|
| `t_alarm` | `Alarm` | 去除 `t_` 前缀 |
| `tb_user` | `User` | 去除 `tb_` 前缀 |
| `tbl_order` | `Order` | 去除 `tbl_` 前缀 |
| `alarm` | `Alarm` | 无前缀直接转驼峰 |

## 代码注释

生成的代码包含完整的中文注释：

### Model 字段注释
```go
type AlarmModel struct {
    // Id 主键
    Id string `gorm:"column:id;primaryKey;type:varchar(36);comment:主键" json:"id"`
    // AppName 应用名
    AppName string `gorm:"column:app_name;type:varchar(100);comment:应用名" json:"appName"`
}
```

### Repository 方法注释
```go
// Create 创建告警记录
func (r *AlarmRepository) Create(ctx context.Context, m *models.AlarmModel) (*models.AlarmModel, error) {
```

### Service 方法注释
```go
// AlarmCreate 创建告警记录
func (s *ServiceImpl) AlarmCreate(ctx context.Context, req *pb.AlarmCreateRequest) (*pb.AlarmCreateResponse, error) {
```

## 自动检测

插件会自动从工作区检测以下信息，减少手动输入：
- **Go Module** - 从 `go.mod` 读取 module 路径
- **PB 包路径** - 从现有 model 文件的 import 语句提取
- **Enum 包路径** - 自动识别 enumspb 导入
- **Proto package** - 从 proto 文件提取 go_package
- **go.mod 项目目录** - 自动扫描工作区中的 Go 项目

## Service 风格

| 风格 | 说明 | 适用场景 |
|------|------|----------|
| 单 Service 结构体 | 所有方法挂在同一个 `ServiceImpl` 上 | 单体服务，如 AccessControlServiceImpl |
| 独立 Service 结构体 | 每个领域独立 Service 结构体 | 多领域服务，如 QuickReplyService |

## 数据库连接

### 前置条件
- **MySQL**: 无需安装 CLI，插件内置 mysql2 驱动直连
- **PostgreSQL**: 无需安装 CLI，插件内置 pg 驱动直连

### 连接流程
1. 首次使用选择「新建连接」
2. 输入数据库类型、地址、端口、用户名、密码、数据库名
3. 可选保存连接配置到工作区
4. 下次使用直接选择已保存的连接

## 版本依赖锁定

`package.json` 中所有依赖使用固定版本号（无 `^` / `~` 前缀），确保构建一致性：

```json
{
  "devDependencies": {
    "@types/node": "20.14.9",
    "@types/vscode": "1.85.0",
    "typescript": "5.5.3"
  }
}
```

## License

MIT
