/*
 * @Author: kamalyes 501893067@qq.com
 * @Date: 2026-11-01 16:20:11
 * @LastEditors: kamalyes 501893067@qq.com
 * @LastEditTime: 2026-11-01 20:10:35
 * @FilePath: \kronos-codegen\patch\decl_insert.go
 * @Description: 往命名的 const / var / type 括号块里插入一个成员，同名声明即视为已存在
 *
 * Copyright (c) 2026 by kamalyes, All Rights Reserved.
 */
package patch

import (
	"fmt"
	"go/token"
	"strings"
)

// DeclKind 声明块的种类
// [EN] DeclKind is the kind of declaration block being patched
type DeclKind string

const (
	// DeclConst const 块，零值也等同于它
	// [EN] DeclConst targets a const block and is also the zero value
	DeclConst DeclKind = "const"
	// DeclVar var 块
	// [EN] DeclVar targets a var block
	DeclVar DeclKind = "var"
	// DeclType type 块，apex 的 service/exports.go 用它做跨包别名
	// [EN] DeclType targets a type block, used by apex for cross-package aliases
	DeclType DeclKind = "type"
)

// DeclInsert 往一个括号声明块里追加成员，覆盖 const、var 与 type 三种块
//
// 成员文本按 Decl 拼装：
//   - const / var：Name [Type] = Value，Value 必填
//   - type 且给了 Value：Name = Value，也就是类型别名
//   - type 且给了 Type：Name Type，也就是类型定义
//
// 幂等依据是「文件里是否已经用同一个 token 声明过同名成员」，走 AST 查声明名而不是搜字符串
// [EN] DeclInsert appends a member to a parenthesized const, var or type block. Idempotency
// is decided by whether the file already declares that name with the same token, queried
// through the AST rather than by substring matching.
type DeclInsert struct {
	// Path 目标文件相对路径
	// [EN] Path is the target file path
	Path string
	// Decl 声明块种类，留空等同 DeclConst
	// [EN] Decl is the declaration kind; empty means DeclConst
	Decl DeclKind
	// Block 定位声明块的锚点
	// [EN] Block anchors the declaration block
	Block BlockRef
	// Name 成员名
	// [EN] Name is the member name
	Name string
	// Type 显式类型：const / var 下是可选的类型标注，type 下是类型定义的目标类型
	// [EN] Type is an optional type annotation for const and var, or the defined type for type
	Type string
	// Value 值表达式：const / var 下必填，type 下给出即表示这是一个类型别名
	// [EN] Value is the value expression, required for const and var; for type it marks an alias
	Value string
	// Doc 可选的上方注释，多行用换行分隔
	// [EN] Doc is the optional preceding comment, newline separated
	Doc string
}

// File 实现 Patch 接口
// [EN] File implements the Patch interface
func (p *DeclInsert) File() string { return p.Path }

// Label 实现 Patch 接口
// [EN] Label implements the Patch interface
func (p *DeclInsert) Label() string {
	return fmt.Sprintf("%s %s", p.kind(), p.Name)
}

// kind 返回声明块种类，归一化掉零值
// [EN] kind returns the declaration kind with the zero value normalized
func (p *DeclInsert) kind() DeclKind {
	if p.Decl == "" {
		return DeclConst
	}
	return p.Decl
}

// tok 返回本补丁操作的声明 token
// [EN] tok returns the declaration token this patch targets
func (p *DeclInsert) tok() (token.Token, error) {
	switch p.kind() {
	case DeclConst:
		return token.CONST, nil
	case DeclVar:
		return token.VAR, nil
	case DeclType:
		return token.TYPE, nil
	default:
		return token.ILLEGAL, locateErr(p.Path, "声明块",
			fmt.Sprintf("未知的 Decl %q，可选 const|var|type", p.Decl), ErrInvalidPatch)
	}
}

// entry 校验字段并拼出成员文本
// [EN] entry validates the fields and builds the member text
func (p *DeclInsert) entry() (string, error) {
	if !token.IsIdentifier(p.Name) {
		return "", locateErr(p.Path, p.Label(),
			fmt.Sprintf("成员名 %q 不是合法标识符", p.Name), ErrInvalidPatch)
	}
	typ, val := strings.TrimSpace(p.Type), strings.TrimSpace(p.Value)
	if typ != "" {
		if _, err := canonTypeExpr(typ); err != nil {
			return "", locateErr(p.Path, p.Label(),
				fmt.Sprintf("Type %q 不是合法类型", p.Type), ErrInvalidPatch)
		}
	}

	if p.kind() == DeclType {
		switch {
		case val != "" && typ != "":
			return "", locateErr(p.Path, p.Label(),
				"type 块的成员只能二选一：给 Value 表示别名，给 Type 表示类型定义", ErrInvalidPatch)
		case val != "":
			if _, err := canonTypeExpr(val); err != nil {
				return "", locateErr(p.Path, p.Label(),
					fmt.Sprintf("Value %q 不是合法类型", p.Value), ErrInvalidPatch)
			}
			return p.Name + " = " + val, nil
		case typ != "":
			return p.Name + " " + typ, nil
		default:
			return "", locateErr(p.Path, p.Label(),
				"type 块的成员需要 Value（别名）或 Type（类型定义）之一", ErrInvalidPatch)
		}
	}

	if val == "" {
		return "", locateErr(p.Path, p.Label(), "Value 不能为空", ErrInvalidPatch)
	}
	if _, err := canonExpr(val); err != nil {
		return "", locateErr(p.Path, p.Label(),
			fmt.Sprintf("Value %q 不是合法表达式", p.Value), ErrInvalidPatch)
	}
	entry := p.Name
	if typ != "" {
		entry += " " + typ
	}
	return entry + " = " + val, nil
}

// Plan 计算插入计划
// [EN] Plan computes the insertion plan
func (p *DeclInsert) Plan(src []byte) (*Plan, error) {
	tok, err := p.tok()
	if err != nil {
		return nil, err
	}
	entry, err := p.entry()
	if err != nil {
		return nil, err
	}
	g, err := parseGo(p.Path, src)
	if err != nil {
		return nil, err
	}
	out := &Plan{File: p.Path, Label: p.Label()}

	// 幂等：同名成员已经声明过就直接 no-op
	// [EN] idempotency: a member with that name already exists, so nothing to do
	if line, ok := g.declaresName(tok, p.Name); ok {
		out.Present = true
		out.Line = line
		return out, nil
	}

	gd, err := g.findDeclBlock(tok, p.Block)
	if err != nil {
		return nil, err
	}
	lines := append(docLines(p.Doc), entry)

	out.Insertions = buildAppend(src, g.off(gd.Lparen)+1, g.off(gd.Rparen), g.lastSpecEnd(gd), false, lines)
	out.Line = lineNumber(src, bodyInsertion(out.Insertions).Offset)
	out.Snippet = joinLines(lines)
	return out, nil
}
