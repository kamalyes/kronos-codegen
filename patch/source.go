/*
 * @Author: kamalyes 501893067@qq.com
 * @Date: 2026-11-01 16:20:11
 * @LastEditors: kamalyes 501893067@qq.com
 * @LastEditTime: 2026-11-01 20:10:35
 * @FilePath: \kronos-codegen\patch\source.go
 * @Description: 字节级文本工具 - 行定位、缩进推导、块尾追加与插入点落地
 *
 * Copyright (c) 2026 by kamalyes, All Rights Reserved.
 */
package patch

import (
	"bytes"
	"sort"
	"strings"
)

// clampOffset 把偏移限制在源码范围内
// [EN] clampOffset keeps an offset inside the source bounds
func clampOffset(src []byte, off int) int {
	if off < 0 {
		return 0
	}
	if off > len(src) {
		return len(src)
	}
	return off
}

// lineStartOffset 返回 off 所在行的行首偏移
// [EN] lineStartOffset returns the offset of the first byte of the line containing off
func lineStartOffset(src []byte, off int) int {
	off = clampOffset(src, off)
	return bytes.LastIndexByte(src[:off], '\n') + 1
}

// lineEndOffset 返回 off 所在行换行符的偏移，最后一行没有换行符时返回 len(src)
// [EN] lineEndOffset returns the offset of the newline ending off's line, or len(src)
func lineEndOffset(src []byte, off int) int {
	off = clampOffset(src, off)
	if i := bytes.IndexByte(src[off:], '\n'); i >= 0 {
		return off + i
	}
	return len(src)
}

// hasBlankLineBetween 判断两个偏移之间是否存在空行
// import 块的分组就是用空行分隔的，这是判定组边界的唯一依据
// [EN] hasBlankLineBetween reports whether a blank line separates the two offsets, which
// is exactly how an import block marks its group boundaries
func hasBlankLineBetween(src []byte, a, b int) bool {
	a, b = clampOffset(src, a), clampOffset(src, b)
	if a >= b {
		return false
	}
	seg := src[a:b]
	// 先跳过 a 所在行的剩余部分，它不算「中间行」
	// [EN] skip the remainder of a's own line; it is not an in-between line
	first := bytes.IndexByte(seg, '\n')
	if first < 0 {
		return false
	}
	rest := seg[first+1:]
	for {
		i := bytes.IndexByte(rest, '\n')
		if i < 0 {
			return false
		}
		if onlySpace(rest[:i]) {
			return true
		}
		rest = rest[i+1:]
	}
}

// insertAfterLine 在 afterOff 所在行之后插入若干整行，缩进取 indentOff 所在行的缩进
//
// 两个偏移要分开给：多行链式语句的结束偏移落在最后一个续行上（缩进更深），
// 新语句应当与整条语句的**起始行**对齐，而不是与续行对齐
// [EN] insertAfterLine inserts whole lines after afterOff's line, taking the indentation
// from indentOff's line. The two differ for a multi-line chained statement, whose end sits
// on a more deeply indented continuation line while the new statement must align with the
// statement's first line.
func insertAfterLine(src []byte, afterOff, indentOff int, lines []string) []Insertion {
	if len(lines) == 0 {
		return nil
	}
	anchorOff := afterOff
	indent := indentAt(src, indentOff)
	var body strings.Builder
	for _, ln := range lines {
		body.WriteString(indent)
		body.WriteString(ln)
		body.WriteByte('\n')
	}
	end := lineEndOffset(src, anchorOff)
	if end >= len(src) {
		// 文件最后一行没有换行符，先补一个
		// [EN] the last line has no newline, so add one first
		return []Insertion{{Offset: len(src), Text: "\n" + strings.TrimSuffix(body.String(), "\n")}}
	}
	return []Insertion{{Offset: end + 1, Text: body.String()}}
}

// lineNumber 返回 off 所在的行号，从 1 开始
// [EN] lineNumber returns the 1-based line number of off
func lineNumber(src []byte, off int) int {
	off = clampOffset(src, off)
	return bytes.Count(src[:off], []byte{'\n'}) + 1
}

