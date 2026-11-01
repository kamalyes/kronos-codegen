# 补丁计划清单 | The patch manifest

> `kronos-codegen` 的清单参考手册 —— 每种 `kind` 的字段、锚点语义、模板变量,逐条对着实现写
>
> [EN] The manifest reference for `kronos-codegen`: fields per `kind`, anchor semantics and template variables, written against the implementation
>
> 返回 [README](../README.md)

「往哪个文件的哪个锚点插什么」全部外置在项目根目录的 `.kronos-codegen.json` 里，本仓库**不内置任何业务清单**——每个工程的文件形态不一样，清单跟着工程走

[EN] The "insert what, where" decision lives entirely in `.kronos-codegen.json` at the project root. This repository ships no business manifest on purpose: file shapes differ per project, so the manifest belongs to the project.

## 顶层结构 | Top-level shape

```json
{
  "version": 1,
  "vars": { "Module": "github.com/acme/my-service" },
  "sections": {
    "entity": [ { "kind": "...", "...": "..." } ]
  }
}
```

| 字段 \| Field | 必填 \| Required | 说明 \| Description |
| --- | --- | --- | 
| `version` | 是 | 目前只支持 `1`，其他值直接报错 |
| `vars` | 否 | 清单级默认变量，会被命令行的 `--var` 覆盖 |
| `sections` | 是 | 按命令分组的步骤列表；`new entity` 用的是 `sections.entity` |

清单解析用了 `DisallowUnknownFields`:**写错字段名会直接报错**，不会被悄悄忽略

## 模板变量 | Template variables

每个步骤里的**所有字符串字段**都会先过一遍 `text/template`，引用不存在的变量会直接报错而不是展开成空串，实体名派生出的变量只做纯粹的大小写与分隔符转换，不含任何业务约定(不规则复数之类请用 `--var` 显式传)：

| 变量 \| Variable | `Order` | `OrderItem` | `HTTPServer` |
| --- | --- | --- | --- |
| `{{.Name}}` | `Order` | `OrderItem` | `HTTPServer` |
| `{{.Lower}}` | `order` | `orderItem` | `httpServer` |
| `{{.Snake}}` | `order` | `order_item` | `http_server` |
| `{{.Kebab}}` | `order` | `order-item` | `http-server` |
| `{{.Upper}}` | `ORDER` | `ORDER_ITEM` | `HTTP_SERVER` |
| `{{.Flat}}` | `order` | `orderitem` | `httpserver` |
| `{{.Delimited}}` | `order` | `order.item` | `http.server` |

## 锚点字段 | Anchor fields

| 字段 \| Field | 适用 \| Applies to | 含义 \| Meaning |
| --- | --- | --- |
| `member` | `const` / `var` / `type` | 目标块内**已经存在**的任意成员名，命中声明它的那个括号块 |
| `marker` | `const` / `var` / `type` / `slice_append` / `stmt` | 标记注释里的一个片段；`stmt` 下命中注释所在的**最内层代码块**，因此能钻进 `RegisterService(func(srv){ ... })` 这类闭包 |
| `near` | `import` / `slice_append` / `stmt` | 已经存在的**兄弟条目**:`import` 下是一个导入路径(新条目落到它所在的空行分组),`slice_append` 下是一个元素表达式，`stmt` 下是一条语句源码 |
| `after` | `stmt` | 布尔，配合 `near` 使用：为 `true` 时紧跟那条语句之后插入，而不是追加到块尾；一个函数体按业务分段（每段前一条横幅注释）时必须用它，否则新语句会落到最后一段 |
| `func` + `recv` | `composite_field` / `slice_append` / `stmt` | 函数名，`recv` 为方法接收者类型名(指针接收者写裸类型名) |
| `var` | `slice_append` | 包级变量名，其值是切片字面量 |

`slice_append` 的锚点优先级是 `marker` > `near` > `var` > `func`,`stmt` 的是 `marker` > `near` > `func`

## 每种 kind 的字段 | Fields per kind

### `create_file` — 新建一个文件

