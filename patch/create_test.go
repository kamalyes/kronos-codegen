/*
 * @Author: kamalyes 501893067@qq.com
 * @Date: 2026-11-01 18:05:12
 * @LastEditors: kamalyes 501893067@qq.com
 * @LastEditTime: 2026-11-01 20:12:06
 * @FilePath: \kronos-codegen\patch\create_test.go
 * @Description: 新建文件层测试 - 格式化、不覆盖、同事务中止、预览与 MkdirAll
 *
 * Copyright (c) 2026 by kamalyes, All Rights Reserved.
 */
package patch

import (
	"errors"
	"go/format"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// unformattedGo 排版歪掉但语法合法的 Go 源码，模板渲染出来的东西常是这样
// [EN] unformattedGo is syntactically valid but badly laid out, as rendered templates often are
const unformattedGo = `/*
 * @Description: 新建文件 fixture
 *
 * Copyright (c) 2026 by kamalyes, All Rights Reserved.
 */
package models

import "time"

// OrderModel 订单模型
type OrderModel struct {
Id string
CreatedAt   time.Time
}

// TableName 指定表名
func (OrderModel) TableName() string { return "orders" }
`

// TestFileCreateFormats 新建的 Go 文件一律格式化，不走 FormatPolicy 的条件判断
// [EN] TestFileCreateFormats asserts new Go files are always gofmt'ed, unconditionally
func TestFileCreateFormats(t *testing.T) {
	store := NewMemStore(nil)
	eng := NewEngine(store)
	// 故意设成 never：新建文件不受 FormatPolicy 影响
	// [EN] deliberately set to never: creation is not governed by FormatPolicy
	eng.SetFormatPolicy(FormatNever)

	c := &FileCreate{Path: "models/order_model.go", Content: []byte(unformattedGo), Origin: "model.go.tmpl"}
	pl, err := eng.Step(c)
	if err != nil {
		t.Fatalf("新建失败: %v", err)
	}
	if !pl.Created || pl.Present {
		t.Fatalf("计划应当是新建且未跳过: %+v", pl)
	}
	if _, err := eng.Write(); err != nil {
		t.Fatalf("写入失败: %v", err)
	}
	got := string(store.Files["models/order_model.go"])
	if formatted, err := format.Source([]byte(got)); err != nil || string(formatted) != got {
		t.Errorf("新建的 Go 文件必须是 gofmt 干净的\n%s", got)
	}
	if !strings.Contains(got, "\tId        string\n") {
		t.Errorf("字段没有被对齐\n%s", got)
	}
	if !strings.HasPrefix(got, "/*\n * @Description: 新建文件 fixture") {
		t.Errorf("文件头注释块应当保留\n%s", got)
	}
	if pl.Lines != countLines([]byte(got)) {
		t.Errorf("行数不符: got %d want %d", pl.Lines, countLines([]byte(got)))
	}
}

// TestFileCreateNeverOverwrites 目标文件已存在就跳过，绝不覆盖
// [EN] TestFileCreateNeverOverwrites skips an existing target and never clobbers it
func TestFileCreateNeverOverwrites(t *testing.T) {
	const existing = "package models\n\n// 我是手写的，不许被覆盖\n"
	store := NewMemStore(map[string]string{"models/order_model.go": existing})
	eng := NewEngine(store)

	pl, err := eng.Step(&FileCreate{Path: "models/order_model.go", Content: []byte(unformattedGo)})
	if err != nil {
		t.Fatalf("不应报错: %v", err)
	}
	if !pl.Present || !pl.NoOp() {
		t.Errorf("已存在的文件应当判为 Present 且是 no-op: %+v", pl)
	}
	written, err := eng.Write()
	if err != nil {
		t.Fatalf("写入失败: %v", err)
	}
	if len(written) != 0 {
		t.Errorf("不应写任何文件，实际 %v", written)
	}
	if string(store.Files["models/order_model.go"]) != existing {
		t.Errorf("已存在的文件被改动了\n%s", store.Files["models/order_model.go"])
	}
}

// TestFileCreateIdempotent 同一步跑两次，第二次是 no-op
// [EN] TestFileCreateIdempotent asserts a second run is a no-op
func TestFileCreateIdempotent(t *testing.T) {
	store := NewMemStore(nil)
	c := &FileCreate{Path: "models/order_model.go", Content: []byte(unformattedGo)}

	eng1 := NewEngine(store)
	if _, err := eng1.Step(c); err != nil {
		t.Fatalf("第一轮失败: %v", err)
	}
	if _, err := eng1.Write(); err != nil {
		t.Fatalf("写入失败: %v", err)
	}
	first := string(store.Files["models/order_model.go"])

	eng2 := NewEngine(store)
	pl, err := eng2.Step(c)
	if err != nil {
		t.Fatalf("第二轮失败: %v", err)
	}
	if !pl.Present {
		t.Errorf("第二轮应当判为已存在")
	}
	w2, err := eng2.Write()
	if err != nil {
		t.Fatalf("第二轮写入失败: %v", err)
	}
	if len(w2) != 0 || string(store.Files["models/order_model.go"]) != first {
		t.Errorf("第二轮产生了改动: %v", w2)
	}
}

// TestFileCreateRejectsInvalidGo 渲染结果不是合法 Go 就报错，宁可不写
// [EN] TestFileCreateRejectsInvalidGo refuses to write an invalid .go file
func TestFileCreateRejectsInvalidGo(t *testing.T) {
	store := NewMemStore(nil)
	eng := NewEngine(store)
	_, err := eng.Step(&FileCreate{
		Path:    "models/order_model.go",
		Content: []byte("package models\n\nfunc broken( {\n"),
	})
	if !errors.Is(err, ErrFormat) {
		t.Fatalf("预期 ErrFormat，实际 %v", err)
	}
	if !strings.Contains(err.Error(), "已放弃写入") {
		t.Errorf("错误信息应当说明放弃写入: %v", err)
	}
	if len(store.Files) != 0 {
		t.Errorf("不合法的内容不应落盘: %v", store.Files)
	}
}

// TestFileCreateNonGoNotFormatted 非 Go 文件不走 gofmt，内容原样落盘
// [EN] TestFileCreateNonGoNotFormatted leaves non-Go content untouched
func TestFileCreateNonGoNotFormatted(t *testing.T) {
	store := NewMemStore(nil)
	eng := NewEngine(store)
	const raw = "{\n    \"a\": 1\n}\n"
	if _, err := eng.Step(&FileCreate{Path: "resources/x.json", Content: []byte(raw)}); err != nil {
		t.Fatalf("新建失败: %v", err)
	}
	if _, err := eng.Write(); err != nil {
		t.Fatalf("写入失败: %v", err)
	}
	if string(store.Files["resources/x.json"]) != raw {
		t.Errorf("非 Go 文件内容被动了\n%q", store.Files["resources/x.json"])
	}
}

// TestFileCreateEmptyContent 空内容报错，不留空文件
// [EN] TestFileCreateEmptyContent refuses empty content
func TestFileCreateEmptyContent(t *testing.T) {
	eng := NewEngine(NewMemStore(nil))
	_, err := eng.Step(&FileCreate{Path: "models/x.go", Content: []byte("  \n")})
	if !errors.Is(err, ErrInvalidPatch) {
		t.Fatalf("预期 ErrInvalidPatch，实际 %v", err)
	}
}

// TestRunCreateThenPatchSameTransaction 新建与插入在同一个事务里，任一步失败整体中止
// [EN] TestRunCreateThenPatchSameTransaction keeps creation and insertion in one transaction
func TestRunCreateThenPatchSameTransaction(t *testing.T) {
	store := NewMemStore(map[string]string{"constants/table.go": constFixture})
	eng := NewEngine(store)
	eng.SetFormatPolicy(FormatNever)

	actions := []Action{
		&FileCreate{Path: "models/order_model.go", Content: []byte(unformattedGo)},
		&DeclInsert{Path: "constants/table.go", Block: BlockRef{Member: "TableNameSecond"},
			Name: "TableNameOrder", Value: `"orders"`},
		// 这一步注定失败：锚点不存在
		// [EN] this step is bound to fail: the anchor does not exist
		&DeclInsert{Path: "constants/table.go", Block: BlockRef{Member: "NoSuchMember"},
			Name: "TableNameThird", Value: `"thirds"`},
	}
	if _, err := eng.Run(actions); !errors.Is(err, ErrNotFound) {
		t.Fatalf("预期 ErrNotFound，实际 %v", err)
	}
	// 中止时一个字节都不该落盘，新文件也不该出现
	// [EN] an abort writes nothing, not even the new file
	if _, ok := store.Files["models/order_model.go"]; ok {
		t.Errorf("中止后新文件不应存在")
	}
	if string(store.Files["constants/table.go"]) != constFixture {
		t.Errorf("中止后已有文件不应被改动")
	}
}

// TestRunCreateBeforePatch 成功路径：新建与插入都落盘
// [EN] TestRunCreateBeforePatch is the happy path where both land
func TestRunCreateBeforePatch(t *testing.T) {
	store := NewMemStore(map[string]string{"constants/table.go": constFixture})
	eng := NewEngine(store)
	eng.SetFormatPolicy(FormatNever)

	plans, err := eng.Run([]Action{
		&FileCreate{Path: "models/order_model.go", Content: []byte(unformattedGo), Origin: "model.go.tmpl"},
		&DeclInsert{Path: "constants/table.go", Block: BlockRef{Member: "TableNameSecond"},
			Name: "TableNameOrder", Value: `"orders"`},
	})
	if err != nil {
		t.Fatalf("执行失败: %v", err)
	}
	if len(plans) != 2 || !plans[0].Created || plans[1].Created {
		t.Fatalf("计划类型不符: %+v", plans)
	}
	written, err := eng.Write()
	if err != nil {
		t.Fatalf("写入失败: %v", err)
	}
	if strings.Join(written, ",") != "constants/table.go,models/order_model.go" {
		t.Errorf("写入文件列表不符: %v", written)
	}

	// 新建文件的预览只给前若干行，不铺全文
	// [EN] the preview shows only the first lines, not the whole body
	diff := eng.Diff()
	if !strings.Contains(diff, "--- /dev/null") ||
		!strings.Contains(diff, "+++ b/models/order_model.go") ||
		!strings.Contains(diff, "新建文件，预览前 8 行") {
		t.Errorf("新建文件的预览格式不符\n%s", diff)
	}
	if !strings.Contains(diff, "……（省略") {
		t.Errorf("预览应当标注省略了多少行\n%s", diff)
	}
	if strings.Contains(diff, "func (OrderModel) TableName()") {
		t.Errorf("预览不应铺出整个新文件\n%s", diff)
	}
	summary := eng.Summary()
	for _, want := range []string{
		"create models/order_model.go  (",
		"add    constants/table.go:15  const TableNameOrder",
		"共 1 个新建文件，1 处新增，0 处已存在跳过，涉及 2 个文件",
	} {
		if !strings.Contains(summary, want) {
			t.Errorf("摘要里缺少 %q\n%s", want, summary)
		}
	}
}

// TestDirStoreCreatesNestedDir 新建文件要能落到还不存在的子目录，并且挡住 ../ 越界
// [EN] TestDirStoreCreatesNestedDir creates missing parent directories and still guards ../
func TestDirStoreCreatesNestedDir(t *testing.T) {
	root := t.TempDir()
	eng := NewEngine(NewDirStore(root))
	if _, err := eng.Step(&FileCreate{
		Path:    "service/inventory/warehouse_service.go",
		Content: []byte("package inventory\n\n// Warehouse 服务\ntype WarehouseService struct{}\n"),
	}); err != nil {
		t.Fatalf("新建失败: %v", err)
	}
	if _, err := eng.Write(); err != nil {
		t.Fatalf("写入失败: %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, "service", "inventory", "warehouse_service.go")); err != nil {
		t.Errorf("嵌套目录没有被创建: %v", err)
	}

	esc := NewEngine(NewDirStore(root))
	if _, err := esc.Step(&FileCreate{Path: "../escaped.go", Content: []byte("package x\n")}); err == nil {
		if _, err := esc.Write(); !errors.Is(err, ErrInvalidPatch) && err == nil {
			t.Errorf("越出根目录的路径应当被拒绝")
		}
	}
	if _, err := os.Stat(filepath.Join(filepath.Dir(root), "escaped.go")); err == nil {
		t.Errorf("越出根目录的文件被写出去了")
	}
}

// TestSkippedCreateShowsInSummary 已存在而被跳过的新建步骤也要出现在摘要里
// 不然摘要的条目数与实际步骤数对不上，用户会以为漏跑了
// [EN] TestSkippedCreateShowsInSummary asserts a skipped creation is still reported, or the
// summary's item count would not add up to the number of steps
func TestSkippedCreateShowsInSummary(t *testing.T) {
	store := NewMemStore(map[string]string{
		"models/order_model.go": "package models\n\n// 手写的\ntype OrderModel struct{}\n",
		"constants/table.go":    constFixture,
	})
	eng := NewEngine(store)
	eng.SetFormatPolicy(FormatNever)

	plans, err := eng.Run([]Action{
		&FileCreate{Path: "models/order_model.go", Content: []byte(unformattedGo)},
		&DeclInsert{Path: "constants/table.go", Block: BlockRef{Member: "TableNameSecond"},
			Name: "TableNameOrder", Value: `"orders"`},
	})
	if err != nil {
		t.Fatalf("执行失败: %v", err)
	}
	if !plans[0].Present {
		t.Fatalf("已存在的文件应判为 Present")
	}
	summary := eng.Summary()
	if !strings.Contains(summary, "skip   models/order_model.go") ||
		!strings.Contains(summary, "(文件已存在)") {
		t.Errorf("摘要里应当报出被跳过的新建步骤\n%s", summary)
	}
	if !strings.Contains(summary, "共 0 个新建文件，1 处新增，1 处已存在跳过，涉及 1 个文件") {
		t.Errorf("摘要统计不符\n%s", summary)
	}
	// 跳过创建之后往同一个文件插条目，走的应当是同一份缓存内容
	// [EN] patching the same file after a skipped creation works off the same cached content
	if _, err := eng.Apply(&StructFieldInsert{Path: "models/order_model.go",
		Struct: "OrderModel", Name: "Id", Type: "string"}); err != nil {
		t.Errorf("跳过创建后往同一文件插入失败: %v", err)
	}
	if got := string(eng.Changes()[0].After); !strings.Contains(got, "Id string") {
		t.Errorf("插入没有生效\n%s", got)
	}
}