// indentAt 返回 off 所在行的前导空白，用于让插入的新行与同级成员对齐
// [EN] indentAt returns the leading whitespace of the line containing off
func indentAt(src []byte, off int) string {
	ls := lineStartOffset(src, off)
	i := ls
	for i < len(src) && (src[i] == ' ' || src[i] == '\t') {
		i++
	}
	return string(src[ls:i])
}

// onlySpace 判断片段是否只含空白
// [EN] onlySpace reports whether the slice contains whitespace only
func onlySpace(b []byte) bool {
	for _, c := range b {
		if c != ' ' && c != '\t' && c != '\r' {
			return false
		}
	}
	return true
}

// nextNonSpaceByte 返回 off 之后第一个非空白字节，没有则返回 0
// [EN] nextNonSpaceByte returns the first non-whitespace byte after off, or 0
func nextNonSpaceByte(src []byte, off int) byte {
	for i := clampOffset(src, off); i < len(src); i++ {
		switch src[i] {
		case ' ', '\t', '\r', '\n':
			continue
		default:
			return src[i]
		}
	}
	return 0
}

// prevNonBlankLineStart 从 from 所在行往上找第一个非空行，返回其行首偏移，找不到返回 -1
// 空块里往往只剩一行 TODO 注释，沿用它的缩进比凭空加一个 tab 更贴近原风格
// [EN] prevNonBlankLineStart walks upwards for the closest non-blank line start, or -1
func prevNonBlankLineStart(src []byte, from int) int {
	ls := lineStartOffset(src, from)
	for ls > 0 {
		end := ls - 1 // 上一行的换行符位置 / [EN] newline of the previous line
		ls = lineStartOffset(src, end)
		if !onlySpace(src[ls:end]) {
			return ls
		}
		if ls == 0 {
			break
		}
	}
	return -1
}

// buildAppend 在一个块的闭合符号前追加若干行
//
// openerEnd 是开始符号之后的偏移，closerOff 是闭合符号本身的偏移
// lastEnd 是块内最后一个成员的结束偏移，空块传 -1
// commaSep 为 true 时，若上一个成员后面缺逗号会补一个（JSON 与复合字面量需要）
// [EN] buildAppend appends lines just before a block's closing token, deriving the member
// indentation from the existing members and adding the missing separator comma if needed
func buildAppend(src []byte, openerEnd, closerOff, lastEnd int, commaSep bool, lines []string) []Insertion {
	return buildAppendIndent(src, openerEnd, closerOff, lastEnd, commaSep, "\t", lines)
}

// buildAppendIndent 与 buildAppend 相同，额外允许指定空块时的缩进单位
// JSON 文件普遍用两个空格，硬塞一个 tab 会和原风格打架
// [EN] buildAppendIndent is buildAppend with an explicit indent unit for empty blocks
func buildAppendIndent(src []byte, openerEnd, closerOff, lastEnd int, commaSep bool, unit string, lines []string) []Insertion {
	if len(lines) == 0 {
		return nil
	}
	baseIndent := indentAt(src, openerEnd)
	childIndent := baseIndent + unit
	switch {
	case lastEnd >= 0 && lineStartOffset(src, lastEnd) != lineStartOffset(src, openerEnd):
		// 最后一个成员独占一行，新行跟它对齐
		// [EN] the last member owns its line, so align the new line with it
		childIndent = indentAt(src, lastEnd)
	case lastEnd >= 0:
		// 成员和开始符号同行（单行块），新行按基准缩进加一级
		// [EN] single-line block: indent the new line one level below the opener
	default:
		// 空块：参考闭合行上方最近的非空行（通常是 TODO 注释）
		// [EN] empty block: follow the closest non-blank line above the closing line
		if p := prevNonBlankLineStart(src, closerOff); p > openerEnd {
			childIndent = indentAt(src, p)
		}
	}

	var body strings.Builder
	for _, ln := range lines {
		body.WriteString(childIndent)
		body.WriteString(ln)
		body.WriteByte('\n')
	}

	needComma := commaSep && lastEnd >= 0 && nextNonSpaceByte(src, lastEnd) != ','
	ls := lineStartOffset(src, closerOff)
	if ls > openerEnd && onlySpace(src[ls:closerOff]) {
		// 常规情形：闭合符独占一行，直接在这一行之前插入新行
		// [EN] common case: the closing token owns its line, insert whole lines before it
		var ins []Insertion
		if needComma {
			ins = append(ins, Insertion{Offset: lastEnd, Text: ","})
		}
		return append(ins, Insertion{Offset: ls, Text: body.String()})
	}

	// 单行块：把闭合符顶到新行上，保持语法合法
	// [EN] single-line block: push the closing token onto a new line to stay valid
	anchor := lastEnd
	if anchor < 0 {
		anchor = openerEnd
	}
	text := "\n" + body.String() + baseIndent
	if needComma {
		text = "," + text
	}
	return []Insertion{{Offset: anchor, Text: text}}
}

