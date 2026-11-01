/*
 * @Author: kamalyes 501893067@qq.com
 * @Date: 2026-11-01 18:05:12
 * @LastEditors: kamalyes 501893067@qq.com
 * @LastEditTime: 2026-11-01 20:12:06
 * @FilePath: \kronos-codegen\patch\json_insert_test.go
 * @Description: JSON locale 插入测试 - 键顺序、缩进风格、幂等与父路径报错
 *
 * Copyright (c) 2026 by kamalyes, All Rights Reserved.
 */
package patch

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"
)

// jsonKeyOrder 用 token 流读出键的出现顺序，不经过 map
// [EN] jsonKeyOrder reads the key order from the token stream, bypassing maps
func jsonKeyOrder(t *testing.T, src string) []string {
	t.Helper()
	dec := json.NewDecoder(strings.NewReader(src))
	if _, err := dec.Token(); err != nil {
		t.Fatalf("不是合法 JSON: %v", err)
	}
	var keys []string
	for {
		tok, err := dec.Token()
		if err != nil {
			t.Fatalf("读取 token 失败: %v", err)
		}
		if d, ok := tok.(json.Delim); ok && d == '}' {
			return keys
		}
		key, ok := tok.(string)
		if !ok {
			t.Fatalf("键不是字符串: %v", tok)
		}
		keys = append(keys, key)
		var v json.RawMessage
		if err := dec.Decode(&v); err != nil {
			t.Fatalf("读取值失败: %v", err)
		}
	}
}

// TestJSONKeyInsertGolden JSON 插入的正常路径，输出必须逐字节等于预期
// [EN] TestJSONKeyInsertGolden covers the happy path byte for byte
func TestJSONKeyInsertGolden(t *testing.T) {
	cases := []struct {
		name   string
		before string
		patch  Patch
		after  string
	}{
		{
			name:   "顶层追加键保持两空格缩进",
			before: localeFixture,
			patch:  &JSONKeyInsert{Path: "locales/en.json", Key: "order.created", Value: `"Order created"`},
			after: `{
  "welcome": "Welcome",
  "success": "Success",
  "user": {
    "not_found": "User not found"
  },
  "goodbye": "Goodbye",
  "order.created": "Order created"
}
`,
		},
		{
			name:   "嵌套对象追加键",
			before: localeFixture,
			patch:  &JSONKeyInsert{Path: "locales/en.json", Parent: []string{"user"}, Key: "disabled", Value: `"User disabled"`},
			after: `{
  "welcome": "Welcome",
  "success": "Success",
  "user": {
    "not_found": "User not found",
    "disabled": "User disabled"
  },
  "goodbye": "Goodbye"
}
`,
		},
		{
			name:   "紧凑单行 JSON 保持紧凑",
			before: localeCompactFixture,
			patch:  &JSONKeyInsert{Path: "locales/en.json", Key: "order", Value: `"Order"`},
			after:  `{"welcome":"Welcome","success":"Success","order":"Order"}`,
		},
		{
			name:   "空对象按探测到的缩进插入",
			before: localeEmptyFixture,
			patch:  &JSONKeyInsert{Path: "locales/en.json", Key: "order", Value: `"Order"`},
			after: `{
  "order": "Order"
}
`,
		},
		{
			name:   "对象值被压成单行",
			before: localeFixture,
			patch:  &JSONKeyInsert{Path: "locales/en.json", Key: "order", Value: "{\n  \"created\": \"ok\"\n}"},
			after: `{
  "welcome": "Welcome",
  "success": "Success",
  "user": {
    "not_found": "User not found"
  },
  "goodbye": "Goodbye",
  "order": {"created":"ok"}
}
`,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, pl := mustApply(t, tc.before, tc.patch, FormatNever)
			if pl.Present {
				t.Fatalf("首次插入不应判为已存在")
			}
			if got != tc.after {
				t.Errorf("输出与预期不一致\n--- got ---\n%s\n--- want ---\n%s", got, tc.after)
			}
			if !json.Valid([]byte(got)) {
				t.Errorf("输出不是合法 JSON\n%s", got)
			}
		})
	}
}

// TestJSONKeyOrderPreserved 原有键顺序必须完整保留，新键追加在末尾
// encoding/json 解到 map 会丢顺序，这条测试守住「不经过 map」的实现约束
// [EN] TestJSONKeyOrderPreserved asserts existing key order survives and the new key lands
// at the end, which is what decoding through a map would destroy
func TestJSONKeyOrderPreserved(t *testing.T) {
	before := `{
  "zebra": "Z",
  "apple": "A",
  "mango": "M"
}
`
	p := &JSONKeyInsert{Path: "locales/en.json", Key: "banana", Value: `"B"`}
	got, _ := mustApply(t, before, p, FormatNever)
	want := []string{"zebra", "apple", "mango", "banana"}
	order := jsonKeyOrder(t, got)
	if strings.Join(order, ",") != strings.Join(want, ",") {
		t.Errorf("键顺序被打乱: got %v, want %v\n%s", order, want, got)
	}
}

