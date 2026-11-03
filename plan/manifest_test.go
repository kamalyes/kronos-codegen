/*
 * @Author: kamalyes 501893067@qq.com
 * @Date: 2026-11-01 18:05:12
 * @LastEditors: kamalyes 501893067@qq.com
 * @LastEditTime: 2026-11-01 20:12:06
 * @FilePath: \kronos-codegen\plan\manifest_test.go
 * @Description: 补丁计划清单测试 - 校验、模板展开、glob 扇出与命名派生
 *
 * Copyright (c) 2026 by kamalyes, All Rights Reserved.
 */
package plan

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kamalyes/kronos-codegen/patch"
)

// writeFile 在临时目录里写一个文件
// [EN] writeFile writes one file into the temp directory
func writeFile(t *testing.T, root, rel, content string) string {
	t.Helper()
	abs := filepath.Join(root, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(abs), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(abs, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return abs
}

// TestLoadErrors 清单校验的边界
// [EN] TestLoadErrors covers manifest validation failures
func TestLoadErrors(t *testing.T) {
	cases := []struct {
		name    string
		content string
		wantMsg string
	}{
		{"版本不符", `{"version":99,"sections":{"entity":[]}}`, "只支持 1"},
		{"没有分组", `{"version":1}`, "没有任何 sections"},
		{"未知字段", `{"version":1,"sections":{},"nope":1}`, "解析补丁计划"},
		{"缺少 kind", `{"version":1,"sections":{"entity":[{"file":"a.go"}]}}`, "缺少 kind"},
		{"未知 kind", `{"version":1,"sections":{"entity":[{"kind":"wat","file":"a.go"}]}}`, "未知的 kind"},
		{"const 缺字段", `{"version":1,"sections":{"entity":[{"kind":"const","file":"a.go"}]}}`, "需要 name 与 value"},
		{"缺 file", `{"version":1,"sections":{"entity":[{"kind":"stmt","stmt":"f()"}]}}`, "需要 file"},
		{"json_key 缺文件", `{"version":1,"sections":{"entity":[{"kind":"json_key","key":"k"}]}}`, "需要 file 或 files"},
		{"import 缺路径", `{"version":1,"sections":{"entity":[{"kind":"import","file":"a.go"}]}}`, "需要 import"},
		{"create_file 既无 template 也无 content", `{"version":1,"sections":{"entity":[{"kind":"create_file","file":"a.go"}]}}`, "不能同时给"},
		{"create_file 同时给 template 与 content", `{"version":1,"sections":{"entity":[{"kind":"create_file","file":"a.go","template":"t.tmpl","content":"x"}]}}`, "不能同时给"},
		{"json_key 没给值", `{"version":1,"sections":{"entity":[{"kind":"json_key","file":"a.json","key":"k"}]}}`, "否则会插入空字符串"},
		{"type 缺 name", `{"version":1,"sections":{"entity":[{"kind":"type","file":"a.go","value":"X"}]}}`, "需要 name"},
		{"type 既无 value 也无 type", `{"version":1,"sections":{"entity":[{"kind":"type","file":"a.go","name":"X"}]}}`, "不能同时给"},
		{"type 同时给 value 与 type", `{"version":1,"sections":{"entity":[{"kind":"type","file":"a.go","name":"X","value":"Y","type":"string"}]}}`, "不能同时给"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			p := writeFile(t, root, DefaultFileName, tc.content)
			_, err := Load(p)
			if err == nil {
				t.Fatalf("预期报错，实际成功")
			}
			if !strings.Contains(err.Error(), tc.wantMsg) {
				t.Errorf("错误信息里应当含有 %q，实际为 %q", tc.wantMsg, err.Error())
			}
		})
	}
}

// TestPatchesTemplateAndGlob 模板展开与 locale glob 扇出
// [EN] TestPatchesTemplateAndGlob covers template expansion and locale glob fan-out
func TestPatchesTemplateAndGlob(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "locales/en.json", "{}")
	writeFile(t, root, "locales/zh.json", "{}")
	writeFile(t, root, "locales/ja.json", "{}")
	p := writeFile(t, root, DefaultFileName, `{
  "version": 1,
  "vars": {"Module": "demo"},
  "sections": {
    "entity": [
      {"kind":"const","file":"constants/{{.Snake}}.go","member":"TableNameExample","name":"TableName{{.Name}}","value":"\"{{.Snake}}s\""},
      {"kind":"json_key","files":["locales/*.json"],"key":"{{.Delimited}}.created","values":{"zh":"{{.Name}}已创建"},"fallback":"{{.Name}} created"}
    ]
  }
}`)
	m, err := Load(p)
	if err != nil {
		t.Fatalf("载入失败: %v", err)
	}
	patches, err := m.Actions("entity", Data("OrderItem"), root)
	if err != nil {
		t.Fatalf("展开失败: %v", err)
	}
	if len(patches) != 4 {
		t.Fatalf("应当展开成 1 + 3 条补丁，实际 %d", len(patches))
	}
	if got := patches[0].File(); got != "constants/order_item.go" {
		t.Errorf("路径模板没展开: %q", got)
	}
	if got := patches[0].Label(); got != "const TableNameOrderItem" {
		t.Errorf("常量名模板没展开: %q", got)
	}
	var files []string
	for _, p := range patches[1:] {
		files = append(files, p.File())
	}
	want := "locales/en.json,locales/ja.json,locales/zh.json"
	if strings.Join(files, ",") != want {
		t.Errorf("glob 扇出不符: got %v want %v", files, want)
	}
}

