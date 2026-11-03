/*
 * @Author: kamalyes 501893067@qq.com
 * @Date: 2026-11-01 16:20:11
 * @LastEditors: kamalyes 501893067@qq.com
 * @LastEditTime: 2026-11-01 20:10:35
 * @FilePath: \kronos-codegen\patch\astutil.go
 * @Description: AST 定位工具 - 只负责算偏移与做幂等查询，不参与任何代码输出
 *
 * Copyright (c) 2026 by kamalyes, All Rights Reserved.
 */
package patch

import (
	"bytes"
	"fmt"
	"go/ast"
	"go/parser"
	"go/printer"
	"go/scanner"
	"go/token"
	"strings"
)

// goFile 一次解析的结果，offsets 用 fset 换算
// [EN] goFile bundles one parse result; offsets are resolved through fset
type goFile struct {
	fset *token.FileSet
	ast  *ast.File
	src  []byte
	path string
}

// parseGo 解析 Go 源码并保留注释，注释必须保留否则标记注释锚点无法工作
// [EN] parseGo parses Go source with comments, which the marker anchors depend on
func parseGo(path string, src []byte) (*goFile, error) {
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, path, src, parser.ParseComments|parser.SkipObjectResolution)
	if err != nil {
		return nil, locateErr(path, "Go 源码", err.Error(), ErrParse)
	}
	return &goFile{fset: fset, ast: f, src: src, path: path}, nil
}

// off 把 token.Pos 换算成字节偏移
// [EN] off converts a token.Pos into a byte offset
func (g *goFile) off(p token.Pos) int {
	if !p.IsValid() {
		return -1
	}
	return g.fset.Position(p).Offset
}

// text 返回节点对应的原始源码片段
// [EN] text returns the original source slice of a node
func (g *goFile) text(n ast.Node) string {
	s, e := g.off(n.Pos()), g.off(n.End())
	if s < 0 || e < 0 || s > e || e > len(g.src) {
		return ""
	}
	return string(g.src[s:e])
}

// print 用 go/printer 打印单个节点，仅用于归一化比较，绝不写回文件
// [EN] print renders one node via go/printer, used only for normalized comparison
func (g *goFile) print(n ast.Node) string {
	var b bytes.Buffer
	if err := printer.Fprint(&b, g.fset, n); err != nil {
		return g.text(n)
	}
	return b.String()
}

// normalizeCode 把一段代码归一化成「以空格分隔的 token 流」，用于结构等价比较
//
// 为什么不直接按空白切分：go/printer 打印多行链式调用时会保留换行，
// 按空白切分得到的是 `x. Foo(). Bar()`，点号后面多一个空格，
// 清单作者必须照着这个怪形状写 near 才能命中，极不直观
// 走 go/scanner 则拿到真正的 token 序列，既与换行和空格无关，
// 又不会误伤字符串字面量里的空格（字面量是一个完整 token）
// [EN] normalizeCode reduces code to a space-separated token stream for structural
// comparison. Splitting on whitespace instead would keep go/printer's line breaks inside a
// chained call as `x. Foo(). Bar()`, forcing manifest authors to reproduce that odd shape;
// and it would rewrite spaces inside string literals. go/scanner avoids both, because a
// literal is a single token.
func normalizeCode(src string) string {
	fset := token.NewFileSet()
	file := fset.AddFile("", fset.Base(), len(src))
	var sc scanner.Scanner
	sc.Init(file, []byte(src), nil, 0)
	var parts []string
	for {
		_, tok, lit := sc.Scan()
		if tok == token.EOF {
			break
		}
		if lit != "" {
			parts = append(parts, lit)
			continue
		}
		parts = append(parts, tok.String())
	}
	return strings.Join(parts, " ")
}

// collapseSpace 把代码压成单行，只用于变更标签的展示，不参与等价判断
// [EN] collapseSpace flattens code onto one line for display labels only
func collapseSpace(s string) string {
	return strings.Join(strings.Fields(s), " ")
}

