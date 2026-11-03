/*
 * @Author: kamalyes 501893067@qq.com
 * @Date: 2026-11-03 17:10:00
 * @LastEditors: kamalyes 501893067@qq.com
 * @LastEditTime: 2026-11-03 17:10:00
 * @FilePath: \kronos-codegen\pbfield\pbfield_test.go
 * @Description: 字段提取与模型映射的断言
 *
 * 夹具在 testdata 里而不是指向某个真实契约仓库的绝对路径：指绝对路径的测试
 * 只在写它的那台机器上通过，CI 与别人的机器必然失败
 *
 * [EN] The fixture lives in testdata rather than pointing at an absolute path inside some
 * real contract repository, since a test with an absolute path passes only on the machine
 * that wrote it and necessarily fails in CI and on anyone else's machine
 *
 * Copyright (c) 2026 by kamalyes, All Rights Reserved.
 */

package pbfield

import (
	"os"
	"path/filepath"
	"testing"
)

// fixture 把 testdata 里的 .pb.go.txt 复制成临时的 .go 供解析
// [EN] fixture copies the .pb.go.txt from testdata into a temporary .go file to parse
func fixture(t *testing.T) string {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("testdata", "sample.pb.go.txt"))
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "sample.pb.go")
	if err := os.WriteFile(path, raw, 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestParseFields(t *testing.T) {
	m, err := Parse(fixture(t), "OrderInfo")
	if err != nil {
		t.Fatal(err)
	}
	if len(m.Fields) != 9 {
		t.Fatalf("字段数 got %d want 9", len(m.Fields))
	}

	// protoimpl 的内部字段必须被滤掉，它们不是业务字段
	// [EN] protoimpl's internal fields must be filtered out, as they are not business fields
	for _, f := range m.Fields {
		switch f.GoName {
		case "state", "sizeCache", "unknownFields":
			t.Errorf("内部字段 %s 不该出现在字段表里", f.GoName)
		}
	}

	want := []struct {
		goName, goType, protoName string
		number                    int
		repeated                  bool
	}{
		{"OrderId", "string", "order_id", 1, false},
		{"TenantId", "string", "tenant_id", 2, false},
		{"Title", "string", "title", 3, false},
		{"Amount", "int64", "amount", 4, false},
		{"Paid", "bool", "paid", 5, false},
		{"Status", "enums.CommonStatus", "status", 6, false},
		{"ItemIds", "[]string", "item_ids", 7, true},
		{"CreatedAt", "*timestamppb.Timestamp", "created_at", 8, false},
		{"UpdatedAt", "*timestamppb.Timestamp", "updated_at", 9, false},
	}
	for i, w := range want {
		got := m.Fields[i]
		if got.GoName != w.goName || got.GoType != w.goType || got.ProtoName != w.protoName ||
			got.Number != w.number || got.Repeated != w.repeated {
			t.Errorf("第 %d 个字段不符\n  got  %+v\n  want %v", i, got, w)
		}
		if got.Comment == "" {
			t.Errorf("第 %d 个字段的注释丢了，双语注释是字段语义的唯一来源", i)
		}
	}

	// 导入表要能把别名解析回真实包路径，否则生成的模型编译不过
	// [EN] The import table must resolve an alias back to its real package path, or the
	// generated model will not compile
	if got := m.Imports["enums"]; got != "github.com/example/demo-proto/pb/enums" {
		t.Errorf("enums 别名解析错误: %q", got)
	}
}

func TestParseMissingMessage(t *testing.T) {
	if _, err := Parse(fixture(t), "NoSuchInfo"); err == nil {
		t.Fatal("消息不存在时应报错")
	}
}

func TestModelMapping(t *testing.T) {
	m, err := Parse(fixture(t), "OrderInfo")
	if err != nil {
		t.Fatal(err)
	}
	fields := Model(m, "order", nil)

	byName := map[string]ModelField{}
	for _, f := range fields {
		byName[f.Name] = f
	}

	cases := map[string]struct{ typ, tag string }{
		// Ids 归一成 IDs 而不是 IDS：对后缀整体 ToUpper 会得到 ItemIDS
		// [EN] Ids normalizes to IDs rather than IDS, which upper-casing the whole suffix gives
		"ItemIDs":   {"[]string", "column:item_ids;serializer:json"},
		"OrderID":   {"string", "column:order_id;type:varchar(36);primaryKey"},
		"TenantID":  {"string", "column:tenant_id;type:varchar(36);not null;default:'';index"},
		"Title":     {"string", "column:title;type:varchar(255);not null;default:''"},
		"Amount":    {"int64", "column:amount;not null;default:0"},
		"Paid":      {"bool", "column:paid;not null;default:false"},
		"Status":    {"enums.CommonStatus", "column:status;type:int;not null;default:0"},
		"CreatedAt": {"time.Time", "column:created_at;autoCreateTime"},
		"UpdatedAt": {"time.Time", "column:updated_at;autoUpdateTime"},
	}
	for name, w := range cases {
		got, ok := byName[name]
		if !ok {
			t.Errorf("缺字段 %s", name)
			continue
		}
		if got.Type != w.typ || got.Tag != w.tag {
			t.Errorf("%s 映射不符\n  got  %s `%s`\n  want %s `%s`", name, got.Type, got.Tag, w.typ, w.tag)
		}
	}

	// 主键不该再带索引：primaryKey 本身就是索引
	// [EN] The primary key carries no extra index, since primaryKey is already one
	if tag := byName["OrderID"].Tag; contains(tag, ";index") {
		t.Errorf("主键多了一个索引: %s", tag)
	}
}

// TestModelIndexColumnsOverride 显式给出索引列时只认那份清单
//
// 默认规则是「非主键的 *_id 列」，刻意不认任何具体业务列名 —— 把 tenant_id
// 写死在工具里，等于要求所有项目都用同一套租户模型
//
// [EN] An explicit index list is the only one honoured
// The default rule is non-primary columns ending in _id and deliberately knows no business
// column name, since hardcoding tenant_id would require every project to share one tenancy model
func TestModelIndexColumnsOverride(t *testing.T) {
	m, err := Parse(fixture(t), "OrderInfo")
	if err != nil {
		t.Fatal(err)
	}
	fields := Model(m, "order", []string{"title"})
	for _, f := range fields {
		switch f.Name {
		case "Title":
			if !contains(f.Tag, ";index") {
				t.Errorf("显式指定的 title 没有索引: %s", f.Tag)
			}
		case "TenantID":
			if contains(f.Tag, ";index") {
				t.Errorf("给了显式清单后 tenant_id 不该再自动加索引: %s", f.Tag)
			}
		}
	}
}

func TestModelImports(t *testing.T) {
	m, err := Parse(fixture(t), "OrderInfo")
	if err != nil {
		t.Fatal(err)
	}
	got := ModelImports(Model(m, "order", nil), m.Imports)
	want := []string{"github.com/example/demo-proto/pb/enums", "time"}
	if len(got) != len(want) {
		t.Fatalf("导入数 got %v want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("导入 %d got %q want %q", i, got[i], want[i])
		}
	}
}

func contains(s, sub string) bool {
	return len(s) >= len(sub) && (func() bool {
		for i := 0; i+len(sub) <= len(s); i++ {
			if s[i:i+len(sub)] == sub {
				return true
			}
		}
		return false
	})()
}
