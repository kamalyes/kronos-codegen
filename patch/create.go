/*
 * @Author: kamalyes 501893067@qq.com
 * @Date: 2026-11-01 16:20:11
 * @LastEditors: kamalyes 501893067@qq.com
 * @LastEditTime: 2026-11-01 20:10:35
 * @FilePath: \kronos-codegen\patch\create.go
 * @Description: 新建文件 - 已存在则跳过绝不覆盖，Go 文件一律格式化，格式化失败就放弃写入
 *
 * Copyright (c) 2026 by kamalyes, All Rights Reserved.
 */
package patch

import (
	"bytes"
	"fmt"
	"go/format"
	"strings"
)

// previewLineCount 新建文件在变更预览里展示的行数上限
// 新文件动辄两三百行，全量刷屏会把真正要看的插入点淹掉
// [EN] previewLineCount caps how many lines of a new file the preview shows, since a new
// file easily runs to hundreds of lines and would drown the insertions that matter
const previewLineCount = 8

// Creator 新建文件的一步
//
// 与 Patch 的区别是它不要求目标文件已经存在：Render 直接给出整个文件的内容
// 渲染（含格式化）全部发生在 Render 里，也就是在任何磁盘写入之前，
// 所以渲染结果不合法时不会留下半个坏文件
// [EN] Creator creates a new file. Unlike Patch it needs no pre-existing target: Render
// yields the whole content. Rendering and formatting both happen inside Render, before any
// disk write, so an invalid result never lands on disk.
type Creator interface {
	// File 返回要新建的文件路径
	// [EN] File returns the path of the file to create
	File() string
	// Label 返回人类可读的变更标签
	// [EN] Label returns the human readable change label
	Label() string
	// Render 返回新文件的完整内容
	// [EN] Render returns the full content of the new file
	Render() ([]byte, error)
}

// Action 一次运行里的一步，要么新建文件（Creator），要么往已有文件插入条目（Patch）
// Engine.Run 按给定顺序执行，任一步失败整体中止
// [EN] Action is one step of a run: either a Creator or a Patch. Engine.Run executes them
// in order and aborts the whole run on the first failure.
type Action interface {
	// File 返回这一步作用的文件路径
	// [EN] File returns the path this step acts on
	File() string
	// Label 返回人类可读的变更标签
	// [EN] Label returns the human readable change label
	Label() string
}

// FileCreate 新建一个文件，内容由调用方给出（通常是模板渲染的结果）
//
// 两条硬规则：
//  1. 目标文件已存在就跳过，**绝不覆盖**，与 JSON 不覆盖已有译文同一取向
//  2. Go 文件一律 gofmt，不走 FormatPolicy 的条件判断——新文件是我们自己生成的，
//     没有历史排版要保护；格式化失败直接报错,宁可不写也不留一个不合法的 .go
//
// [EN] FileCreate creates one file from caller-supplied content, usually a rendered
// template. Two hard rules: an existing target is skipped and never overwritten, and Go
// files are always gofmt'ed (no FormatPolicy condition, because a brand new file has no
// historical layout to protect) with a hard failure rather than an invalid .go on disk.
type FileCreate struct {
	// Path 要新建的文件相对路径
	// [EN] Path is the relative path of the file to create
	Path string
	// Content 文件的完整内容
	// [EN] Content is the full file content
	Content []byte
	// Origin 内容来源的可读描述，如模板路径，只用于变更标签
	// [EN] Origin describes where the content came from, used only in the label
	Origin string
	// NoFormat 为 true 时跳过 gofmt，非 Go 文件本来就会跳过
	// [EN] NoFormat skips gofmt; non-Go files skip it anyway
	NoFormat bool
}

// File 实现 Creator 与 Action
// [EN] File implements Creator and Action
func (c *FileCreate) File() string { return c.Path }

// Label 实现 Creator 与 Action
// [EN] Label implements Creator and Action
func (c *FileCreate) Label() string {
	if c.Origin != "" {
		return fmt.Sprintf("新建 %s <- %s", c.Path, c.Origin)
	}
	return fmt.Sprintf("新建 %s", c.Path)
}

// Render 返回新文件的内容，Go 文件会先过一遍 gofmt
// 格式化失败说明模板渲染出了不合法的 Go 源码，这时报错而不是把坏文件写出去
// [EN] Render returns the content, running gofmt over Go files first; a failure means the
// template rendered invalid Go, so it errors out instead of writing a broken file
func (c *FileCreate) Render() ([]byte, error) {
	if c.Path == "" {
		return nil, locateErr("<空路径>", "新建文件", "Path 不能为空", ErrInvalidPatch)
	}
	if len(bytes.TrimSpace(c.Content)) == 0 {
		return nil, locateErr(c.Path, "新建文件", "渲染结果是空内容，已放弃写入", ErrInvalidPatch)
	}
	content := c.Content
	if !c.NoFormat && isGoFile(c.Path) {
		out, err := format.Source(content)
		if err != nil {
			return nil, locateErr(c.Path, "新建文件",
				fmt.Sprintf("渲染结果不是合法 Go 源码，已放弃写入: %v", err), ErrFormat)
		}
		content = out
	}
	if !bytes.HasSuffix(content, []byte{'\n'}) {
		content = append(append([]byte(nil), content...), '\n')
	}
	return content, nil
}

// countLines 统计内容的行数，末尾换行不额外算一行
// [EN] countLines counts lines, not counting the trailing newline as an extra line
func countLines(content []byte) int {
	if len(content) == 0 {
		return 0
	}
	n := bytes.Count(content, []byte{'\n'})
	if !bytes.HasSuffix(content, []byte{'\n'}) {
		n++
	}
	return n
}

// previewLines 取内容的前 n 行，截断时补一行省略提示
// [EN] previewLines takes the first n lines, adding an elision note when truncated
func previewLines(content []byte, n int) string {
	lines := splitLines(content)
	if len(lines) <= n {
		return strings.Join(lines, "\n")
	}
	kept := append([]string(nil), lines[:n]...)
	kept = append(kept, fmt.Sprintf("……（省略 %d 行）", len(lines)-n))
	return strings.Join(kept, "\n")
}

// createPreview 渲染新建文件的变更预览
// 不是可用的 patch，只是给人看的：路径 + 行数 + 前若干行
// [EN] createPreview renders the preview of a new file: path, line count and first lines.
// It is deliberately not an appliable patch, only something to read.
func createPreview(path string, content []byte, n int) string {
	total := countLines(content)
	var b strings.Builder
	fmt.Fprintf(&b, "--- /dev/null\n+++ b/%s\n", path)
	shown := total
	if shown > n {
		shown = n
	}
	fmt.Fprintf(&b, "@@ -0,0 +1,%d @@ 新建文件，预览前 %d 行\n", total, shown)
	for _, ln := range splitLines(content)[:shown] {
		b.WriteString("+")
		b.WriteString(ln)
		b.WriteString("\n")
	}
	if total > shown {
		fmt.Fprintf(&b, "+……（省略 %d 行）\n", total-shown)
	}
	return b.String()
}