// canonExpr 解析表达式源码并归一化，解析失败说明补丁定义里的表达式写错了
// [EN] canonExpr parses an expression snippet and returns its normalized form
func canonExpr(snippet string) (string, error) {
	g, err := parseGo("snippet.go", []byte("package p\nvar _ = "+snippet+"\n"))
	if err != nil {
		return "", err
	}
	if len(g.ast.Decls) != 1 {
		return "", fmt.Errorf("%w: 表达式片段不合法 %q", ErrInvalidPatch, snippet)
	}
	gd, ok := g.ast.Decls[0].(*ast.GenDecl)
	if !ok || len(gd.Specs) != 1 {
		return "", fmt.Errorf("%w: 表达式片段不合法 %q", ErrInvalidPatch, snippet)
	}
	vs, ok := gd.Specs[0].(*ast.ValueSpec)
	if !ok || len(vs.Values) != 1 {
		return "", fmt.Errorf("%w: 表达式片段不合法 %q", ErrInvalidPatch, snippet)
	}
	return normalizeCode(g.print(vs.Values[0])), nil
}

// canonStmts 解析语句片段并归一化，返回每条语句的归一化文本
// [EN] canonStmts parses a statement snippet and returns the normalized form of each stmt
func canonStmts(snippet string) ([]string, error) {
	g, err := parseGo("snippet.go", []byte("package p\nfunc _() {\n"+snippet+"\n}\n"))
	if err != nil {
		return nil, err
	}
	fd, ok := g.ast.Decls[0].(*ast.FuncDecl)
	if !ok || fd.Body == nil || len(fd.Body.List) == 0 {
		return nil, fmt.Errorf("%w: 语句片段不合法 %q", ErrInvalidPatch, snippet)
	}
	out := make([]string, 0, len(fd.Body.List))
	for _, st := range fd.Body.List {
		out = append(out, normalizeCode(g.print(st)))
	}
	return out, nil
}

// canonTypeExpr 解析类型表达式源码并归一化
// 类型位置能出现 []string、map[k]v 这类不是合法「值表达式」的写法，所以不能走 canonExpr
// [EN] canonTypeExpr parses a type expression; the type position accepts forms such as
// []string that are not valid value expressions, so canonExpr cannot be reused here
func canonTypeExpr(snippet string) (string, error) {
	g, err := parseGo("snippet.go", []byte("package p\ntype _ = "+snippet+"\n"))
	if err != nil {
		return "", err
	}
	gd, ok := g.ast.Decls[0].(*ast.GenDecl)
	if !ok || len(gd.Specs) != 1 {
		return "", fmt.Errorf("%w: 类型片段不合法 %q", ErrInvalidPatch, snippet)
	}
	ts, ok := gd.Specs[0].(*ast.TypeSpec)
	if !ok {
		return "", fmt.Errorf("%w: 类型片段不合法 %q", ErrInvalidPatch, snippet)
	}
	return normalizeCode(g.print(ts.Type)), nil
}

// specNames 返回一个 spec 声明的名字
// ValueSpec 可以一行声明多个名字，TypeSpec 只有一个，ImportSpec 没有名字概念
// [EN] specNames returns the identifiers a spec declares
func specNames(s ast.Spec) []*ast.Ident {
	switch v := s.(type) {
	case *ast.ValueSpec:
		return v.Names
	case *ast.TypeSpec:
		return []*ast.Ident{v.Name}
	}
	return nil
}

// specComments 返回一个 spec 上方与行尾的注释组
// [EN] specComments returns the doc and trailing comment groups of a spec
func specComments(s ast.Spec) (doc, line *ast.CommentGroup) {
	switch v := s.(type) {
	case *ast.ValueSpec:
		return v.Doc, v.Comment
	case *ast.TypeSpec:
		return v.Doc, v.Comment
	case *ast.ImportSpec:
		return v.Doc, v.Comment
	}
	return nil, nil
}

// commentContains 判断注释组里是否出现了给定标记
// [EN] commentContains reports whether a comment group mentions the marker
func commentContains(cg *ast.CommentGroup, marker string) bool {
	if cg == nil || marker == "" {
		return false
	}
	for _, c := range cg.List {
		if strings.Contains(c.Text, marker) {
			return true
		}
	}
	return false
}

// BlockRef 定位一个括号声明块（const 块或 var 块）
// 两个锚点二选一：Marker 走块上的标记注释，Member 走块内已存在的成员名
// [EN] BlockRef locates a parenthesized declaration block, by marker comment or by member
type BlockRef struct {
	// Marker 标记注释片段，如 kronos:consts=table-name
	// [EN] Marker is a fragment of the marker comment, e.g. kronos:consts=table-name
	Marker string
	// Member 块内已经存在的任意成员名
	// [EN] Member is any identifier already declared inside the block
	Member string
}

