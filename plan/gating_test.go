/*
 * @Author: kamalyes 501893067@qq.com
 * @Date: 2026-11-03 18:10:00
 * @LastEditors: kamalyes 501893067@qq.com
 * @LastEditTime: 2026-11-03 18:10:00
 * @FilePath: \kronos-codegen\plan\gating_test.go
 * @Description: optional / when / group 三个闸门的断言
 *
 * 这三个字段各管一种「这一条不该执行」的情形，彼此不能互相替代：
 *  optional 管文件不存在、when 管模板变量为空、group 管同组有条目定位不到
 * 都是实测逼出来的，所以各自都要有测试钉住
 *
 * [EN] Assertions for the optional, when and group gates
 * Each covers a different reason a step should not run and none substitutes for another:
 * optional for an absent file, when for an empty template variable, and group for a sibling
 * step that cannot be located
 * All three came out of measurements, so each is pinned by a test
 *
 * Copyright (c) 2026 by kamalyes, All Rights Reserved.
 */

package plan

import (
	"os"
	"path/filepath"
	"testing"
)

// gatingManifest 一份只用来测闸门的清单
func gatingManifest(t *testing.T) *Manifest {
	t.Helper()
	raw := []byte(`{
  "version": 1,
  "sections": {
    "entity": [
      {"kind": "var", "file": "present.go", "name": "A{{.Name}}", "value": "1"},
      {"kind": "var", "file": "absent.go", "name": "B{{.Name}}", "value": "2", "optional": true},
      {"kind": "var", "file": "present.go", "name": "C{{.Name}}", "value": "{{.PbAlias}}", "when": "PbAlias"}
    ]
  }
}`)
	m, err := Parse(raw, "测试清单")
	if err != nil {
		t.Fatal(err)
	}
	return m
}

// root 造一个只有 present.go 的项目目录
func root(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	src := "package demo\n\nvar (\n\tSeed = 0\n)\n"
	if err := os.WriteFile(filepath.Join(dir, "present.go"), []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}
	return dir
}

// TestOptionalSkipsAbsentFile 目标文件不存在时，optional 的条目被跳过而不是报错
// [EN] An optional step whose target file is absent is skipped rather than failing
func TestOptionalSkipsAbsentFile(t *testing.T) {
	m := gatingManifest(t)
	dir := root(t)

	items, err := m.Items("entity", Data("Order"), dir)
	if err != nil {
		t.Fatalf("不该报错: %v", err)
	}
	for _, it := range items {
		if it.Action.File() == "absent.go" {
			t.Errorf("absent.go 不存在，这一条应当被跳过")
		}
	}
}

// TestOptionalAbsentFileWithoutFlagFails 同一条不加 optional 时必须报错
//
// 这条反向断言是必要的：如果「文件不存在」本来就不报错，optional 这个字段就是
// 装饰品，上面那条测试也证明不了任何事
//
// [EN] The same step without the flag must fail
// This negative assertion is necessary: if an absent file never failed, the optional field
// would be decoration and the test above would prove nothing
func TestOptionalAbsentFileWithoutFlagFails(t *testing.T) {
	raw := []byte(`{"version":1,"sections":{"entity":[
	  {"kind":"var","file":"absent.go","name":"B{{.Name}}","value":"2"}]}}`)
	m, err := Parse(raw, "测试清单")
	if err != nil {
		t.Fatal(err)
	}
	items, err := m.Items("entity", Data("Order"), root(t))
	if err != nil {
		// 展开阶段就报错也算挡住了
		return
	}
	if len(items) != 1 {
		t.Fatalf("没加 optional 的条目被跳过了，items=%d", len(items))
	}
}

// TestWhenSkipsOnEmptyVar 模板变量为空时，when 的条目被跳过
//
// 为什么不能只靠 optional：这里 present.go 是存在的，缺的是数据
// 不跳过的话插进去的是 `C Order = ` 这种值为空的声明
//
// [EN] A when step is skipped while its variable is empty
// Why optional cannot cover this: present.go does exist and what is missing is the data,
// and not skipping would insert a declaration with an empty value
func TestWhenSkipsOnEmptyVar(t *testing.T) {
	m := gatingManifest(t)
	dir := root(t)

	// PbAlias 缺席
	items, err := m.Items("entity", Data("Order"), dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 {
		t.Fatalf("PbAlias 为空时应只剩 1 条，实得 %d", len(items))
	}

	// PbAlias 有值
	data := Data("Order")
	data["PbAlias"] = "commonpb"
	items, err = m.Items("entity", data, dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 2 {
		t.Fatalf("PbAlias 有值时应有 2 条，实得 %d", len(items))
	}
}

// TestWhenSkipsOnFalseBool 布尔变量为 false 时同样跳过
// HasService 这类开关是布尔，空串判断覆盖不到它
//
// [EN] A false boolean also skips, since a switch such as HasService is a bool and an
// empty-string check would not cover it
func TestWhenSkipsOnFalseBool(t *testing.T) {
	raw := []byte(`{"version":1,"sections":{"entity":[
	  {"kind":"var","file":"present.go","name":"D{{.Name}}","value":"1","when":"HasService"}]}}`)
	m, err := Parse(raw, "测试清单")
	if err != nil {
		t.Fatal(err)
	}
	dir := root(t)

	data := Data("Order")
	data["HasService"] = false
	items, err := m.Items("entity", data, dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 0 {
		t.Fatalf("HasService 为 false 时应全部跳过，实得 %d 条", len(items))
	}

	data["HasService"] = true
	items, err = m.Items("entity", data, dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 {
		t.Fatalf("HasService 为 true 时应有 1 条，实得 %d", len(items))
	}
}

// TestGroupIsCarried group 标记要原样带到 Item 上
//
// 整组跳过的决策在调用方（new 命令）做，它需要这个标记才能把同组的条目认出来
// 这里只断言清单层把标记带出来了，不吞掉
//
// [EN] The group tag is carried through to Item
// The all-or-nothing decision is made by the caller, the new command, which needs the tag
// to recognise siblings; this only asserts the manifest layer carries it rather than
// swallowing it
func TestGroupIsCarried(t *testing.T) {
	raw := []byte(`{"version":1,"sections":{"entity":[
	  {"kind":"var","file":"present.go","name":"E{{.Name}}","value":"1","group":"exports"},
	  {"kind":"var","file":"present.go","name":"F{{.Name}}","value":"2"}]}}`)
	m, err := Parse(raw, "测试清单")
	if err != nil {
		t.Fatal(err)
	}
	items, err := m.Items("entity", Data("Order"), root(t))
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 2 {
		t.Fatalf("应有 2 条，实得 %d", len(items))
	}
	if items[0].Group != "exports" {
		t.Errorf("第一条的 group 丢了: %q", items[0].Group)
	}
	if items[1].Group != "" {
		t.Errorf("第二条没有 group，却带上了 %q", items[1].Group)
	}
}

// TestUnknownFieldStillRejected 新增三个字段之后，写错字段名仍然要报错
// 清单解析用 DisallowUnknownFields，这是写错字段名能立刻暴露的前提
//
// [EN] A misspelled field is still rejected after the three new fields were added, since
// the manifest is parsed with DisallowUnknownFields and that is what surfaces typos
func TestUnknownFieldStillRejected(t *testing.T) {
	raw := []byte(`{"version":1,"sections":{"entity":[
	  {"kind":"var","file":"present.go","name":"G","value":"1","optionl":true}]}}`)
	if _, err := Parse(raw, "测试清单"); err == nil {
		t.Fatal("字段名写错时应当报错")
	}
}
