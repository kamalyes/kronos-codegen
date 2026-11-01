/*
 * @Author: kamalyes 501893067@qq.com
 * @Date: 2026-11-01 16:20:11
 * @LastEditors: kamalyes 501893067@qq.com
 * @LastEditTime: 2026-11-01 20:10:35
 * @FilePath: \kronos-codegen\patch\stmt_insert.go
 * @Description: 往函数体或闭包体里插入注册语句，典型场景是服务注册与 bootstrap
 *
 * Copyright (c) 2026 by kamalyes, All Rights Reserved.
 */
package patch

import (
	"fmt"
	"go/ast"
	"strings"
)

// StmtInsert 往一个块的末尾插入语句
// 三种锚点按优先级生效：Marker 命中标记注释所在的最内层块（能钻进
// RegisterService(func(...){...}) 这种闭包）> Near 命中含有某条已有语句的最内层块
// > Func/Recv 命中函数体本身
// 幂等依据是把块内已有语句和目标语句都归一化后比较
// [EN] StmtInsert appends statements to the end of a block, located either by a marker
// comment (which reaches into closures) or by function name; idempotency compares
// printer-normalized statement text
type StmtInsert struct {
	// Path 目标文件相对路径
	// [EN] Path is the target file path
	Path string
	// Marker 标记注释片段，命中其所在的最内层块
	// [EN] Marker is a marker comment; the innermost block containing it is the target
	Marker string
	// Near 块里已经存在的某条语句源码，命中包含它的那个块
	// 生产代码里没有标记注释时用这个锚点，不必先去改目标文件
	// [EN] Near is a statement already present in the block; that block becomes the target
	Near string
	// After 为 true 时紧跟 Near 那条语句之后插入，而不是追加到块尾
	// 一个函数体里按业务分段（每段前面一条横幅注释）时，追加到块尾会落到最后一段，
	// 这个开关让新语句落在它该在的那一段里
	// [EN] After inserts immediately below the Near statement instead of appending at the end
	// of its block, which matters when one function body is divided into commented sections
	After bool
	// Func 目标函数名
	// [EN] Func is the target function name
	Func string
	// Recv 可选的方法接收者类型名
	// [EN] Recv is the optional receiver type name
	Recv string
	// Stmt 待插入的语句源码，支持多行
	// [EN] Stmt is the statement source to insert, multi-line allowed
	Stmt string
	// Doc 可选的上方注释
	// [EN] Doc is the optional preceding comment
	Doc string
}

// File 实现 Patch 接口
// [EN] File implements the Patch interface
func (p *StmtInsert) File() string { return p.Path }

// Label 实现 Patch 接口
// [EN] Label implements the Patch interface
func (p *StmtInsert) Label() string {
	where := p.Func
	switch {
	case p.Marker != "":
		where = p.Marker
	case p.Near != "":
		where = p.Near
	}
	return fmt.Sprintf("stmt %s <- %s", where, collapseSpace(p.Stmt))
}

// Plan 计算插入计划
// [EN] Plan computes the insertion plan
func (p *StmtInsert) Plan(src []byte) (*Plan, error) {
	if strings.TrimSpace(p.Stmt) == "" {
		return nil, locateErr(p.Path, p.Label(), "Stmt 不能为空", ErrInvalidPatch)
	}
	want, err := canonStmts(p.Stmt)
	if err != nil {
		return nil, locateErr(p.Path, p.Label(), fmt.Sprintf("Stmt 不是合法语句: %v", err), ErrInvalidPatch)
	}
	g, err := parseGo(p.Path, src)
	if err != nil {
		return nil, err
	}
	block, anchor, err := p.locate(g)
	if err != nil {
		return nil, err
	}
	out := &Plan{File: p.Path, Label: p.Label()}

	// 幂等：块内已有归一化等价的语句序列
	// [EN] idempotency: the block already contains the equivalent statements
	if line, ok := containsStmts(g, block, want); ok {
		out.Present = true
		out.Line = line
		return out, nil
	}

	lines := append(docLines(p.Doc), dedentLines(p.Stmt)...)
	if p.After {
		if anchor == nil {
			return nil, locateErr(p.Path, p.Label(),
				"After 需要配合 Near 使用，Marker 与 Func 锚点没有可紧跟的语句", ErrInvalidPatch)
		}
		out.Insertions = insertAfterLine(src, g.off(anchor.End()), g.off(anchor.Pos()), lines)
		out.Line = lineNumber(src, bodyInsertion(out.Insertions).Offset)
		out.Snippet = joinLines(lines)
		return out, nil
	}
	lastEnd := -1
	if n := len(block.List); n > 0 {
		lastEnd = g.off(block.List[n-1].End())
	}
	out.Insertions = buildAppend(src, g.off(block.Lbrace)+1, g.off(block.Rbrace), lastEnd, false, lines)
	out.Line = lineNumber(src, bodyInsertion(out.Insertions).Offset)
	out.Snippet = joinLines(lines)
	return out, nil
}

// locate 按锚点找到目标块，Near 锚点还会返回被命中的那条语句供 After 使用
// [EN] locate resolves the target block; a Near anchor also returns the matched statement
func (p *StmtInsert) locate(g *goFile) (*ast.BlockStmt, ast.Stmt, error) {
	if p.Marker != "" {
		pos, err := g.findMarkerComment(p.Marker)
		if err != nil {
			return nil, nil, err
		}
		n := innermostNodeAt(g.ast, pos, func(n ast.Node) bool {
			_, ok := n.(*ast.BlockStmt)
			return ok
		})
		if n == nil {
			return nil, nil, locateErr(p.Path, p.Label(),
				fmt.Sprintf("标记 %q 不在任何代码块内部", p.Marker), ErrNotFound)
		}
		return n.(*ast.BlockStmt), nil, nil
	}
	if p.Near != "" {
		near, err := canonStmts(p.Near)
		if err != nil {
			return nil, nil, locateErr(p.Path, p.Label(), fmt.Sprintf("Near 不是合法语句: %v", err), ErrInvalidPatch)
		}
		if block, at := g.findBlockContainingStmt(near[0]); block != nil {
			return block, at, nil
		}
		return nil, nil, locateErr(p.Path, p.Label(),
			fmt.Sprintf("找不到含有语句 %s 的代码块", p.Near), ErrNotFound)
	}
	if p.Func == "" {
		return nil, nil, locateErr(p.Path, p.Label(), "必须给出 Marker、Near 或 Func", ErrInvalidPatch)
	}
	fd, err := g.findFunc(p.Func, p.Recv)
	if err != nil {
		return nil, nil, err
	}
	return fd.Body, nil, nil
}

// containsStmts 判断块内是否已经存在给定的归一化语句序列
// 单条语句只要出现过即可，多条语句要求连续出现
// [EN] containsStmts reports whether the block already holds the normalized statements
func containsStmts(g *goFile, block *ast.BlockStmt, want []string) (int, bool) {
	if len(want) == 0 {
		return 0, false
	}
	got := make([]string, len(block.List))
	for i, st := range block.List {
		got[i] = normalizeCode(g.print(st))
	}
	for i := 0; i+len(want) <= len(got); i++ {
		match := true
		for j := range want {
			if got[i+j] != want[j] {
				match = false
				break
			}
		}
		if match {
			return g.fset.Position(block.List[i].Pos()).Line, true
		}
	}
	return 0, false
}
