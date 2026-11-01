/*
 * @Author: kamalyes 501893067@qq.com
 * @Date: 2026-11-01 16:20:11
 * @LastEditors: kamalyes 501893067@qq.com
 * @LastEditTime: 2026-11-01 20:10:35
 * @FilePath: \kronos-codegen\patch\import_insert.go
 * @Description: 往 import 括号块插入一条导入 - 按路径判幂等，按空行识别分组落位
 *
 * Copyright (c) 2026 by kamalyes, All Rights Reserved.
 */
package patch

import (
	"fmt"
	"go/ast"
	"go/token"
	"strconv"
	"strings"
)

// ImportInsert 往 import 括号块里追加一条导入，支持可选别名
//
// 两件事和其他补丁不同：
//  1. 幂等判定按**导入路径**而不是别名：同一个路径导入两次是编译错误，所以路径已存在但别名
//     不一致时直接报 ErrConflict，绝不插第二条
//  2. Go 的 import 块习惯按标准库 / 三方 / 内部用空行分组，gofmt 不会跨组重排，
//     所以 Near 给出同组里已有的某个导入路径，新条目就落到那一组的末尾
//
// [EN] ImportInsert appends one import to a parenthesized import block. Idempotency is
// keyed on the import PATH, never the alias: importing the same path twice does not
// compile, so a path that already exists under a different alias raises ErrConflict.
// Import blocks are conventionally split into blank-line separated groups that gofmt never
// merges, so Near names an existing path whose group the new entry joins.
type ImportInsert struct {
	// Path 目标文件相对路径
	// [EN] Path is the target file path
	Path string
	// Import 导入路径
	// [EN] Import is the import path
	Import string
	// Alias 可选别名，支持 _ 与 .
	// [EN] Alias is the optional import alias; _ and . are accepted
	Alias string
	// Near 可选：同组里已经存在的某个导入路径，新条目落到它所在分组的末尾
	// 留空则追加到 import 块的最后一组
	// [EN] Near optionally names an existing import path whose group the new entry joins;
	// empty appends to the last group of the block
	Near string
	// Doc 可选的上方注释
	// [EN] Doc is the optional preceding comment
	Doc string
}

// File 实现 Patch 接口
// [EN] File implements the Patch interface
func (p *ImportInsert) File() string { return p.Path }

// Label 实现 Patch 接口
// [EN] Label implements the Patch interface
func (p *ImportInsert) Label() string {
	if p.Alias != "" {
		return fmt.Sprintf("import %s %q", p.Alias, p.Import)
	}
	return fmt.Sprintf("import %q", p.Import)
}

// Plan 计算插入计划
// [EN] Plan computes the insertion plan
func (p *ImportInsert) Plan(src []byte) (*Plan, error) {
	path := strings.TrimSpace(p.Import)
	if path == "" {
		return nil, locateErr(p.Path, p.Label(), "Import 不能为空", ErrInvalidPatch)
	}
	if strings.ContainsAny(path, "\"`\n") {
		return nil, locateErr(p.Path, p.Label(),
			fmt.Sprintf("导入路径 %q 含有非法字符", p.Import), ErrInvalidPatch)
	}
	if p.Alias != "" && p.Alias != "." && !token.IsIdentifier(p.Alias) {
		return nil, locateErr(p.Path, p.Label(),
			fmt.Sprintf("别名 %q 不是合法标识符", p.Alias), ErrInvalidPatch)
	}
	g, err := parseGo(p.Path, src)
	if err != nil {
		return nil, err
	}
	out := &Plan{File: p.Path, Label: p.Label()}

	// 幂等：按导入路径判定，别名不一致则报冲突
	// [EN] idempotency keyed on the path; a differing alias is a conflict
	for _, spec := range g.ast.Imports {
		if importPath(spec) != path {
			continue
		}
		got := importAlias(spec)
		if got == p.Alias {
			out.Present = true
			out.Line = g.fset.Position(spec.Pos()).Line
			return out, nil
		}
		return nil, locateErr(p.Path, p.Label(), fmt.Sprintf(
			"路径 %q 已经以别名 %s 导入过，与期望的别名 %s 冲突，不能重复导入同一路径",
			path, describeAlias(got), describeAlias(p.Alias)), ErrConflict)
	}

	gd, err := g.findImportDecl(p.Near, p.Label())
	if err != nil {
		return nil, err
	}
	entry := strconv.Quote(path)
	if p.Alias != "" {
		entry = p.Alias + " " + entry
	}
	lines := append(docLines(p.Doc), entry)

	if p.Near != "" {
		anchor, err := g.importGroupEnd(gd, p.Near, p.Label())
		if err != nil {
			return nil, err
		}
		out.Insertions = insertAfterLine(src, g.off(anchor.End()), g.off(anchor.Pos()), lines)
	} else {
		out.Insertions = buildAppend(src, g.off(gd.Lparen)+1, g.off(gd.Rparen), g.lastSpecEnd(gd), false, lines)
	}
	out.Line = lineNumber(src, bodyInsertion(out.Insertions).Offset)
	out.Snippet = joinLines(lines)
	return out, nil
}

