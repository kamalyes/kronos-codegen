# demo - 一个能跑能编译的最小例子

这个目录是 `kronos-codegen` 的可运行例子 它存在的理由很直接:这个工具的参考手册
(`../docs/MANIFEST.md`)写得很全,但在它之前整个仓库里没有一份跑得通的清单,
于是要用它就得照着手册从零编一份、还要自己判断锚点落在自己代码的哪一行

## 跑一遍

```bash
cd ..                                   # 回到 kronos-codegen 根目录
go build -o /tmp/kcg .

# 1. 先看会改什么，不写盘
/tmp/kcg --root demo new entity Order --pb demo/contract/order.pb.go --dry-run

# 2. 真写
/tmp/kcg --root demo new entity Order --pb demo/contract/order.pb.go --yes

# 3. 生成出来的代码是能编译的，这是唯一的证明
cd demo && go build ./...

# 4. 再跑一次：幂等，全部跳过
cd .. && /tmp/kcg --root demo new entity Order --pb demo/contract/order.pb.go --yes

# 5. 复位，便于重跑
demo/reset.sh
```

## 它做了什么

一条命令新建 3 个文件、往 6 个已存在的文件里插 9 处条目:

| 新建 | |
|---|---|
| `models/order_model.go` | 字段来自契约,不是固定骨架 |
| `repository/order_repository.go` | |
| `service/order/order_service.go` | 契约里有 `OrderService`,所以嵌了未实现桩 |

| 接线 | 靠什么定位 |
|---|---|
| `models/table_names.go` 的 var 块 | 全文件只有一个 var 块,免锚点 |
| `repository/factory.go` 的结构体字段 | 结构体名 `RepositoryFactory` |
| `repository/factory.go` 构造里的初始化项 | 函数名 `NewRepositoryFactory` |
| `migrations/auto_migrate.go` 的模型切片 | 函数名 `GetAllModels` |
| `service/exports.go` 的 import / type / var 三处别名 | `kronos:exports` 标记注释 |
| `locales/*.json` 的 error 段 | JSON 路径 |

## 字段是从契约来的,不是编的

`contract/order.pb.go` 模仿 protoc 的产出 生成器读它的结构体字段名、`protobuf` tag
与行尾注释,得到字段表:

```go
OrderID  string            `gorm:"column:order_id;type:varchar(36);primaryKey"`   // 订单ID | [EN] Order ID
TenantID string            `gorm:"column:tenant_id;...;index"`                    // 租户ID | [EN] Tenant ID
Status   enums.OrderStatus `gorm:"column:status;type:int;not null;default:0"`     // 状态 | [EN] Status
ItemIDs  []string          `gorm:"column:item_ids;serializer:json"`               // 条目ID列表
```

几处值得注意的:

- **双语注释原样带过来** 契约里的注释是字段语义的唯一来源,生成时丢掉就只剩名字
- **`repeated` 落 JSON 列** 而不是拆关联表:关联表要额外的表名、外键与迁移,
  生成器判断不出业务是否真的需要可查询的关联
- **枚举保留 protoc 给的类型**并带上它真实的包路径 导入是从字段类型反推的,
  漏一条就是 `undefined` —— 写这个 demo 时真漏过一次,编译当场抓到
- **`Ids` 归一成 `IDs`** 而不是 `IDS`:对后缀整体 `ToUpper` 会得到后者
- **表名用 `{{.Snake}}s`**,不含不规则复数 模型文件生成之后归项目所有,直接改

## 这个 demo 为什么自带清单和模板

`.kronos-codegen.json` 与 `.kronos-codegen/templates/` 是这个 demo 自己的

内置清单(`kronos-codegen init` 可以取出来看)的模板导入 `go-sqlbuilder` 与
`go-rpc-gateway`,在这个 demo 里解析不到那些依赖 所以 demo 用一份简化的模板,
代价是少演示了嵌 `BaseRepository` 那一层,换来的是 `go build` 真的能过 ——
一个编译不过的 demo 证明不了任何事

顺带它也演示了另一条路:**项目放一份自己的清单,就整体覆盖内置的那份**

## 契约里没有对应服务的情形

`contract/order_grpc.pb.go` 里有 `UnimplementedOrderServiceServer`,所以生成的服务
嵌了它 —— 服务从生成出来的那一刻就满足 gRPC 接口、可以直接注册,没写的方法返回
`Unimplemented` 而不是让编译失败

把那个文件删掉再生成,服务就不会嵌桩 这件事是探出来的而不是假设的:契约里没有
对应服务时嵌它编译不过
