/*
 * @Author: kamalyes 501893067@qq.com
 * @Date: 2026-11-01 16:20:11
 * @LastEditors: kamalyes 501893067@qq.com
 * @LastEditTime: 2026-11-01 20:10:35
 * @FilePath: \kronos-codegen\patch\struct_insert.go
 * @Description: 往 struct 里加字段，以及往构造函数的复合字面量里加对应初始化项
 *
 * Copyright (c) 2026 by kamalyes, All Rights Reserved.
 */
package patch

import (
	"fmt"
	"go/ast"
	"go/token"
	"strings"
)

// StructFieldInsert 往结构体末尾追加一个字段，典型场景是 Repository 工厂结构体
// 幂等依据是结构体里是否已有同名字段，匿名字段按其类型名比对
// [EN] StructFieldInsert appends a field to a struct; idempotency is decided by whether
// the struct already declares a field with that name
type StructFieldInsert struct {
	// Path 目标文件相对路径
	// [EN] Path is the target file path
	Path string
	// Struct 结构体类型名
	// [EN] Struct is the struct type name
	Struct string
	// Name 字段名
	// [EN] Name is the field name
	Name string
	// Type 字段类型源码
	// [EN] Type is the field type expression
	Type string
	// Tag 可选的结构体 tag，不含反引号
	// [EN] Tag is the optional struct tag, without backquotes
	Tag string
	// Doc 可选的上方注释
	// [EN] Doc is the optional preceding comment
	Doc string
}

// File 实现 Patch 接口
// [EN] File implements the Patch interface
func (p *StructFieldInsert) File() string { return p.Path }

// Label 实现 Patch 接口
// [EN] Label implements the Patch interface
func (p *StructFieldInsert) Label() string {
	return fmt.Sprintf("struct %s 字段 %s", p.Struct, p.Name)
}

// Plan 计算插入计划
// [EN] Plan computes the insertion plan
func (p *StructFieldInsert) Plan(src []byte) (*Plan, error) {
	if !token.IsIdentifier(p.Name) {
		return nil, locateErr(p.Path, p.Label(), fmt.Sprintf("字段名 %q 不是合法标识符", p.Name), ErrInvalidPatch)
	}
	if strings.TrimSpace(p.Type) == "" {
		return nil, locateErr(p.Path, p.Label(), "Type 不能为空", ErrInvalidPatch)
	}
	g, err := parseGo(p.Path, src)
	if err != nil {
		return nil, err
	}
	st, err := g.findStruct(p.Struct)
	if err != nil {
		return nil, err
	}
	out := &Plan{File: p.Path, Label: p.Label()}

	// 幂等：字段已存在就不重复插
	// [EN] idempotency: the field already exists
	for _, f := range st.Fields.List {
		if len(f.Names) == 0 {
			// 匿名字段按类型名比对 / [EN] embedded field compared by its type name
			if normalizeCode(g.print(f.Type)) == p.Name {
				out.Present = true
				out.Line = g.fset.Position(f.Pos()).Line
				return out, nil
			}
			continue
		}
		for _, n := range f.Names {
			if n.Name == p.Name {
				out.Present = true
				out.Line = g.fset.Position(n.Pos()).Line
				return out, nil
			}
		}
	}

	entry := p.Name + " " + strings.TrimSpace(p.Type)
	if p.Tag != "" {
		entry += " `" + p.Tag + "`"
	}
	lines := append(docLines(p.Doc), entry)

	lastEnd := -1
	if n := len(st.Fields.List); n > 0 {
		lastEnd = g.off(st.Fields.List[n-1].End())
	}
	out.Insertions = buildAppend(src, g.off(st.Fields.Opening)+1, g.off(st.Fields.Closing), lastEnd, false, lines)
	out.Line = lineNumber(src, bodyInsertion(out.Insertions).Offset)
	out.Snippet = joinLines(lines)
	return out, nil
}

