/*
 * @Author: kamalyes 501893067@qq.com
 * @Date: 2026-11-01 16:20:11
 * @LastEditors: kamalyes 501893067@qq.com
 * @LastEditTime: 2026-11-01 20:10:35
 * @FilePath: \kronos-codegen\patch\json_insert.go
 * @Description: 保序的 JSON 键插入 - 用 json.Decoder 的 token 流定位偏移，不经过 map
 *
 * Copyright (c) 2026 by kamalyes, All Rights Reserved.
 */
package patch

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"path"
	"strconv"
	"strings"
)

// JSONKeyInsert 往 JSON 对象里插入一个键，保持原有键顺序与缩进风格
//
// 为什么不用 map：encoding/json 解到 map[string]any 会丢键顺序，再 Marshal 出来会按字典序
// 重排整个文件，28 个语言文件会全量变脏；用 json.RawMessage 保序需要预先知道 schema
// 这里走 json.Decoder 的 token 流，只为了拿到「目标对象闭合括号的字节偏移」和
// 「最后一个成员值的结束偏移」，然后在原字节上拼接，其余字节一律不动
// [EN] JSONKeyInsert inserts one key into a JSON object while preserving key order and
// indentation. Decoding into a map would lose ordering and reserialize the whole file, so
// the decoder's token stream is used purely to compute byte offsets for a text splice.
type JSONKeyInsert struct {
	// Path 目标文件相对路径
	// [EN] Path is the target file path
	Path string
	// Parent 父对象的键路径，空表示顶层对象
	// [EN] Parent is the key path of the parent object; empty means the root object
	Parent []string
	// Key 要插入的键名
	// [EN] Key is the key to insert
	Key string
	// Value 键值的 JSON 字面量源码，如 "\"Hello\"" 或 {"a":1}
	// [EN] Value is the JSON literal of the value
	Value string
}

// File 实现 Patch 接口
// [EN] File implements the Patch interface
func (p *JSONKeyInsert) File() string { return p.Path }

// Label 实现 Patch 接口
// [EN] Label implements the Patch interface
func (p *JSONKeyInsert) Label() string {
	return fmt.Sprintf("json 键 %s", strings.Join(append(append([]string{}, p.Parent...), p.Key), "."))
}

// Plan 计算插入计划
// [EN] Plan computes the insertion plan
func (p *JSONKeyInsert) Plan(src []byte) (*Plan, error) {
	if p.Key == "" {
		return nil, locateErr(p.Path, p.Label(), "Key 不能为空", ErrInvalidPatch)
	}
	if !json.Valid([]byte(p.Value)) {
		return nil, locateErr(p.Path, p.Label(), fmt.Sprintf("Value %q 不是合法 JSON", p.Value), ErrInvalidPatch)
	}
	var compactVal bytes.Buffer
	if err := json.Compact(&compactVal, []byte(p.Value)); err != nil {
		return nil, locateErr(p.Path, p.Label(), fmt.Sprintf("Value 压缩失败: %v", err), ErrInvalidPatch)
	}
	obj, err := locateJSONObject(p.Path, src, p.Parent, p.Label())
	if err != nil {
		return nil, err
	}
	out := &Plan{File: p.Path, Label: p.Label()}

	// 幂等：键已存在，不覆盖已有翻译
	// [EN] idempotency: the key already exists and existing translations are never clobbered
	if line, ok := obj.keys[p.Key]; ok {
		out.Present = true
		out.Line = line
		return out, nil
	}

	entry := strconv.Quote(p.Key) + jsonColon(src, obj) + compactVal.String()
	inner := src[obj.openEnd:obj.closeOff]
	if !bytes.ContainsRune(inner, '\n') {
		// 紧凑对象：保持单行风格，直接接在最后一个成员后面
		// [EN] compact object: stay on one line, splice right after the last member
		anchor, text := obj.openEnd, entry
		if obj.lastEnd >= 0 {
			anchor = obj.lastEnd
			text = "," + jsonSpaceAfterComma(inner) + entry
		}
		out.Insertions = []Insertion{{Offset: anchor, Text: text}}
	} else {
		out.Insertions = buildAppendIndent(src, obj.openEnd, obj.closeOff, obj.lastEnd, true,
			detectJSONIndentUnit(src), []string{entry})
	}
	out.Line = lineNumber(src, bodyInsertion(out.Insertions).Offset)
	out.Snippet = entry
	return out, nil
}

// LocaleKeys 为一组 locale 文件生成同一个键的插入补丁
// values 按文件名（不含扩展名，如 en、zh-tw）给出译文，缺失的语言退回 fallback
// [EN] LocaleKeys builds one JSONKeyInsert per locale file, falling back when a language
// has no translation of its own
func LocaleKeys(files []string, parent []string, key string, values map[string]string, fallback string) []Patch {
	out := make([]Patch, 0, len(files))
	for _, f := range files {
		lang := strings.TrimSuffix(path.Base(f), path.Ext(f))
		text, ok := values[lang]
		if !ok {
			text = fallback
		}
		out = append(out, &JSONKeyInsert{
			Path:   f,
			Parent: parent,
			Key:    key,
			Value:  string(mustJSONString(text)),
		})
	}
	return out
}

// mustJSONString 把纯文本编码成 JSON 字符串字面量
// [EN] mustJSONString encodes plain text as a JSON string literal
func mustJSONString(s string) []byte {
	b, err := json.Marshal(s)
	if err != nil {
		return []byte(strconv.Quote(s))
	}
	return b
}

