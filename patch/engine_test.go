/*
 * @Author: kamalyes 501893067@qq.com
 * @Date: 2026-11-01 18:05:12
 * @LastEditors: kamalyes 501893067@qq.com
 * @LastEditTime: 2026-11-01 20:12:06
 * @FilePath: \kronos-codegen\patch\engine_test.go
 * @Description: 补丁引擎测试 - 多补丁同文件、dry-run 不写盘、摘要与 diff 输出
 *
 * Copyright (c) 2026 by kamalyes, All Rights Reserved.
 */
package patch

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestEngineMultiplePatchesSameFile struct 字段与构造函数初始化项是一对，必须能落在同一个文件上
// [EN] TestEngineMultiplePatchesSameFile applies the paired struct/constructor patches
func TestEngineMultiplePatchesSameFile(t *testing.T) {
	store := NewMemStore(map[string]string{"repository/factory.go": factoryFixture})
	eng := NewEngine(store)
	eng.SetFormatPolicy(FormatNever)

	patches := []Patch{
		&StructFieldInsert{Path: "repository/factory.go", Struct: "RepositoryFactory",
			Name: "Order", Type: "*OrderRepository"},
		&CompositeFieldInsert{Path: "repository/factory.go", Func: "NewRepositoryFactory",
			Type: "RepositoryFactory", Key: "Order", Value: "NewOrderRepository()"},
	}
	plans, err := eng.ApplyAll(patches)
	if err != nil {
		t.Fatalf("应用失败: %v", err)
	}
	if len(plans) != 2 {
		t.Fatalf("应当产生 2 条计划，实际 %d", len(plans))
	}
	for _, pl := range plans {
		if pl.NoOp() {
			t.Errorf("计划不应为 no-op: %+v", pl)
		}
	}

	changes := eng.Changes()
	if len(changes) != 1 {
		t.Fatalf("两条补丁在同一个文件上，应当只有 1 个 FileChange，实际 %d", len(changes))
	}
	after := string(changes[0].After)
	if !strings.Contains(after, "\tOrder *OrderRepository\n") {
		t.Errorf("struct 字段没插进去\n%s", after)
	}
	if !strings.Contains(after, "\t\t\tOrder: NewOrderRepository(),\n") {
		t.Errorf("初始化项没插进去\n%s", after)
	}
	assertCommentsKept(t, factoryFixture, after)

	// dry-run 语义：Apply 只在内存里累积，没调 Write 之前存储不变
	// [EN] dry-run semantics: Apply only buffers; the store stays untouched until Write
	if string(store.Files["repository/factory.go"]) != factoryFixture {
		t.Errorf("Apply 阶段不应写盘")
	}

	written, err := eng.Write()
	if err != nil {
		t.Fatalf("写入失败: %v", err)
	}
	if len(written) != 1 || written[0] != "repository/factory.go" {
		t.Errorf("写入文件列表不符: %v", written)
	}
	if string(store.Files["repository/factory.go"]) != after {
		t.Errorf("写盘内容与内存内容不一致")
	}

	// 幂等：同一批补丁再跑一遍，什么都不该变
	// [EN] idempotency: a second run of the same batch changes nothing
	eng2 := NewEngine(store)
	eng2.SetFormatPolicy(FormatNever)
	plans2, err := eng2.ApplyAll(patches)
	if err != nil {
		t.Fatalf("第二轮应用失败: %v", err)
	}
	for _, pl := range plans2 {
		if !pl.Present {
			t.Errorf("第二轮应全部判为已存在: %+v", pl)
		}
	}
	w2, err := eng2.Write()
	if err != nil {
		t.Fatalf("第二轮写入失败: %v", err)
	}
	if len(w2) != 0 {
		t.Errorf("第二轮不应写任何文件，实际 %v", w2)
	}
}

