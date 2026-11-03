# kronos-codegen - 幂等代码生成器

> `kronos-codegen` 是一个用 Go 写的代码生成器：新增一个实体，它既按项目自己的模板**生成新文件**，也往七八个**已存在的文件**里接线：表名常量、仓库工厂的字段与初始化项、迁移的模型清单、服务注册语句、跨包别名、几十个 locale 文件的键，每一处插入都是**幂等**的(重复执行不产生重复条目)，并且除插入点之外**逐字节不变**——文件头的版权注释块、手工排版、空行分组都不会被动

> [EN] `kronos-codegen` is a Go code generator for the hard half of scaffolding: adding one entity means editing files that already exist — table-name constants, repository factory fields and their constructor items, the migration model list, service registration statements, cross-package aliases, and a key in dozens of locale files. Every insertion is idempotent, and every byte outside the insertion point is preserved verbatim, so copyright headers, hand-tuned alignment and blank-line grouping survive untouched.

[![stable](https://img.shields.io/badge/stable-beta-yellow.svg)](https://github.com/kamalyes/kronos-codegen)
[![license](https://img.shields.io/github/license/kamalyes/kronos-codegen)]()
[![commit](https://img.shields.io/github/last-commit/kamalyes/kronos-codegen)]()
[![issues](https://img.shields.io/github/issues/kamalyes/kronos-codegen)]()
[![go](https://img.shields.io/github/go-mod/go-version/kamalyes/kronos-codegen)]()
[![size](https://img.shields.io/github/repo-size/kamalyes/kronos-codegen)]()
[![Go Reference](https://pkg.go.dev/badge/github.com/kamalyes/kronos-codegen?status.svg)](https://pkg.go.dev/github.com/kamalyes/kronos-codegen?tab=doc)

## 📚 文档导航 | Documentation

### 📖 本仓库文档 | In this repository

| 文档 \| Document | 内容 \| Contents |
| --- | --- |
| [清单参考 \| Manifest reference](./docs/MANIFEST.md) | `.kronos-codegen.json` 的顶层结构、模板变量、锚点字段与免锚点规则、三个闸门字段,以及九种 `kind` 逐个的字段表与示例 |
| [可运行例子 \| Runnable demo](./demo/README.md) | 自带契约的最小项目:一条命令建 3 个文件、接线 9 处,生成出来的代码能编译 复位脚本可反复演示 |

### 📂 代码位置 | Code layout

| 位置 \| Location | 内容 \| Contents |
| --- | --- |
| [`patch/`](./patch) | 幂等补丁引擎：AST 定位插入点 + 字节级文本拼接、七类插入、unified diff、原子写盘 |
| [`plan/`](./plan) | 声明式清单(`.kronos-codegen.json`)的解析与展开、实体名的命名派生、三个闸门字段 |
| [`pbfield/`](./pbfield) | 从 protoc 生成的 `.pb.go` 里提取消息字段,映射成 GORM 模型字段;推断契约包的导入路径与别名、探测有没有对应的 gRPC 服务 |
| [`builtin/`](./builtin) | 内置的补丁计划与三个模板,用 `go:embed` 嵌进二进制 |
| [`demo/`](./demo) | 可运行的最小例子,自带一份零外部依赖的假契约 |
| `main.go` / `cli.go` / `new.go` / `check.go` / `init.go` | 命令行外壳：全局 flag、子命令编排、确认与输出 |

- [📖 API 文档 \| API reference](https://pkg.go.dev/github.com/kamalyes/kronos-codegen)
- [🐛 问题反馈 \| Issues](https://github.com/kamalyes/kronos-codegen/issues)

## 🚀 核心特性 | Features

- 🧬 **AST 定位 + 文本拼接 | Locate with AST, splice with text** — `go/parser` 只负责算出插入点的字节偏移，修改是在原始字节上做切片拼接，不走 `go/printer` 整文件回写，因此没有 diff 噪音
- ♻️ **全类型幂等 | Idempotent everywhere** — 重复执行是 no-op；幂等判定一律走 AST 查询(声明名、`KeyValueExpr` 的 key、`go/printer` 归一化后的语句文本)，注释或字符串字面量里的同名文本不会被误判
- 🏗️ **建文件 + 接线一体 | Create and wire in one pass** — `create_file` 按项目内的 `text/template` 模板生成新文件，与插入步骤同一个事务；新建的 Go 文件一律 gofmt，渲染结果不合法就报错而不是落一个坏 `.go`
- 🧩 **七类插入 | Seven insertion kinds** — `const` / `var` / `type` 括号块、`import` 块、struct 字段、复合字面量初始化项、切片元素、函数体与闭包体里的语句、JSON 键
- 🎯 **四种锚点 | Four anchor styles** — 标记注释 `marker`、块内已有成员 `member`、已有兄弟条目 `near`、函数名 `func`(+`recv`);`near` 让生产代码**不必先埋标记注释**就能当锚点
- 🔢 **JSON 保序 | Order-preserving JSON** — 走 `json.Decoder` 的 token 流只取字节偏移，不经过 `map`，键顺序与缩进风格原样保留，28 个语言文件不会全量变脏
- 🏃 **写盘前预览 | Preview before writing** — `--dry-run` 给变更摘要(文件：行号 + 插入片段)与标准 unified diff，不加 `--yes` 还会在写盘前询问
- 🔍 **一致性校验 | Drift check** — `check --entity` 复用同一套幂等判定，逐条报哪些条目还没落地，漂移时退出码 3，可直接接 CI
- 🛡️ **全有或全无 | All or nothing** — 改动先在内存里累积，任何一条锚点找不到就整批中止，工作区零改动；落盘走临时文件 + rename，并保留原文件权限
- 📐 **排版可控 | Controlled formatting** — `--format if-clean`(默认)只在原文件本身已经 gofmt 干净时才做归一化，手工排版的文件永远不会被整体重排
- 📦 **零三方依赖 | Zero dependencies** — 只用 Go 标准库，没有 `go.sum`
- 🔌 **引擎可复用 | Reusable engine** — `patch` 与 `plan` 是普通的公开包，别的工具可以直接 import
- 📥 **内置清单 | Built-in manifest** — 约定形态的服务**零配置可用**，不必先写一份清单；项目放一份自己的就整体覆盖，`init` 可以把内置的取出来改
- 🧾 **字段来自契约 | Fields from the contract** — `--pb` 指向 protoc 生成的 `.pb.go`，模型按契约的字段生成，连双语注释一起带过来；读 `.pb.go` 而不是 `.proto` 是为了直接用 protoc 的类型映射，不可能与它不一致，也不必为解析 proto 文法引一条重依赖
- 🚧 **三个闸门 | Three gates** — `optional` 管文件不存在、`when` 管模板变量为空、`group` 管同组定位不到就整组跳过；最后一个是为了避免**部分接线**——不接线是留个人工步骤，部分接线是先把构建弄坏再让人修

## ⚠️ 六个必读行为 | Six behaviours to know first

写清单之前先过一遍这六条，每一条背后都是一个会让人困惑半小时的真实结果

[EN] Read these six before writing a manifest; each one is a real half-hour of confusion

| # | 行为 \| Behaviour | 不知道的后果 \| What goes wrong |
| --- | --- | --- |
| 1 | `--format if-clean`(默认)**只在原文件本身已经 gofmt 干净时**才归一化 | 带模板占位符或排版不规范的文件里，新插入的成员不会自动对齐(这是故意的，否则会整文件重排)；而在干净文件里 gofmt **会在 import 组内重排**，所以 `near` 只要选对分组，排序交给归一化 |
| 2 | `import` 的幂等按**导入路径**判定，不是别名 | 同一路径已经以别名 `notifysvc` 导入过，清单又要求别名 `notify`，会直接报冲突并退出 1，而不是插第二条——同路径导入两次根本编译不过 |
| 3 | JSON 键已存在就跳过，**绝不覆盖已有译文** | 想改译文得手动改或先删键；清单改了 `fallback` 对已落地的文件没有任何作用 |
| 4 | 插入步骤只往**已存在的文件**里加条目，不会创建文件，也不会创建 `const ( ... )` / `import ( ... )` 这些块——建文件是 `create_file` 的事 | 目标文件或括号块不存在时报 `patch: target anchor not found` 并退出 1；单行 `import "x"`、单行 `const X = 1` 没有可追加的位置，报错会明确提示先改成括号块 |
| 5 | `create_file` 的目标已存在就跳过，**绝不覆盖** | 想重新生成得自己先删掉旧文件；没有 `--force` 这种开关是故意的，手写过的实现不该被一条命令悄悄顶掉 |
| 6 | `const` / `var` / `type` 的幂等按「**文件里是否已声明过同名成员**」判定，不限于目标块 | 同名成员如果声明在同一文件的另一个块里，本次会被判为「已存在」而跳过，不会再插一条 |

## 📦 安装 | Installation

```bash
# 安装到 GOPATH/bin | install into GOPATH/bin
go install github.com/kamalyes/kronos-codegen@latest

# 从源码构建 | build from source
git clone https://github.com/kamalyes/kronos-codegen.git && cd kronos-codegen
go build -o kronos-codegen .

# 交叉编译 | cross compile
GOOS=linux   GOARCH=arm64 go build -o kronos-codegen-linux-arm64 .
GOOS=windows GOARCH=amd64 go build -o kronos-codegen.exe .
```

## 🚀 快速开始 | Quick start

```bash
# 1. 先看会改什么，不写盘 | preview only
kronos-codegen new entity Order --root ./my-service --dry-run

# 2. 确认后写盘（会询问一次）| write after one confirmation
kronos-codegen new entity Order --root ./my-service

# 3. CI 里零交互 | non-interactive
kronos-codegen new entity Order --root ./my-service --yes

# 4. 校验某个实体的条目是否都已落地 | check one entity for drift
kronos-codegen check --entity Order --root ./my-service

# 5. 按契约生成：模型字段取自 .pb.go 里的消息 | derive model fields from the contract
kronos-codegen new entity Role --root ./my-service \
  --pb ../share-proto/pb/common/user.pb.go --yes
```

不给 `--pb` 时模型只生成骨架:字段是一个实体的实质,没有契约不该由生成器编出来

[EN] Without `--pb` the model is a skeleton, since the fields are the substance of an entity
and should not be invented by a generator

想先看它跑起来再读文档,去 [demo/](./demo/):一条命令新建 3 个文件、往 6 个已存在的
文件里插 9 处条目,**生成出来的代码能编译**

## 🧪 从零到一：契约与服务都用模板生成 | From scratch with both templates

下面这套流程实跑验证过,每一步的输出都核对过,最后 `go build ./...` 退出码 0

前置:`kopy`、`protoc` 与 `protoc-gen-go` / `protoc-gen-go-grpc` /
`protoc-grpc-gateway` / `protoc-go-inject-tag` 都在 `PATH` 里
只想先看生成器本身跑起来、不想装这些,去 [demo/](./demo/) —— 它自带契约,零前置条件

```bash
# 1. 渲染契约仓库 | render the contract hub
kopy new ./kronos-share-proto-template -o ./demo-share-proto -y \
  --var OrgName=kamalyes --var OrgPrefix=demo \
  --var ProjectName=demo-share-proto \
  --var ModuleName=github.com/kamalyes/demo-share-proto

# 2. 生成 pb | generate the pb files
cd demo-share-proto && bash scripts/generate-modular.sh && go build ./... && cd ..

# 3. 渲染服务仓库 | render the service
#    ShareProtoModule 必须指向你自己的契约模块，见下面那条坑
kopy new ./kronos-service-template -o ./demo-catalog-service -y \
  --var OrgName=kamalyes --var ObjectPrefix=demo --var ServiceType=catalog \
  --var ServiceName=Product --var ModuleName=catalog \
  --var ShareProtoModule=github.com/kamalyes/demo-share-proto

# 4. 契约还没发布时指到本地 | point at the local contract until it is published
cd demo-catalog-service
go mod edit -replace github.com/kamalyes/demo-share-proto=../demo-share-proto
go mod tidy && go build ./...

# 5. 往契约加一个消息，重新生成 pb | add a message to the contract and regenerate
#    在 demo-share-proto 的 proto/common/*.proto 里加 message，再跑一次第 2 步

# 6. 生成实体并接线 | generate the entity and wire it in
kronos-codegen --root . new entity Supplier \
  --pb ../demo-share-proto/pb/common/inventory.pb.go --yes

# 7. 编译 | build
go build ./...
```

### 两条实跑踩到的坑 | Two things measured the hard way

**第 3 步的 `ShareProtoModule` 不能省** `-y` 会让它取默认值,而默认值刻意指向
`kronos-share-proto-template` 的 **`build` 分支** —— 那个模板的 CI 会把自己按默认
变量渲染一遍、跑完 protoc、验证能编译,再强推到 `build` 分支,于是同一个仓库既是
模板又是可消费的依赖 这个默认值本身是好的:**不需要自己的契约仓库时,第 1、2 步
可以整个跳过** 但一旦你渲染了自己的契约,就必须用 `--var` 把它指过去,否则服务
导入的是模板仓库的 `pb/`,而那个仓库的 master 里 `pb/` 只有一个 `.gitkeep`

**契约里消息与服务常常不在一个包** 实测:`SupplierInfo` 在 `pb/common`,
`WarehouseService` 在 `pb/inventory` 所以 `--pb` 指的是**消息所在**的文件,而生成的
服务要嵌的未实现桩来自**服务所在**的包 —— 生成器会搜整棵 `pb` 树把后者找出来,
两个包分别以 `{{.MsgAlias}}` 与 `{{.SvcAlias}}` 进模板,不必你指

### 网关不是 `new entity` 的目标 | A gateway is not a target for new entity

`kronos-gateway-template` 里没有 `models/table_names.go`、`repository/factory.go`、
`migrations/`、`bootstrap/pbmo.go` 中的任何一个 —— 网关路由请求,不拥有实体
线上的那两个网关也是同样的形态

[EN] A gateway owns no entities: the gateway template has none of the anchor files, and the
production gateways share that shape

## 🖥️ 命令与全局 flag | Commands and global flags

| 命令 \| Command | 作用 \| What it does |
| --- | --- |
| `new entity <Name>` | 展开清单的 `sections.entity`：先按模板生成该实体的新文件，再把接线条目幂等插入已存在的文件 |
| `check [--entity <Name>]` | 给了 `--entity` 就逐条报告该实体的条目是否已落地；不给则只校验清单本身与目标文件是否存在 |
| `version` | 打印版本号 |
| `help` | 打印用法 |

`<Name>` 必须是**以大写字母开头的合法 Go 标识符**，否则退出码 2

| 全局 flag \| Global flag | 默认 \| Default | 说明 \| Description |
| --- | --- | --- |
| `--root <dir>` | 当前目录 | 项目根目录，清单与所有相对路径都按它解析 |
| `--dry-run` | 关 | 只输出变更摘要与 diff，不写盘，也不询问 |
| `--yes` | 关 | 零交互，跳过写盘前的 `[y/N]` 确认 |
| `--plan <file>` | `<root>/.kronos-codegen.json` | 补丁计划清单路径 |
| `--format <policy>` | `if-clean` | gofmt 归一化策略：`if-clean` / `never` / `always` |
| `--var key=value` | 无 | 附加模板变量，可重复，优先级高于清单的 `vars` 与命名派生 |

全局 flag 既可以写在子命令**前面**，也可以写在位置参数**后面**，下面两条等价：

```bash
kronos-codegen --root ./svc --dry-run new entity Order
kronos-codegen new entity Order --root ./svc --dry-run
```

### 格式化策略 | Format policy

| 取值 \| Value | 行为 \| Behaviour |
| --- | --- |
| `if-clean`(默认) | 原文件本身已经是 gofmt 干净的才在拼接后跑一遍 `format.Source`；因为 gofmt 对已干净的文件是幂等的，这一遍只会修正被插入破坏的那一处列对齐，别处不产生噪音 |
| `never` | 永不归一化，除插入点外逐字节不变；代价是插入更长的成员名后块内列对齐会不齐，`gofmt -l` 会报 |
| `always` | 总是归一化；目标文件不是合法 Go 源码时报 `patch: gofmt normalization failed` |

### 退出码 | Exit codes

| 码 \| Code | 含义 \| Meaning |
| --- | --- |
| `0` | 成功，或所有条目都已存在无需改动，或用户主动取消 |
| `1` | 执行失败：锚点找不到、import 别名冲突、清单解析失败、写盘失败 |
| `2` | 用法错误：参数缺失、实体名不合法、根目录不存在、`--format` 取值未知 |
| `3` | `check` 发现漂移：有条目还没落地 |

## 📋 补丁计划清单 | The patch manifest

「往哪个文件的哪个锚点插什么」写在补丁计划清单里 清单按这个顺序取:

| 优先级 | 来源 | 说明 |
| --- | --- | --- |
| 1 | `--plan <file>` | 显式指定 指定了却找不到文件是用法错误,直接报错而不是悄悄换成内置的 |
| 2 | `<root>/.kronos-codegen.json` | 项目自己的清单,**整体覆盖**内置的那份 |
| 3 | 内置清单 | 覆盖 kronos 形态的服务,**零配置可用** |

[EN] The manifest is resolved in that order: an explicit `--plan`, then the project's own
`.kronos-codegen.json` which overrides the built-in wholesale, then the built-in manifest

### 内置清单凭什么成立 | Why a built-in manifest works

因为这套服务的锚点全是**约定名**而不是项目特有的名字 逐个核对过线上服务与
`kronos-service-template`:仓库工厂一律叫 `RepositoryFactory` / `NewRepositoryFactory`,
迁移清单一律叫 `GetAllModels`,`models/table_names.go` 一律只有一个 `var` 块
(所以那一条可以免锚点,见 [docs/MANIFEST.md](./docs/MANIFEST.md#免锚点--anchorless))

清单里刻意不含任何具体业务列名 早先的模型模板写死过 `tenant_id`,那是把某个产品的
租户模型焊进了一个通用工具 —— 别的项目用 `org_id`、或者没有租户维度

[EN] It works because this service shape anchors entirely on convention names rather than
project-specific ones, verified one by one against the production services and the template
The manifest deliberately contains no business column name, since an earlier model template
hardcoded `tenant_id` and that welded one product's tenancy model into a general tool

### 形态不一样怎么办 | When your project differs

```bash
kronos-codegen init           # 把内置清单与模板取进项目，改成本项目的形态
kronos-codegen init --dry-run # 先看会写哪些文件
```

`init` 会把清单落在 `.kronos-codegen.json`、模板落在 `.kronos-codegen/templates/`,
并把清单里的 `builtin:` 模板引用改写成取出来的那份路径 —— 否则改了模板不生效

[EN] `init` copies the manifest and templates into the project and rewrites the `builtin:`
references to the copied paths, since a copied manifest still pointing into the binary would
ignore edits to the copied templates

**九种 `kind` 的完整字段、四种锚点、免锚点规则、三个闸门字段(`optional` / `when` /
`group`)与模板变量对照表见 [docs/MANIFEST.md](./docs/MANIFEST.md)**

**想直接看它跑起来:[demo/](./demo/) 是一个自带契约、能生成也能编译的最小例子**

## 🔍 dry-run 与 check | Preview and drift check

`--dry-run` 先给逐条摘要(`add` / `skip` + 文件：行号 + 插入片段)，再给标准 unified diff：

```text
变更摘要:
  add   constants/cache_keys.go:26  const CacheKeyOrder
          | // CacheKeyOrder Order 缓存键前缀
          | CacheKeyOrder = CacheKeyPrefix + "order:"
  skip  locales/zh.json:22  json 键 error.order_not_found  (已存在)
  共 1 处新增，1 处已存在跳过，涉及 1 个文件

变更预览:
--- a/constants/cache_keys.go
+++ b/constants/cache_keys.go
@@ -23,6 +23,8 @@
 
 	// CacheKeyNotifySendLock 发送通知幂等锁
 	CacheKeyNotifySendLock = CacheKeyPrefix + "send_lock:"
+	// CacheKeyOrder Order 缓存键前缀
+	CacheKeyOrder = CacheKeyPrefix + "order:"
 )
```

`check` 复用同一套幂等判定，不写任何文件：

```bash
# 校验清单本身与目标文件是否存在 | validate the manifest and its targets
kronos-codegen check --root ./my-service

# 逐条校验某个实体是否已落地，漂移时退出码 3 | per-entry drift check
kronos-codegen check --entity Order --root ./my-service
```

```text
  ok    constants/cache_keys.go:27  const CacheKeyOrder
  miss  service/exports.go:18  type OrderServiceImpl

实体 Order: 1 条已落地，1 条缺失
```

## 🔌 当库使用 | Using it as a library

`patch` 和 `plan` 是普通的公开包，补丁引擎本身不与任何具体项目耦合：

```go
import "github.com/kamalyes/kronos-codegen/patch"

eng := patch.NewEngine(patch.NewDirStore("/path/to/project"))
eng.SetFormatPolicy(patch.FormatIfClean)

if _, err := eng.Run([]patch.Action{
	&patch.FileCreate{
		// 已存在就跳过，绝不覆盖；Go 文件一律 gofmt，不合法就报错不写盘
		// [EN] skipped if present, never overwritten; Go files are always gofmt'ed
		Path:    "models/order_model.go",
		Content: rendered,
	},
	&patch.DeclInsert{
		Path:  "constants/table.go",
		Decl:  patch.DeclConst,
		Block: patch.BlockRef{Member: "TableNameExample"},
		Name:  "TableNameOrder",
		Value: `"orders"`,
	},
	&patch.JSONKeyInsert{
		Path:   "locales/en.json",
		Parent: []string{"error"},
		Key:    "order_not_found",
		Value:  `"Order not found"`,
	},
}); err != nil {
	// 任何一条失败都整批中止，磁盘上一个字节都没动
	// [EN] any failure aborts the whole batch; not one byte is written
	return err
}

fmt.Print(eng.Summary())
fmt.Print(eng.Diff())
written, err := eng.Write()
```

错误都可以用 `errors.Is` 归类：`patch.ErrInvalidPatch`(补丁定义不合法)、`patch.ErrNotFound`(锚点找不到)、`patch.ErrParse`(目标文件不是合法 Go / JSON)、`patch.ErrConflict`(import 别名冲突)、`patch.ErrFormat`(归一化失败)，定位失败会带上 `*patch.LocateError`，里面有文件、锚点与具体原因

测试里可以用 `patch.NewMemStore` 把整个引擎跑在内存里，完全不碰磁盘

## 🧪 开发 | Development

```bash
go build ./...
go vet ./...
gofmt -l .
go test ./... -count=1
```

## 📄 许可证 | License

MIT，见 [LICENSE](./LICENSE)
