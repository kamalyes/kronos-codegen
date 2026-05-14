# Kronos Codegen Prompts

Kronos 现在只做一件事：把你在 `.proto` 或 `.go` 文件里选中的代码，整理成一段可以直接交给 Codex 执行的实现提示词。

它不再内置数据库连接、不再提供 dashboard、不再用固定模板直接生成 CRUD 文件。真正的代码实现交给 Codex 在目标服务里读取真实上下文后完成，这样同一套入口可以复用到后续十几个或二十个 Apex 风格服务。

## 右键操作

选中一段 Proto、Go model、Go interface 或 service 方法后，右键使用：

- `Kronos: 生成新代码提示词`
- `Kronos: 结构体字段变更提示词`
- `Kronos: 实现接口提示词`
- `Kronos: 自定义 Codex 任务提示词`

插件会复制一段完整提示词到剪贴板，并尝试打开配置的 Codex/Chat 命令。

## 设计原则

- 选区是输入，不是最终真理。Codex 必须先在目标服务里 `rg` 搜索相近实体和现有实现。
- 如果选区来自 `apex-share-proto`，它只代表共享契约；实现代码必须落到具体服务里。
- 如果当前文件已经在某个 `*-service` Go module 里，插件默认把这个 module 当目标服务。
- 如果当前文件在 shared proto 或公共库里，并且工作区里有多个服务，插件会让你选择目标服务。
- 插件只总结上下文、目标、约束和建议搜索词，不再维护一套容易过期的生成模板。

## Codex 提示词包含什么

生成的提示词会包含：

- 目标服务根目录和 Go module。
- 当前选区所在文件、行号、语言类型。
- 从选区识别到的 message、service、rpc、struct、interface、字段。
- 工作区里可见的 Go module，例如具体服务、`apex-share-proto`、`go-pbmo`、`go-sqlbuilder`、`go-rpc-gateway`。
- Apex 风格约束：models、repository、service、errors、bootstrap、PBMO 注册、repository factory、go-sqlbuilder、go-pbmo、错误码、迁移和测试。

## 推荐使用方式

### 生成新代码

选中 `message XxxInfo`、一组 RPC、一个 Go model 或一个接口，右键 `Kronos: 生成新代码提示词`。

Codex 会被要求：

- 先找当前服务中相似实体的完整实现链路。
- 再按真实目录和命名生成缺失代码。
- 必要时补齐 model、repository、service、errors、bootstrap/pbmo、factory、migration、test。

### 结构体字段变更

选中新的 proto message 或 Go struct，右键 `Kronos: 结构体字段变更提示词`。

Codex 会被要求同步字段影响面：

- model 字段和 tag。
- proto request/response 或 PB 类型。
- PBMO 转换和注册。
- repository query/update。
- service 参数校验和响应组装。
- migration、测试和错误码。

### 实现接口

选中 proto rpc/service、Go interface 或方法签名，右键 `Kronos: 实现接口提示词`。

Codex 会被要求按目标服务已有 receiver、注册方式、错误处理、repository 调用和 PBMO 转换方式补齐接口实现。

## 配置

```jsonc
{
  // 生成提示词后尝试执行的 VS Code 命令。
  // 提示词一定会先复制到剪贴板，所以命令不存在也不影响使用。
  "kronos-codegen.codexCommand": "workbench.action.chat.open"
}
```

## 开发

```bash
yarn install
yarn compile
```

调试：

1. 在 VS Code 中打开 `kronos-codegen`。
2. 按 `F5` 启动 Extension Development Host。
3. 在 Apex 服务或 `apex-share-proto` 中选中 `.go` / `.proto` 代码测试右键菜单。

打包：

```bash
yarn package
```