```json
{
  "kind": "create_file",
  "file": "models/{{.Snake}}_model.go",
  "template": ".kronos-codegen/templates/model.go.tmpl"
}
```

| 字段 | 必填 | 说明 |
| --- | --- | --- |
| `file` | 是 | 要新建的文件相对路径，父目录不存在会自动创建 |
| `template` | 与 `content` 二选一 | 项目内模板文件的相对路径，内容用 `text/template` 和同一套变量渲染 |
| `content` | 与 `template` 二选一 | 直接内联新文件的内容，同样会过一遍模板展开 |

模板放在项目里而不是内置在 codegen 里，取向与清单外置一致：每个工程的文件形态不一样，模板跟着工程走，约定俗成的位置是 `.kronos-codegen/templates/`，但路径由清单自己决定，只要不越出项目根目录

三条硬规则：

1. **目标文件已存在就跳过，绝不覆盖**，与 JSON 不覆盖已有译文同一取向；想重新生成请先自己删掉旧文件
2. **新建的 Go 文件一律 gofmt**，不受 `--format` 影响——新文件没有需要保护的历史排版；渲染结果不是合法 Go 源码时直接报错并整批中止，**宁可不写也不留一个坏 .go**
3. **`create_file` 步骤总是先执行**，与它在清单里的书写位置无关（同类之间保持书写顺序），所以 dry-run 读起来是「先建了哪些文件，再往哪些文件插了什么」

模板引用了不存在的变量、模板文件读不到、模板语法有错，都会在任何写入之前报错

### `const` / `var` — 往括号声明块插一个成员

```json
{
  "kind": "const",
  "file": "constants/cache_keys.go",
  "member": "CacheKeyPrefix",
  "name": "CacheKey{{.Name}}",
  "value": "CacheKeyPrefix + \"{{.Snake}}:\"",
  "type": "",
  "doc": "CacheKey{{.Name}} {{.Name}} 缓存键前缀"
}
```

| 字段 | 必填 | 说明 |
| --- | --- | --- |
| `file` | 是 | 目标文件相对路径(斜杠分隔) |
| `member` / `marker` | 二选一 | 定位括号块的锚点 |
| `name` | 是 | 成员名，必须是合法 Go 标识符 |
| `value` | 是 | 值表达式源码，解析失败直接报错 |
| `type` | 否 | 显式类型标注，拼成 `name type = value` |
| `doc` | 否 | 插入内容上方的注释，多行用 `\n` 分隔，自动补 `//` |

### `type` — 往 `type ( ... )` 块插别名或类型定义

```json
{
  "kind": "type",
  "file": "service/exports.go",
  "member": "NotifyServiceImpl",
  "name": "{{.Name}}ServiceImpl",
  "value": "{{.Flat}}svc.{{.Name}}ServiceImpl"
}
```

`value` 与 `type` **恰好给一个**：给 `value` 生成别名 `name = value`，给 `type` 生成类型定义 `name type`，两个都给或都不给都会报错，锚点与幂等判定与 `const` / `var` 完全一致

### `import` — 往 import 块插一条导入

```json
{
  "kind": "import",
  "file": "service/exports.go",
  "import": "github.com/acme/my-service/service/{{.Snake}}",
  "alias": "{{.Flat}}svc",
  "near": "github.com/acme/my-service/service/notify"
}
```

| 字段 | 必填 | 说明 |
| --- | --- | --- |
| `file` | 是 | 目标文件 |
| `import` | 是 | 导入路径 |
| `alias` | 否 | 导入别名，支持 `_` 与 `.` |
| `near` | 否 | 同组里已有的一个导入路径，新条目落到那一组的末尾；留空则追加到 import 块的最后一组 |
| `doc` | 否 | 上方注释 |

Go 的 import 块习惯按标准库 / 三方 / 内部用空行分组，**gofmt 会在组内重排但从不合并分组**，所以 `near` 只需要把新条目放进正确的那一组，排序交给 `--format if-clean` 的归一化，幂等按导入路径判定，别名不一致会报冲突(见必读行为第 2 条)

### `struct_field` — 往 struct 末尾加字段