// TestPatchesMissingVar 模板引用了不存在的变量要直接报错，不能悄悄留空
// [EN] TestPatchesMissingVar asserts an unknown variable fails loudly
func TestPatchesMissingVar(t *testing.T) {
	root := t.TempDir()
	p := writeFile(t, root, DefaultFileName, `{
  "version": 1,
  "sections": {"entity": [
    {"kind":"const","file":"a.go","member":"X","name":"{{.NoSuchVar}}","value":"1"}
  ]}
}`)
	m, err := Load(p)
	if err != nil {
		t.Fatalf("载入失败: %v", err)
	}
	if _, err := m.Actions("entity", Data("Order"), root); err == nil {
		t.Fatalf("预期报错，实际成功")
	} else if !strings.Contains(err.Error(), "NoSuchVar") {
		t.Errorf("错误信息里应当提到变量名，实际为 %q", err.Error())
	}
}

// TestPatchesUnknownSection 分组不存在时报出现有分组
// [EN] TestPatchesUnknownSection lists the available sections when one is missing
func TestPatchesUnknownSection(t *testing.T) {
	m := &Manifest{Version: 1, Sections: map[string][]Step{"entity": nil, "field": nil}}
	_, err := m.Actions("nope", nil, ".")
	if err == nil || !strings.Contains(err.Error(), "entity, field") {
		t.Errorf("应当报出现有分组，实际 %v", err)
	}
}

// TestVars 命名派生
// [EN] TestVars covers the naming derivations
func TestVars(t *testing.T) {
	cases := []struct {
		in   string
		want map[string]string
	}{
		{"Order", map[string]string{"Name": "Order", "Lower": "order", "Snake": "order", "Kebab": "order", "Upper": "ORDER", "Flat": "order", "Delimited": "order"}},
		{"OrderItem", map[string]string{"Name": "OrderItem", "Lower": "orderItem", "Snake": "order_item", "Kebab": "order-item", "Upper": "ORDER_ITEM", "Flat": "orderitem", "Delimited": "order.item"}},
		{"order_item", map[string]string{"Name": "OrderItem", "Lower": "orderItem", "Snake": "order_item"}},
		{"HTTPServer", map[string]string{"Name": "HTTPServer", "Lower": "httpServer", "Snake": "http_server"}},
		{"AccessControlPolicy", map[string]string{"Name": "AccessControlPolicy", "Snake": "access_control_policy"}},
	}
	for _, tc := range cases {
		t.Run(tc.in, func(t *testing.T) {
			got := Vars(tc.in)
			for k, want := range tc.want {
				if got[k] != want {
					t.Errorf("%s[%s] = %q, want %q", tc.in, k, got[k], want)
				}
			}
		})
	}
}

// TestPatchesImportAndTypeKinds import 与 type 两个 kind 的清单分发
// [EN] TestPatchesImportAndTypeKinds covers manifest dispatch for the import and type kinds
func TestPatchesImportAndTypeKinds(t *testing.T) {
	root := t.TempDir()
	p := writeFile(t, root, DefaultFileName, `{
  "version": 1,
  "sections": {
    "entity": [
      {
        "kind": "import",
        "file": "service/exports.go",
        "import": "github.com/example/demo-service/service/{{.Snake}}",
        "alias": "{{.Flat}}svc",
        "near": "github.com/example/demo-service/service/notify"
      },
      {
        "kind": "type",
        "file": "service/exports.go",
        "member": "NotifyServiceImpl",
        "name": "{{.Name}}ServiceImpl",
        "value": "{{.Flat}}svc.{{.Name}}ServiceImpl"
      },
      {
        "kind": "var",
        "file": "service/exports.go",
        "member": "NewNotifyService",
        "name": "New{{.Name}}Service",
        "value": "{{.Flat}}svc.New{{.Name}}Service"
      }
    ]
  }
}`)
	m, err := Load(p)
	if err != nil {
		t.Fatalf("载入失败: %v", err)
	}
	patches, err := m.Actions("entity", Data("NotifyTemplate"), root)
	if err != nil {
		t.Fatalf("展开失败: %v", err)
	}
	wantLabels := []string{
		`import notifytemplatesvc "github.com/example/demo-service/service/notify_template"`,
		"type NotifyTemplateServiceImpl",
		"var NewNotifyTemplateService",
	}
	if len(patches) != len(wantLabels) {
		t.Fatalf("应当展开成 %d 条补丁，实际 %d", len(wantLabels), len(patches))
	}
	for i, want := range wantLabels {
		if got := patches[i].Label(); got != want {
			t.Errorf("patches[%d].Label() = %q, want %q", i, got, want)
		}
	}
}

