/*
 * @Author: kamalyes 501893067@qq.com
 * @Date: 2026-11-01 16:20:11
 * @LastEditors: kamalyes 501893067@qq.com
 * @LastEditTime: 2026-11-01 20:10:35
 * @FilePath: \kronos-codegen\patch\slice_insert.go
 * @Description: 往切片字面量末尾追加元素，典型场景是迁移的模型清单与索引清单
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

// SliceAppend 往切片字面量末尾追加一个元素
// 四种锚点按优先级生效：Marker（字面量内的标记注释）> Near（字面量里已有的某个元素）
// > Var（包级变量）> Func（函数体里的切片字面量）
// 幂等依据是把已有元素和目标元素都用 go/printer 归一化后比较，纯注释改动不会误判
// [EN] SliceAppend appends one element to a slice literal located by marker, package-level
// variable or enclosing function; idempotency compares printer-normalized element text
type SliceAppend struct {
	// Path 目标文件相对路径
	// [EN] Path is the target file path
	Path string
	// Marker 切片字面量内部的标记注释片段
	// [EN] Marker is a marker comment sitting inside the slice literal
	Marker string
	// Near 字面量里已经存在的某个元素表达式，命中包含它的那个字面量
	// 生产代码里没有标记注释时用这个锚点，不必先去改目标文件
	// [EN] Near is an element already present in the literal; the literal holding it wins
	Near string
	// Var 包级变量名，其值是切片字面量
	// [EN] Var is the package-level variable whose value is the slice literal
	Var string
	// Func 返回该切片的函数名
	// [EN] Func is the function returning the slice literal
	Func string
	// Recv 可选的方法接收者类型名
	// [EN] Recv is the optional receiver type name
	Recv string
	// Type 可选的字面量类型文本，如 []interface{}，用于同一函数里有多个切片时消歧
	// [EN] Type optionally disambiguates by literal type, e.g. []interface{}
	Type string
	// Element 待追加的元素表达式源码，支持多行
	// [EN] Element is the element expression source, multi-line allowed
	Element string
	// Doc 可选的上方注释
	// [EN] Doc is the optional preceding comment
	Doc string
}

// File 实现 Patch 接口
// [EN] File implements the Patch interface
func (p *SliceAppend) File() string { return p.Path }

// Label 实现 Patch 接口
// [EN] Label implements the Patch interface
func (p *SliceAppend) Label() string {
	where := p.Func
	switch {
	case p.Marker != "":
		where = p.Marker
	case p.Near != "":
		where = p.Near
	case p.Var != "":
		where = p.Var
	}
	return fmt.Sprintf("slice %s 元素 %s", where, collapseSpace(p.Element))
}

// Plan 计算插入计划
// [EN] Plan computes the insertion plan
func (p *SliceAppend) Plan(src []byte) (*Plan, error) {
	if strings.TrimSpace(p.Element) == "" {
		return nil, locateErr(p.Path, p.Label(), "Element 不能为空", ErrInvalidPatch)
	}
	want, err := canonExpr(p.Element)
	if err != nil {
		return nil, locateErr(p.Path, p.Label(), fmt.Sprintf("Element 不是合法表达式: %v", err), ErrInvalidPatch)
	}
	g, err := parseGo(p.Path, src)
	if err != nil {
		return nil, err
	}
	cl, err := p.locate(g)
	if err != nil {
		return nil, err
	}
	out := &Plan{File: p.Path, Label: p.Label()}

	// 幂等：归一化后已有同样的元素
	// [EN] idempotency: a structurally identical element is already present
	for _, el := range cl.Elts {
		if normalizeCode(g.print(el)) == want {
			out.Present = true
			out.Line = g.fset.Position(el.Pos()).Line
			return out, nil
		}
	}

	lines := append(docLines(p.Doc), withTrailingComma(dedentLines(p.Element))...)
	lastEnd := -1
	if n := len(cl.Elts); n > 0 {
		lastEnd = g.off(cl.Elts[n-1].End())
	}
	out.Insertions = buildAppend(src, g.off(cl.Lbrace)+1, g.off(cl.Rbrace), lastEnd, true, lines)
	out.Line = lineNumber(src, bodyInsertion(out.Insertions).Offset)
	out.Snippet = joinLines(lines)
	return out, nil
}

// locate 按锚点优先级找到目标切片字面量
// [EN] locate resolves the target slice literal following the anchor precedence
func (p *SliceAppend) locate(g *goFile) (*ast.CompositeLit, error) {
	switch {
	case p.Marker != "":
		pos, err := g.findMarkerComment(p.Marker)
		if err != nil {
			return nil, err
		}
		n := innermostNodeAt(g.ast, pos, func(n ast.Node) bool {
			cl, ok := n.(*ast.CompositeLit)
			if !ok {
				return false
			}
			_, isArr := cl.Type.(*ast.ArrayType)
			return isArr || cl.Type == nil
		})
		if n == nil {
			return nil, locateErr(p.Path, p.Label(),
				fmt.Sprintf("标记 %q 不在任何切片字面量内部", p.Marker), ErrNotFound)
		}
		return n.(*ast.CompositeLit), nil

	case p.Near != "":
		near, err := canonExpr(p.Near)
		if err != nil {
			return nil, locateErr(p.Path, p.Label(), fmt.Sprintf("Near 不是合法表达式: %v", err), ErrInvalidPatch)
		}
		if cl := g.findLitContainingElem(near); cl != nil {
			return cl, nil
		}
		return nil, locateErr(p.Path, p.Label(),
			fmt.Sprintf("找不到含有元素 %s 的字面量", p.Near), ErrNotFound)

	case p.Var != "":
		for _, d := range g.ast.Decls {
			gd, ok := d.(*ast.GenDecl)
			if !ok || gd.Tok != token.VAR {
				continue
			}
			for _, s := range gd.Specs {
				vs, ok := s.(*ast.ValueSpec)
				if !ok {
					continue
				}
				for i, n := range vs.Names {
					if n.Name != p.Var || i >= len(vs.Values) {
						continue
					}
					if cl, ok := vs.Values[i].(*ast.CompositeLit); ok {
						return cl, nil
					}
					return nil, locateErr(p.Path, p.Label(),
						fmt.Sprintf("变量 %s 的值不是复合字面量", p.Var), ErrNotFound)
				}
			}
		}
		return nil, locateErr(p.Path, p.Label(), fmt.Sprintf("包级变量 %s 不存在", p.Var), ErrNotFound)

	case p.Func != "":
		fd, err := g.findFunc(p.Func, p.Recv)
		if err != nil {
			return nil, err
		}
		cl := g.findCompositeLit(fd.Body, p.Type, p.Type == "")
		if cl == nil {
			detail := fmt.Sprintf("函数 %s 里找不到切片字面量", p.Func)
			if p.Type != "" {
				detail = fmt.Sprintf("函数 %s 里找不到类型为 %s 的字面量", p.Func, p.Type)
			}
			return nil, locateErr(p.Path, p.Label(), detail, ErrNotFound)
		}
		return cl, nil

	default:
		return nil, locateErr(p.Path, p.Label(), "必须给出 Marker、Near、Var 或 Func 之一", ErrInvalidPatch)
	}
}

// withTrailingComma 给元素的最后一行补上尾逗号
// gofmt 风格的多行字面量要求最后一个元素也带逗号，单行字面量里带逗号同样合法
// [EN] withTrailingComma adds the trailing comma gofmt-style literals require
func withTrailingComma(lines []string) []string {
	if len(lines) == 0 {
		return lines
	}
	last := len(lines) - 1
	if !strings.HasSuffix(strings.TrimRight(lines[last], " \t"), ",") {
		lines[last] = strings.TrimRight(lines[last], " \t") + ","
	}
	return lines
}