```json
{
  "kind": "struct_field",
  "file": "repository/factory.go",
  "struct": "RepositoryFactory",
  "name": "{{.Name}}",
  "type": "*{{.Name}}Repository",
  "tag": "json:\"{{.Snake}}\"",
  "doc": ""
}
```

`struct` / `name` / `type` 必填，`tag` 不含反引号(会自动补上)，空块里只剩一行 `// TODO` 注释时，新字段会沿用那一行的缩进

### `composite_field` — 往复合字面量加 `key: value`

```json
{
  "kind": "composite_field",
  "file": "repository/factory.go",
  "func": "NewRepositoryFactory",
  "type": "RepositoryFactory",
  "key": "{{.Name}}",
  "value": "New{{.Name}}Repository()"
}
```

`func` / `key` / `value` 必填，`type` 用于同一函数里有多个字面量时消歧(按类型文本匹配，`&T{}` 写 `T`)，留空取函数体里第一个复合字面量，与 `struct_field` 成对使用，正好覆盖「工厂加字段 + 构造函数加初始化项」

### `slice_append` — 往切片字面量末尾加元素

```json
{
  "kind": "slice_append",
  "file": "migrations/auto_migrate.go",
  "func": "GetAllModels",
  "near": "&models.ExampleModel{}",
  "element": "&models.{{.Name}}Model{}"
}
```

`element` 必填且必须是合法表达式(支持多行)，锚点四选一，幂等判定会把已有元素和目标元素都用 `go/printer` 归一化后比较，所以**排版不同也认得出是同一条**，上一个元素缺尾逗号时会自动补

### `stmt` — 往函数体或闭包体插语句

```json
{
  "kind": "stmt",
  "file": "bootstrap/services.go",
  "near": "pb.RegisterExampleServiceServer(srv, s.exampleSvc)",
  "stmt": "pb.Register{{.Name}}ServiceServer(srv, s.{{.Lower}}Svc)"
}
```

`stmt` 必填且必须是合法语句(支持多行，缩进会按目标块重算)，锚点三选一

默认追加到目标块的**末尾**，给 `"after": true` 则紧跟 `near` 那条语句之后插入；一个函数体里按业务分段时(每段前面一条横幅注释)必须用 `after`，否则新语句会落到最后一段里

幂等判定走 `go/scanner` 的 token 流比较，所以与换行和空格完全无关：多行链式调用写成一行就能当 `near` 锚点

```json
{
  "kind": "stmt",
  "file": "bootstrap/kvcache.go",
  "after": true,
  "near": "cachex.NewModelKV[models.ProductModel](base, db).Fields(\"Name\").Register()",
  "stmt": "cachex.NewModelCache[models.{{.Name}}Model](base, db).\n\tKeyFields(\"TenantId\", \"Id\").\n\tRegister()"
}
```

上面这条 `near` 在源码里其实是三行的链式调用，写成一行照样命中

### `json_key` — 往 JSON 对象插一个键

```json
{
  "kind": "json_key",
  "files": ["locales/*.json"],
  "parent": ["error"],
  "key": "{{.Snake}}_not_found",
  "values": { "zh": "订单不存在", "zh-tw": "訂單不存在" },
  "fallback": "{{.Name}} not found"
}
```

| 字段 | 必填 | 说明 |
| --- | --- | --- |
| `file` / `files` | 二选一 | `files` 支持 glob，一次扇出到所有语言文件；glob 没匹配到文件会报错 |
| `key` | 是 | 要插入的键名 |
| `parent` | 否 | 父对象的键路径，如 `["error"]`；留空表示顶层对象 |
| `value` | 与下面二选一 | **原始 JSON 字面量**，会被 `json.Compact` 压成单行，所有文件用同一个值 |
| `values` + `fallback` | 与 `value` 二选一 | 按语言给译文，键是文件名去掉扩展名(`en.json` → `en`)，缺的语言退回 `fallback`；值会被编码成 JSON 字符串 |

三者全不给会报错(否则会静默插入空字符串)，键顺序与缩进风格原样保留，紧凑单行 JSON 保持紧凑，冒号与逗号后的空格按原文件风格镜像