// jsonObject 一个 JSON 对象在源字节里的位置信息
// [EN] jsonObject records where one JSON object lives in the source bytes
type jsonObject struct {
	// openEnd 左花括号之后的偏移
	// [EN] openEnd is the offset just after the opening brace
	openEnd int
	// closeOff 右花括号自身的偏移
	// [EN] closeOff is the offset of the closing brace itself
	closeOff int
	// lastEnd 最后一个成员值的结束偏移，空对象为 -1
	// [EN] lastEnd is the end offset of the last member value, or -1 when empty
	lastEnd int
	// keys 已有键到行号的映射，用于幂等判定
	// [EN] keys maps existing keys to their line numbers, used for idempotency
	keys map[string]int
}

// locateJSONObject 在源字节里定位 parent 指向的对象，parent 为空时返回顶层对象
// [EN] locateJSONObject resolves the object addressed by parent, or the root object
func locateJSONObject(file string, src []byte, parent []string, label string) (*jsonObject, error) {
	dec := json.NewDecoder(bytes.NewReader(src))
	dec.UseNumber()
	t, err := dec.Token()
	if err != nil {
		return nil, locateErr(file, label, fmt.Sprintf("JSON 解析失败: %v", err), ErrParse)
	}
	if d, ok := t.(json.Delim); !ok || d != '{' {
		return nil, locateErr(file, label, "顶层不是 JSON 对象", ErrParse)
	}
	self, target, err := consumeJSONObject(dec, src, parent)
	if err != nil {
		if _, ok := err.(*LocateError); ok {
			return nil, err
		}
		return nil, locateErr(file, label, fmt.Sprintf("JSON 解析失败: %v", err), ErrParse)
	}
	if len(parent) == 0 {
		return &self, nil
	}
	if target == nil {
		return nil, locateErr(file, label,
			fmt.Sprintf("父对象路径 %s 不存在", strings.Join(parent, ".")), ErrNotFound)
	}
	return target, nil
}

// consumeJSONObject 消费一个对象（左花括号已被读走），顺带在子树里找目标对象
// 返回值依次是当前对象自身的位置信息、want 指向的目标对象、错误
// [EN] consumeJSONObject consumes one object whose opening brace was already read, and
// locates the object addressed by want along the way
func consumeJSONObject(dec *json.Decoder, src []byte, want []string) (jsonObject, *jsonObject, error) {
	self := jsonObject{openEnd: int(dec.InputOffset()), lastEnd: -1, keys: map[string]int{}}
	var target *jsonObject
	for {
		t, err := dec.Token()
		if err != nil {
			if err == io.EOF {
				return self, nil, fmt.Errorf("对象没有闭合")
			}
			return self, nil, err
		}
		if d, ok := t.(json.Delim); ok {
			if d == '}' {
				self.closeOff = int(dec.InputOffset()) - 1
				return self, target, nil
			}
			return self, nil, fmt.Errorf("对象里出现意外的 %v", d)
		}
		key, ok := t.(string)
		if !ok {
			return self, nil, fmt.Errorf("对象键不是字符串: %v", t)
		}
		self.keys[key] = lineNumber(src, int(dec.InputOffset()))

		matched := len(want) > 0 && want[0] == key
		vt, err := dec.Token()
		if err != nil {
			return self, nil, err
		}
		if d, ok := vt.(json.Delim); ok {
			switch d {
			case '{':
				var sub []string
				if matched {
					sub = want[1:]
				}
				child, childTarget, err := consumeJSONObject(dec, src, sub)
				if err != nil {
					return self, nil, err
				}
				switch {
				case matched && len(want) == 1:
					c := child
					target = &c
				case childTarget != nil:
					target = childTarget
				}
			case '[':
				if matched {
					return self, nil, fmt.Errorf("路径段 %q 对应的是数组而不是对象", key)
				}
				if err := skipJSONArray(dec); err != nil {
					return self, nil, err
				}
			default:
				return self, nil, fmt.Errorf("值位置出现意外的 %v", d)
			}
		} else if matched {
			return self, nil, fmt.Errorf("路径段 %q 对应的是标量而不是对象", key)
		}
		self.lastEnd = int(dec.InputOffset())
	}
}

// skipJSONArray 跳过一个数组（左方括号已被读走）
// [EN] skipJSONArray skips one array whose opening bracket was already read
func skipJSONArray(dec *json.Decoder) error {
	depth := 1
	for depth > 0 {
		t, err := dec.Token()
		if err != nil {
			return err
		}
		if d, ok := t.(json.Delim); ok {
			switch d {
			case '[', '{':
				depth++
			case ']', '}':
				depth--
			}
		}
	}
	return nil
}

// jsonColon 按对象内已有风格决定冒号后面是否带空格
// [EN] jsonColon mirrors the object's existing spacing after the colon
func jsonColon(src []byte, obj *jsonObject) string {
	inner := src[obj.openEnd:obj.closeOff]
	if len(obj.keys) == 0 || bytes.Contains(inner, []byte(`": `)) || bytes.ContainsRune(inner, '\n') {
		return ": "
	}
	return ":"
}

// jsonSpaceAfterComma 按对象内已有风格决定逗号后面是否带空格
// [EN] jsonSpaceAfterComma mirrors the object's existing spacing after a comma
func jsonSpaceAfterComma(inner []byte) string {
	if bytes.Contains(inner, []byte(", ")) {
		return " "
	}
	return ""
}

// detectJSONIndentUnit 探测文件的缩进单位，空对象插入时用得上
// [EN] detectJSONIndentUnit sniffs the file's indent unit, used when the object is empty
func detectJSONIndentUnit(src []byte) string {
	for _, line := range bytes.Split(src, []byte{'\n'}) {
		trimmed := bytes.TrimLeft(line, " \t")
		if len(trimmed) == 0 || len(trimmed) == len(line) {
			continue
		}
		return string(line[:len(line)-len(trimmed)])
	}
	return "  "
}