// String 返回锚点的可读描述
// [EN] String returns a readable description of the anchor
func (r BlockRef) String() string {
	switch {
	case r.Marker != "" && r.Member != "":
		return fmt.Sprintf("marker=%q member=%q", r.Marker, r.Member)
	case r.Marker != "":
		return fmt.Sprintf("marker=%q", r.Marker)
	case r.Member != "":
		return fmt.Sprintf("member=%q", r.Member)
	default:
		return "<空锚点>"
	}
}

// empty 判断锚点是否为空
// [EN] empty reports whether the anchor is unset
func (r BlockRef) empty() bool { return r.Marker == "" && r.Member == "" }

// findDeclBlock 找到匹配锚点的括号声明块
// Marker 优先，其次 Member，两者都给时先试 Marker
// [EN] findDeclBlock locates the parenthesized declaration block matching the anchor
func (g *goFile) findDeclBlock(tok token.Token, ref BlockRef) (*ast.GenDecl, error) {
	kind := strings.ToLower(tok.String()) + " 块"
	var blocks []*ast.GenDecl
	for _, d := range g.ast.Decls {
		gd, ok := d.(*ast.GenDecl)
		if !ok || gd.Tok != tok {
			continue
		}
		blocks = append(blocks, gd)
	}
	if len(blocks) == 0 {
		return nil, locateErr(g.path, kind, fmt.Sprintf("文件里没有任何 %s 声明", tok.String()), ErrNotFound)
	}

	// 没给锚点时，文件里只有一个该种类的括号块就用它
	//
	// 这一条是为了让「内置清单」成立：内置清单不可能知道各项目已有的成员名
	// （models/table_names.go 里一个项目叫 TableNameNotifyRecord、另一个叫
	// TableNameRole），而这类文件按约定全文件只有一个 var 块
	// 有多个时仍然报错并把候选列出来 —— 这种情况下猜哪个块都是错的
	//
	// [EN] With no anchor given, a file holding exactly one parenthesized block of that
	// kind resolves to it
	// This is what makes a built-in manifest possible: it cannot know the member names a
	// given project already has, since models/table_names.go names its first entry
	// TableNameNotifyRecord in one service and TableNameRole in another, while by
	// convention such a file holds a single var block
	// Several blocks still raise an error listing the candidates, because guessing between
	// them would be wrong either way
	if ref.empty() {
		var paren []*ast.GenDecl
		for _, gd := range blocks {
			if gd.Lparen.IsValid() {
				paren = append(paren, gd)
			}
		}
		if len(paren) == 1 {
			return paren[0], nil
		}
		if len(paren) == 0 {
			return nil, locateErr(g.path, kind, fmt.Sprintf("文件里没有带括号的 %s 块，无法免锚点定位", tok.String()), ErrNotFound)
		}
		lines := make([]string, 0, len(paren))
		for _, gd := range paren {
			lines = append(lines, fmt.Sprintf("第 %d 行", g.fset.Position(gd.Pos()).Line))
		}
		return nil, locateErr(g.path, kind, fmt.Sprintf("文件里有 %d 个 %s 块（%s），必须给出 Marker 或 Member 锚点",
			len(paren), tok.String(), strings.Join(lines, "、")), ErrInvalidPatch)
	}

	if ref.Marker != "" {
		for _, gd := range blocks {
			if commentContains(gd.Doc, ref.Marker) {
				return g.requireParen(gd, kind, ref)
			}
			for _, s := range gd.Specs {
				doc, line := specComments(s)
				if commentContains(doc, ref.Marker) || commentContains(line, ref.Marker) {
					return g.requireParen(gd, kind, ref)
				}
			}
		}
		// 标记注释可能是块内的游离注释，没有挂到任何 spec 上
		// [EN] the marker may be a floating comment inside the parens, attached to nothing
		for _, cg := range g.ast.Comments {
			if !commentContains(cg, ref.Marker) {
				continue
			}
			for _, gd := range blocks {
				if gd.Lparen.IsValid() && cg.Pos() > gd.Lparen && cg.End() < gd.Rparen {
					return g.requireParen(gd, kind, ref)
				}
			}
		}
	}

	if ref.Member != "" {
		for _, gd := range blocks {
			for _, s := range gd.Specs {
				for _, n := range specNames(s) {
					if n.Name == ref.Member {
						return g.requireParen(gd, kind, ref)
					}
				}
			}
		}
	}
	return nil, locateErr(g.path, kind, fmt.Sprintf("锚点 %s 未匹配到任何 %s 块", ref, tok.String()), ErrNotFound)
}