// bodyInsertion 返回插入计划里真正承载新内容的那个插入点
// 补逗号的插入点不算，行号要取新内容所在位置
// [EN] bodyInsertion returns the insertion that carries the new content
func bodyInsertion(ins []Insertion) Insertion {
	if len(ins) == 0 {
		return Insertion{}
	}
	return ins[len(ins)-1]
}

// applyInsertions 按偏移升序一次性把所有插入点拼接到源码上
// 同一偏移上的多个插入按给定顺序落地
// [EN] applyInsertions splices every insertion onto src in a single ascending pass
func applyInsertions(src []byte, ins []Insertion) []byte {
	if len(ins) == 0 {
		return src
	}
	ordered := make([]Insertion, len(ins))
	copy(ordered, ins)
	sort.SliceStable(ordered, func(i, j int) bool { return ordered[i].Offset < ordered[j].Offset })

	var b bytes.Buffer
	b.Grow(len(src) + 64*len(ordered))
	prev := 0
	for _, in := range ordered {
		off := clampOffset(src, in.Offset)
		if off < prev {
			off = prev
		}
		b.Write(src[prev:off])
		b.WriteString(in.Text)
		prev = off
	}
	b.Write(src[prev:])
	return b.Bytes()
}

// docLines 把多行注释文本规整成以 // 开头的注释行
// [EN] docLines turns free-form doc text into // comment lines
func docLines(doc string) []string {
	if strings.TrimSpace(doc) == "" {
		return nil
	}
	var out []string
	for _, ln := range strings.Split(doc, "\n") {
		ln = strings.TrimSpace(ln)
		if ln == "" {
			out = append(out, "//")
			continue
		}
		if strings.HasPrefix(ln, "//") {
			out = append(out, ln)
			continue
		}
		out = append(out, "// "+ln)
	}
	return out
}

// dedentLines 把一段可能带缩进的多行代码拆行并去掉公共前导空白
// 插入时每行会重新加上目标块的缩进，相对层级因此得以保留
// [EN] dedentLines splits a snippet into lines and strips the common leading whitespace
func dedentLines(snippet string) []string {
	raw := strings.Split(strings.Trim(snippet, "\n"), "\n")
	prefix := ""
	first := true
	for _, ln := range raw {
		if strings.TrimSpace(ln) == "" {
			continue
		}
		lead := ln[:len(ln)-len(strings.TrimLeft(ln, " \t"))]
		if first {
			prefix = lead
			first = false
			continue
		}
		prefix = commonPrefix(prefix, lead)
	}
	out := make([]string, 0, len(raw))
	for _, ln := range raw {
		out = append(out, strings.TrimRight(strings.TrimPrefix(ln, prefix), " \t\r"))
	}
	return out
}

// commonPrefix 返回两个字符串的公共前缀
// [EN] commonPrefix returns the common prefix of two strings
func commonPrefix(a, b string) string {
	n := len(a)
	if len(b) < n {
		n = len(b)
	}
	i := 0
	for i < n && a[i] == b[i] {
		i++
	}
	return a[:i]
}
