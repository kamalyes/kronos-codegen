package models

// 表名集中登记 全文件只有一个 var 块，所以清单里的 var 条目可以不给锚点
// [EN] Table names are registered here; the file holds a single var block, so the manifest's
// var step needs no anchor
var (
	TableNameSeed = SeedModel{}.TableName()
)