// requireParen 要求命中的块是括号块，单行声明没有可追加的位置
// [EN] requireParen rejects a matched declaration that is not parenthesized
func (g *goFile) requireParen(gd *ast.GenDecl, kind string, ref BlockRef) (*ast.GenDecl, error) {
	if !gd.Lparen.IsValid() || !gd.Rparen.IsValid() {
		return nil, locateErr(g.path, kind,
			fmt.Sprintf("锚点 %s 命中的是单行声明而不是括号块，无法追加成员", ref), ErrNotFound)
	}
	return gd, nil
}

// declaresName 判断文件里是否已经用给定 token 声明过该名字
// [EN] declaresName reports whether the file already declares name with the given token
func (g *goFile) declaresName(tok token.Token, name string) (int, bool) {
	for _, d := range g.ast.Decls {
		gd, ok := d.(*ast.GenDecl)
		if !ok || gd.Tok != tok {
			continue
		}
		for _, s := range gd.Specs {
			for _, n := range specNames(s) {
				if n.Name == name {
					return g.fset.Position(n.Pos()).Line, true
				}
			}
		}
	}
	return 0, false
}

// lastSpecEnd 返回块内最后一个 spec 的结束偏移，空块返回 -1
// [EN] lastSpecEnd returns the end offset of the last spec, or -1 for an empty block
func (g *goFile) lastSpecEnd(gd *ast.GenDecl) int {
	if len(gd.Specs) == 0 {
		return -1
	}
	return g.off(gd.Specs[len(gd.Specs)-1].End())
}

// findStruct 找到给定名字的结构体类型声明
// [EN] findStruct locates the struct type declaration with the given name
func (g *goFile) findStruct(name string) (*ast.StructType, error) {
	if name == "" {
		return nil, locateErr(g.path, "结构体", "Struct 字段不能为空", ErrInvalidPatch)
	}
	var found *ast.TypeSpec
	for _, d := range g.ast.Decls {
		gd, ok := d.(*ast.GenDecl)
		if !ok || gd.Tok != token.TYPE {
			continue
		}
		for _, s := range gd.Specs {
			ts, ok := s.(*ast.TypeSpec)
			if ok && ts.Name.Name == name {
				found = ts
			}
		}
	}
	if found == nil {
		return nil, locateErr(g.path, "结构体", fmt.Sprintf("类型 %s 不存在", name), ErrNotFound)
	}
	st, ok := found.Type.(*ast.StructType)
	if !ok {
		return nil, locateErr(g.path, "结构体", fmt.Sprintf("类型 %s 不是结构体", name), ErrNotFound)
	}
	return st, nil
}

// findFunc 找到给定名字的函数或方法，recv 非空时还要匹配接收者类型名
// [EN] findFunc locates a function or method, optionally matching the receiver type
func (g *goFile) findFunc(name, recv string) (*ast.FuncDecl, error) {
	if name == "" {
		return nil, locateErr(g.path, "函数", "Func 字段不能为空", ErrInvalidPatch)
	}
	for _, d := range g.ast.Decls {
		fd, ok := d.(*ast.FuncDecl)
		if !ok || fd.Name.Name != name {
			continue
		}
		if recv != "" && recvTypeName(fd) != recv {
			continue
		}
		if fd.Body == nil {
			return nil, locateErr(g.path, "函数", fmt.Sprintf("函数 %s 没有函数体", name), ErrNotFound)
		}
		return fd, nil
	}
	target := name
	if recv != "" {
		target = "(" + recv + ")." + name
	}
	return nil, locateErr(g.path, "函数", fmt.Sprintf("函数 %s 不存在", target), ErrNotFound)
}