// TestCreateFileRendersTemplate create_file 走项目内模板，变量与插入层共用一套
// [EN] TestCreateFileRendersTemplate renders a project-owned template with the shared vars
func TestCreateFileRendersTemplate(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, ".kronos-codegen/templates/model.go.tmpl", `package models

// {{.Name}}Model {{.Name}} 数据模型
type {{.Name}}Model struct {
Id string
}

// TableName 指定表名
func ({{.Name}}Model) TableName() string { return "{{.Snake}}s" }
`)
	// create_file 故意写在最后，断言它会被提到最前面
	// [EN] create_file is deliberately last, to assert it gets hoisted to the front
	p := writeFile(t, root, DefaultFileName, `{
  "version": 1,
  "sections": {
    "entity": [
      {"kind":"var","file":"models/table_names.go","member":"TableNameProduct",
       "name":"TableName{{.Name}}","value":"{{.Name}}Model{}.TableName()"},
      {"kind":"create_file","file":"models/{{.Snake}}_model.go",
       "template":".kronos-codegen/templates/model.go.tmpl"},
      {"kind":"create_file","file":"docs/{{.Kebab}}.md",
       "content":"# {{.Name}}\n"}
    ]
  }
}`)
	m, err := Load(p)
	if err != nil {
		t.Fatalf("载入失败: %v", err)
	}
	actions, err := m.Actions("entity", Data("OrderItem"), root)
	if err != nil {
		t.Fatalf("展开失败: %v", err)
	}
	if len(actions) != 3 {
		t.Fatalf("应当展开成 3 步，实际 %d", len(actions))
	}
	wantFiles := []string{"models/order_item_model.go", "docs/order-item.md", "models/table_names.go"}
	for i, want := range wantFiles {
		if got := actions[i].File(); got != want {
			t.Errorf("actions[%d].File() = %q, want %q（create_file 应当排在最前）", i, got, want)
		}
	}
	c, ok := actions[0].(*patch.FileCreate)
	if !ok {
		t.Fatalf("actions[0] 应当是 *patch.FileCreate，实际 %T", actions[0])
	}
	body := string(c.Content)
	for _, want := range []string{"type OrderItemModel struct", `return "order_items"`} {
		if !strings.Contains(body, want) {
			t.Errorf("模板没有正确渲染，缺少 %q\n%s", want, body)
		}
	}
	if c.Origin != ".kronos-codegen/templates/model.go.tmpl" {
		t.Errorf("Origin 应当是模板路径，实际 %q", c.Origin)
	}
	inline, ok := actions[1].(*patch.FileCreate)
	if !ok || string(inline.Content) != "# OrderItem\n" {
		t.Errorf("内联 content 没有展开: %+v", actions[1])
	}
}

// TestCreateFileTemplateErrors create_file 的边界报错
// [EN] TestCreateFileTemplateErrors covers the create_file failure modes
func TestCreateFileTemplateErrors(t *testing.T) {
	root := t.TempDir()
	p := writeFile(t, root, DefaultFileName, `{
  "version": 1,
  "sections": {"entity": [
    {"kind":"create_file","file":"models/x.go","template":".kronos-codegen/templates/nope.tmpl"}
  ]}
}`)
	m, err := Load(p)
	if err != nil {
		t.Fatalf("载入失败: %v", err)
	}
	if _, err := m.Actions("entity", Data("Order"), root); err == nil {
		t.Fatalf("模板文件不存在时应当报错")
	} else if !strings.Contains(err.Error(), "读取模板") {
		t.Errorf("错误信息不够清楚: %v", err)
	}

	writeFile(t, root, ".kronos-codegen/templates/bad.tmpl", "package p\n// {{.Name\n")
	p2 := writeFile(t, root, "bad.json", `{
  "version": 1,
  "sections": {"entity": [
    {"kind":"create_file","file":"models/x.go","template":".kronos-codegen/templates/bad.tmpl"}
  ]}
}`)
	m2, err := Load(p2)
	if err != nil {
		t.Fatalf("载入失败: %v", err)
	}
	if _, err := m2.Actions("entity", Data("Order"), root); err == nil || !strings.Contains(err.Error(), "不合法") {
		t.Errorf("模板语法错误应当报错，实际 %v", err)
	}

	writeFile(t, root, ".kronos-codegen/templates/miss.tmpl", "package p // {{.NoSuchVar}}\n")
	p3 := writeFile(t, root, "miss.json", `{
  "version": 1,
  "sections": {"entity": [
    {"kind":"create_file","file":"models/x.go","template":".kronos-codegen/templates/miss.tmpl"}
  ]}
}`)
	m3, err := Load(p3)
	if err != nil {
		t.Fatalf("载入失败: %v", err)
	}
	if _, err := m3.Actions("entity", Data("Order"), root); err == nil || !strings.Contains(err.Error(), "NoSuchVar") {
		t.Errorf("模板引用未知变量应当报错，实际 %v", err)
	}
}