// TestEngineApplyAllAbortsWithoutWriting 中途失败时磁盘必须保持原样
// [EN] TestEngineApplyAllAbortsWithoutWriting asserts a mid-batch failure writes nothing
func TestEngineApplyAllAbortsWithoutWriting(t *testing.T) {
	store := NewMemStore(map[string]string{"repository/factory.go": factoryFixture})
	eng := NewEngine(store)
	eng.SetFormatPolicy(FormatNever)
	_, err := eng.ApplyAll([]Patch{
		&StructFieldInsert{Path: "repository/factory.go", Struct: "RepositoryFactory",
			Name: "Order", Type: "*OrderRepository"},
		&StructFieldInsert{Path: "repository/factory.go", Struct: "NoSuchStruct",
			Name: "Order", Type: "*OrderRepository"},
	})
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("预期 ErrNotFound，实际 %v", err)
	}
	if string(store.Files["repository/factory.go"]) != factoryFixture {
		t.Errorf("失败后不应写盘")
	}
}

// TestEngineMissingFile 目标文件不存在时要给出可读报错
// [EN] TestEngineMissingFile asserts a readable error for a missing target file
func TestEngineMissingFile(t *testing.T) {
	eng := NewEngine(NewMemStore(nil))
	_, err := eng.Apply(&DeclInsert{Path: "constants/table.go",
		Block: BlockRef{Member: "X"}, Name: "Y", Value: "1"})
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("预期 ErrNotFound，实际 %v", err)
	}
	if !strings.Contains(err.Error(), "constants/table.go") || !strings.Contains(err.Error(), "文件不存在") {
		t.Errorf("错误信息不够清楚: %v", err)
	}
}

// TestEngineSummary 摘要要报出文件、行号与条目标签，已存在的标 skip
// [EN] TestEngineSummary asserts the overview reports file, line, label and skip state
func TestEngineSummary(t *testing.T) {
	store := NewMemStore(map[string]string{"constants/table.go": constFixture})
	eng := NewEngine(store)
	eng.SetFormatPolicy(FormatNever)
	p := &DeclInsert{Path: "constants/table.go", Block: BlockRef{Member: "TableNameSecond"},
		Name: "TableNameOrder", Value: `"orders"`}
	if _, err := eng.Apply(p); err != nil {
		t.Fatalf("应用失败: %v", err)
	}
	if _, err := eng.Apply(p); err != nil {
		t.Fatalf("重复应用失败: %v", err)
	}
	got := eng.Summary()
	for _, want := range []string{
		"add    constants/table.go:15  const TableNameOrder",
		`| TableNameOrder = "orders"`,
		"skip   constants/table.go:15  const TableNameOrder  (已存在)",
		"共 0 个新建文件，1 处新增，1 处已存在跳过，涉及 1 个文件",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("摘要里缺少 %q\n实际输出:\n%s", want, got)
		}
	}
}

// TestDirStoreWriteAndGuard 目录存储要能原子写入，并挡住越出根目录的路径
// [EN] TestDirStoreWriteAndGuard covers atomic writes and the root escape guard
func TestDirStoreWriteAndGuard(t *testing.T) {
	root := t.TempDir()
	target := filepath.Join(root, "constants", "table.go")
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(target, []byte(constFixture), 0o600); err != nil {
		t.Fatal(err)
	}

	store := NewDirStore(root)
	eng := NewEngine(store)
	eng.SetFormatPolicy(FormatNever)
	if _, err := eng.Apply(&DeclInsert{Path: "constants/table.go",
		Block: BlockRef{Member: "TableNameSecond"}, Name: "TableNameOrder", Value: `"orders"`}); err != nil {
		t.Fatalf("应用失败: %v", err)
	}
	if _, err := eng.Write(); err != nil {
		t.Fatalf("写入失败: %v", err)
	}
	got, err := os.ReadFile(target)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(got), "TableNameOrder") {
		t.Errorf("文件没有被更新\n%s", got)
	}
	st, err := os.Stat(target)
	if err != nil {
		t.Fatal(err)
	}
	if st.Mode().Perm() != 0o600 {
		t.Errorf("原有文件权限应当保留，got %v", st.Mode().Perm())
	}

	if _, err := store.Read("../outside.go"); !errors.Is(err, ErrInvalidPatch) {
		t.Errorf("越出根目录的路径应当被拒绝，实际 %v", err)
	}
}