// recvTypeName 返回方法接收者的类型名，指针接收者会脱掉星号
// [EN] recvTypeName returns the receiver type name, dereferencing a pointer receiver
func recvTypeName(fd *ast.FuncDecl) string {
	if fd.Recv == nil || len(fd.Recv.List) == 0 {
		return ""
	}
	t := fd.Recv.List[0].Type
	if star, ok := t.(*ast.StarExpr); ok {
		t = star.X
	}
	if id, ok := t.(*ast.Ident); ok {
		return id.Name
	}
	if idx, ok := t.(*ast.IndexExpr); ok { // 泛型接收者 / [EN] generic receiver
		if id, ok := idx.X.(*ast.Ident); ok {
			return id.Name
		}
	}
	return ""
}

// findCompositeLit 在 root 子树里找复合字面量
// typeName 非空时按归一化后的类型文本匹配，sliceOnly 为 true 时只要切片或数组字面量
// [EN] findCompositeLit finds a composite literal under root, optionally type-matched
func (g *goFile) findCompositeLit(root ast.Node, typeName string, sliceOnly bool) *ast.CompositeLit {
	want := normalizeCode(typeName)
	var found *ast.CompositeLit
	ast.Inspect(root, func(n ast.Node) bool {
		if found != nil {
			return false
		}
		cl, ok := n.(*ast.CompositeLit)
		if !ok {
			return true
		}
		if sliceOnly {
			if _, ok := cl.Type.(*ast.ArrayType); !ok {
				return true
			}
		}
		if want != "" {
			if cl.Type == nil || normalizeCode(g.print(cl.Type)) != want {
				return true
			}
		}
		found = cl
		return false
	})
	return found
}

// innermostNodeAt 在 root 子树里找包含 pos 的最内层节点，match 决定候选类型
// 用于标记注释锚点：注释落在哪个块里，就往那个块里插
// [EN] innermostNodeAt finds the innermost matching node whose range contains pos
func innermostNodeAt(root ast.Node, pos token.Pos, match func(ast.Node) bool) ast.Node {
	var best ast.Node
	ast.Inspect(root, func(n ast.Node) bool {
		if n == nil {
			return false
		}
		// 不包含 pos 的子树直接剪掉
		// [EN] prune subtrees that do not contain pos
		if pos < n.Pos() || pos > n.End() {
			return false
		}
		if match(n) && (best == nil || n.Pos() >= best.Pos()) {
			best = n
		}
		return true
	})
	return best
}

// findMarkerComment 找到含有标记的注释，返回其位置
// [EN] findMarkerComment returns the position of the comment carrying the marker
func (g *goFile) findMarkerComment(marker string) (token.Pos, error) {
	for _, cg := range g.ast.Comments {
		for _, c := range cg.List {
			if strings.Contains(c.Text, marker) {
				return c.Pos(), nil
			}
		}
	}
	return token.NoPos, locateErr(g.path, "标记注释", fmt.Sprintf("文件里找不到标记 %q", marker), ErrNotFound)
}

// findBlockContainingStmt 找到包含给定语句的最内层块，同时返回被命中的那条语句
// 生产代码里往往没有标记注释，用「块里已经有的某条语句」当锚点就不必先去改目标文件
// 返回被命中的语句是为了支持「紧跟它之后插入」，而不是只能追加到块尾
// [EN] findBlockContainingStmt locates the innermost block holding the given statement and
// returns the matched statement as well, so a caller can insert right after it instead of
// only appending at the end of the block
func (g *goFile) findBlockContainingStmt(want string) (*ast.BlockStmt, ast.Stmt) {
	var found *ast.BlockStmt
	var at ast.Stmt
	ast.Inspect(g.ast, func(n ast.Node) bool {
		if found != nil {
			return false
		}
		block, ok := n.(*ast.BlockStmt)
		if !ok {
			return true
		}
		for _, st := range block.List {
			if normalizeCode(g.print(st)) == want {
				found, at = block, st
				return false
			}
		}
		return true
	})
	return found, at
}

// findLitContainingElem 找到包含给定元素的最内层复合字面量
// [EN] findLitContainingElem locates the innermost composite literal holding the element
func (g *goFile) findLitContainingElem(want string) *ast.CompositeLit {
	var found *ast.CompositeLit
	ast.Inspect(g.ast, func(n ast.Node) bool {
		if found != nil {
			return false
		}
		cl, ok := n.(*ast.CompositeLit)
		if !ok {
			return true
		}
		for _, el := range cl.Elts {
			if normalizeCode(g.print(el)) == want {
				found = cl
				return false
			}
		}
		return true
	})
	return found
}