// TestJSONKeyIdempotent 同一个键插入两次，第二次是 no-op 且不覆盖已有译文
// [EN] TestJSONKeyIdempotent asserts the second insertion is a no-op and never clobbers
func TestJSONKeyIdempotent(t *testing.T) {
	p := &JSONKeyInsert{Path: "locales/en.json", Key: "welcome", Value: `"Different value"`}
	got, pl := mustApply(t, localeFixture, p, FormatNever)
	if !pl.Present {
		t.Fatalf("已存在的键应判为 Present")
	}
	if got != localeFixture {
		t.Errorf("已存在的键不应改动文件\n%s", got)
	}
	if pl.Line != 2 {
		t.Errorf("Present 时应报告已存在条目的行号，got %d want 2", pl.Line)
	}

	nested := &JSONKeyInsert{Path: "locales/en.json", Parent: []string{"user"}, Key: "disabled", Value: `"User disabled"`}
	once, _ := mustApply(t, localeFixture, nested, FormatNever)
	twice, pl2 := mustApply(t, once, nested, FormatNever)
	if !pl2.Present || twice != once {
		t.Errorf("嵌套键重复插入不是 no-op\n%s", twice)
	}
}

// TestJSONKeyErrors JSON 相关的边界报错
// [EN] TestJSONKeyErrors covers the JSON failure modes
func TestJSONKeyErrors(t *testing.T) {
	cases := []struct {
		name    string
		before  string
		patch   Patch
		wantErr error
		wantMsg string
	}{
		{
			name:    "父路径不存在",
			before:  localeFixture,
			patch:   &JSONKeyInsert{Path: "a.json", Parent: []string{"nope"}, Key: "k", Value: `"v"`},
			wantErr: ErrNotFound,
			wantMsg: "nope",
		},
		{
			name:    "父路径指向标量",
			before:  localeFixture,
			patch:   &JSONKeyInsert{Path: "a.json", Parent: []string{"welcome"}, Key: "k", Value: `"v"`},
			wantErr: ErrParse,
			wantMsg: "标量",
		},
		{
			name:    "顶层不是对象",
			before:  `["a","b"]`,
			patch:   &JSONKeyInsert{Path: "a.json", Key: "k", Value: `"v"`},
			wantErr: ErrParse,
			wantMsg: "顶层不是 JSON 对象",
		},
		{
			name:    "文件不是合法 JSON",
			before:  `{"a": }`,
			patch:   &JSONKeyInsert{Path: "a.json", Key: "k", Value: `"v"`},
			wantErr: ErrParse,
			wantMsg: "JSON 解析失败",
		},
		{
			name:    "值不是合法 JSON",
			before:  localeFixture,
			patch:   &JSONKeyInsert{Path: "a.json", Key: "k", Value: `不是 JSON`},
			wantErr: ErrInvalidPatch,
			wantMsg: "不是合法 JSON",
		},
		{
			name:    "键名为空",
			before:  localeFixture,
			patch:   &JSONKeyInsert{Path: "a.json", Value: `"v"`},
			wantErr: ErrInvalidPatch,
			wantMsg: "Key 不能为空",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, _, err := ApplyToSource([]byte(tc.before), tc.patch, FormatNever)
			if err == nil {
				t.Fatalf("预期报错，实际成功")
			}
			if !errors.Is(err, tc.wantErr) {
				t.Errorf("错误类型不符: got %v, want %v", err, tc.wantErr)
			}
			if !strings.Contains(err.Error(), tc.wantMsg) {
				t.Errorf("错误信息里应当含有 %q，实际为 %q", tc.wantMsg, err.Error())
			}
		})
	}
}

// TestLocaleKeys 一个键扇出到多个语言文件，缺译文的语言退回 fallback
// [EN] TestLocaleKeys fans one key out to several locale files with a fallback
func TestLocaleKeys(t *testing.T) {
	files := []string{"locales/en.json", "locales/zh.json", "locales/ja.json"}
	patches := LocaleKeys(files, nil, "order.created", map[string]string{
		"zh": "订单已创建",
	}, "Order created")
	if len(patches) != 3 {
		t.Fatalf("应当生成 3 条补丁，实际 %d", len(patches))
	}
	store := NewMemStore(map[string]string{
		"locales/en.json": localeFixture,
		"locales/zh.json": localeFixture,
		"locales/ja.json": localeFixture,
	})
	eng := NewEngine(store)
	if _, err := eng.ApplyAll(patches); err != nil {
		t.Fatalf("应用失败: %v", err)
	}
	if _, err := eng.Write(); err != nil {
		t.Fatalf("写入失败: %v", err)
	}
	if !strings.Contains(string(store.Files["locales/zh.json"]), `"order.created": "订单已创建"`) {
		t.Errorf("中文译文没有用上\n%s", store.Files["locales/zh.json"])
	}
	if !strings.Contains(string(store.Files["locales/ja.json"]), `"order.created": "Order created"`) {
		t.Errorf("缺译文的语言没有退回 fallback\n%s", store.Files["locales/ja.json"])
	}
}