// CompositeFieldInsert 往复合字面量里追加一个 key: value 初始化项
// 典型场景是构造函数里的 &RepositoryFactory{...}，与 StructFieldInsert 配对使用
// 幂等依据是字面量里是否已有同名 key，走 KeyValueExpr 的 AST 判断
// [EN] CompositeFieldInsert appends a key: value element to a composite literal, typically
// the one inside a constructor; idempotency is decided by the literal's existing keys
type CompositeFieldInsert struct {
	// Path 目标文件相对路径
	// [EN] Path is the target file path
	Path string
	// Func 字面量所在的函数名
	// [EN] Func is the function holding the literal
	Func string
	// Recv 可选的方法接收者类型名
	// [EN] Recv is the optional receiver type name
	Recv string
	// Type 复合字面量的类型名，为空则取函数体里第一个复合字面量
	// [EN] Type is the literal's type name; empty takes the first literal in the body
	Type string
	// Key 初始化项的字段名
	// [EN] Key is the element's field name
	Key string
	// Value 初始化项的值表达式源码
	// [EN] Value is the element's value expression
	Value string
	// Doc 可选的上方注释
	// [EN] Doc is the optional preceding comment
	Doc string
}

// File 实现 Patch 接口
// [EN] File implements the Patch interface
func (p *CompositeFieldInsert) File() string { return p.Path }

// Label 实现 Patch 接口
// [EN] Label implements the Patch interface
func (p *CompositeFieldInsert) Label() string {
	return fmt.Sprintf("%s() 初始化项 %s", p.Func, p.Key)
}

// Plan 计算插入计划
// [EN] Plan computes the insertion plan
func (p *CompositeFieldInsert) Plan(src []byte) (*Plan, error) {
	if !token.IsIdentifier(p.Key) {
		return nil, locateErr(p.Path, p.Label(), fmt.Sprintf("Key %q 不是合法标识符", p.Key), ErrInvalidPatch)
	}
	if _, err := canonExpr(p.Value); err != nil {
		return nil, locateErr(p.Path, p.Label(), fmt.Sprintf("Value %q 不是合法表达式", p.Value), ErrInvalidPatch)
	}
	g, err := parseGo(p.Path, src)
	if err != nil {
		return nil, err
	}
	fd, err := g.findFunc(p.Func, p.Recv)
	if err != nil {
		return nil, err
	}
	cl := g.findCompositeLit(fd.Body, p.Type, false)
	if cl == nil {
		detail := fmt.Sprintf("函数 %s 里找不到复合字面量", p.Func)
		if p.Type != "" {
			detail = fmt.Sprintf("函数 %s 里找不到类型为 %s 的复合字面量", p.Func, p.Type)
		}
		return nil, locateErr(p.Path, p.Label(), detail, ErrNotFound)
	}
	out := &Plan{File: p.Path, Label: p.Label()}

	// 幂等：同名 key 已存在
	// [EN] idempotency: an element with that key already exists
	for _, el := range cl.Elts {
		kv, ok := el.(*ast.KeyValueExpr)
		if !ok {
			continue
		}
		if id, ok := kv.Key.(*ast.Ident); ok && id.Name == p.Key {
			out.Present = true
			out.Line = g.fset.Position(kv.Pos()).Line
			return out, nil
		}
	}

	lines := append(docLines(p.Doc), p.Key+": "+strings.TrimSpace(p.Value)+",")
	lastEnd := -1
	if n := len(cl.Elts); n > 0 {
		lastEnd = g.off(cl.Elts[n-1].End())
	}
	out.Insertions = buildAppend(src, g.off(cl.Lbrace)+1, g.off(cl.Rbrace), lastEnd, true, lines)
	out.Line = lineNumber(src, bodyInsertion(out.Insertions).Offset)
	out.Snippet = joinLines(lines)
	return out, nil
}

// joinLines 把多行片段拼成预览用的单块文本
// [EN] joinLines joins snippet lines for preview output
func joinLines(lines []string) string { return strings.Join(lines, "\n") }