// describeAlias 把空别名描述成「无别名」，让冲突信息读得懂
// [EN] describeAlias renders an empty alias readably in the conflict message
func describeAlias(a string) string {
	if a == "" {
		return "<无别名>"
	}
	return strconv.Quote(a)
}

// importPath 取 ImportSpec 的导入路径，已去掉引号
// [EN] importPath returns the unquoted import path of a spec
func importPath(spec *ast.ImportSpec) string {
	if v, err := strconv.Unquote(spec.Path.Value); err == nil {
		return v
	}
	return strings.Trim(spec.Path.Value, "\"`")
}

// importAlias 取 ImportSpec 的别名，无别名返回空串
// [EN] importAlias returns the alias of a spec, or an empty string
func importAlias(spec *ast.ImportSpec) string {
	if spec.Name == nil {
		return ""
	}
	return spec.Name.Name
}

// importSpecs 取一个 import 声明块里的全部 spec
// [EN] importSpecs returns every spec of one import declaration
func importSpecs(gd *ast.GenDecl) []*ast.ImportSpec {
	out := make([]*ast.ImportSpec, 0, len(gd.Specs))
	for _, s := range gd.Specs {
		if is, ok := s.(*ast.ImportSpec); ok {
			out = append(out, is)
		}
	}
	return out
}

// findImportDecl 找到可追加的 import 括号块
// near 非空时取包含该导入路径的那个块
// [EN] findImportDecl locates the appendable parenthesized import block
func (g *goFile) findImportDecl(near, label string) (*ast.GenDecl, error) {
	var decls []*ast.GenDecl
	for _, d := range g.ast.Decls {
		gd, ok := d.(*ast.GenDecl)
		if !ok || gd.Tok != token.IMPORT {
			continue
		}
		decls = append(decls, gd)
	}
	if len(decls) == 0 {
		return nil, locateErr(g.path, label,
			"文件里没有 import 声明，补丁只负责往已存在的 import 块里插入条目", ErrNotFound)
	}
	if near != "" {
		for _, gd := range decls {
			for _, spec := range importSpecs(gd) {
				if importPath(spec) != near {
					continue
				}
				if !gd.Lparen.IsValid() {
					return nil, locateErr(g.path, label, fmt.Sprintf(
						"锚点 %q 命中的是单行 import 声明，无法追加条目，请先改成 import ( ... ) 括号块",
						near), ErrNotFound)
				}
				return gd, nil
			}
		}
		return nil, locateErr(g.path, label,
			fmt.Sprintf("锚点导入路径 %q 不在该文件的 import 块里", near), ErrNotFound)
	}
	for _, gd := range decls {
		if gd.Lparen.IsValid() {
			return gd, nil
		}
	}
	return nil, locateErr(g.path, label,
		"文件里只有单行 import 声明，无法追加条目，请先改成 import ( ... ) 括号块", ErrNotFound)
}

// importGroupEnd 返回 near 所在分组的最后一个 spec
// 分组边界就是空行，这也是 gofmt 唯一不会跨越的边界
// [EN] importGroupEnd returns the last spec of the group holding near; a blank line marks
// the group boundary, which is the one boundary gofmt never crosses
func (g *goFile) importGroupEnd(gd *ast.GenDecl, near, label string) (*ast.ImportSpec, error) {
	specs := importSpecs(gd)
	idx := -1
	for i, spec := range specs {
		if importPath(spec) == near {
			idx = i
			break
		}
	}
	if idx < 0 {
		return nil, locateErr(g.path, label,
			fmt.Sprintf("锚点导入路径 %q 不在该 import 块里", near), ErrNotFound)
	}
	last := idx
	for last+1 < len(specs) &&
		!hasBlankLineBetween(g.src, g.off(specs[last].End()), g.off(specs[last+1].Pos())) {
		last++
	}
	return specs[last], nil
}
