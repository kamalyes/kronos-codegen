/*
 * @Author: kamalyes 501893067@qq.com
 * @Date: 2026-11-01 16:20:11
 * @LastEditors: kamalyes 501893067@qq.com
 * @LastEditTime: 2026-11-01 20:10:35
 * @FilePath: \kronos-codegen\patch\patch.go
 * @Description: 幂等补丁引擎核心类型 - 补丁接口、插入计划、错误类型与格式化策略
 *
 * Copyright (c) 2026 by kamalyes, All Rights Reserved.
 */

// Package patch 实现「AST 定位 + 文本拼接」的幂等补丁引擎
//
// 它解决的是代码生成里最烦的那一半：新增一个实体，除了生成新文件，还要往一堆**已经存在**的
// 文件里插入条目——表名常量、工厂字段与对应的初始化项、迁移的模型清单、服务注册语句、
// 跨包别名、几十个 locale 文件的键，这些插入必须幂等（重复执行不产生重复条目），
// 而且不能把目标文件的排版与注释搅乱
//
// 用法 / Usage:
//
//	eng := patch.NewEngine(patch.NewDirStore("/path/to/project"))
//	eng.SetFormatPolicy(patch.FormatIfClean)
//	_, err := eng.Run([]patch.Action{
//		&patch.FileCreate{
//			// 已存在就跳过，绝不覆盖；Go 文件一律 gofmt，不合法就报错不写盘
//			// [EN] skipped if present, never overwritten; Go files are always gofmt'ed
//			Path:    "models/order_model.go",
//			Content: rendered,
//		},
//		&patch.DeclInsert{
//			Path:  "constants/table.go",
//			Decl:  patch.DeclConst,
//			Block: patch.BlockRef{Member: "TableNameExample"},
//			Name:  "TableNameOrder",
//			Value: `"orders"`,
//		},
//		&patch.ImportInsert{
//			Path:   "service/exports.go",
//			Import: "example.com/app/service/order",
//			Alias:  "ordersvc",
//			Near:   "example.com/app/service/notify",
//		},
//	})
//	if err != nil {
//		return err // 任何一条失败都整批中止，磁盘上一个字节都没动
//	}
//	fmt.Print(eng.Summary(), eng.Diff()) // dry-run 预览
//	written, err := eng.Write()          // 确认后一次性落盘
//
// 七类插入：DeclInsert（const / var / type 括号块）、ImportInsert（import 块）、
// StructFieldInsert（struct 字段）、CompositeFieldInsert（复合字面量初始化项）、
// SliceAppend（切片元素）、StmtInsert（函数体或闭包体里的语句）、JSONKeyInsert（JSON 键）
//
// 新建文件走 FileCreate（实现 Creator），与上面七类插入共用 Action 接口与 Engine.Run，
// 因此「建文件 + 接线」是一个事务：任一步失败整体中止，磁盘上一个字节都没动
// [EN] New files go through FileCreate (a Creator), sharing the Action interface and
// Engine.Run with the insertions above, so creating and wiring form one transaction
//
// 设计要点：
//  1. go/parser 只负责算出插入点的字节偏移，真正的修改是在原始字节上做切片拼接
//     因此除插入点之外的每一个字节都保持原样，文件头的版权注释块、对齐、空行都不会被动
//  2. 不走 go/printer 整文件回写：printer 会重排声明间距、丢失部分游离注释的位置
//     产生巨量 diff 噪音，对「往已存在文件里插一条」这种场景是负收益
//  3. 幂等判定一律走 AST 查询（声明名、KeyValueExpr 的 key、归一化后的语句文本）
//     不用字符串 contains，避免注释或字符串字面量里的同名文本造成误判
//
// [EN] Package patch implements an idempotent patch engine built on "locate with the AST,
// splice with text". It handles the annoying half of code generation: besides writing new
// files, adding an entity means inserting entries into files that already exist, and those
// insertions must be idempotent without disturbing the target's layout or comments.
//
// go/parser only yields byte offsets; the edit itself is a byte-level splice, so every byte
// outside the insertion points is preserved verbatim. Whole-file go/printer round-tripping
// is deliberately avoided because it reflows declarations and drops floating comment
// positions. Idempotency is always decided by AST queries, never by substring matching.
package patch

import (
	"errors"
	"fmt"
	"strings"
)

// FormatPolicy 控制补丁落地后是否对整个 Go 文件做 gofmt 归一化
// 插入一个比现有成员更长的名字会打破 const 块或 struct 的列对齐
// 归一化可以把对齐补回来，代价是 diff 会多出同块内的对齐变化
// [EN] FormatPolicy decides whether a whole-file gofmt normalization runs after splicing
type FormatPolicy int

const (
	// FormatIfClean 仅当原文件本身已经是 gofmt 干净的才归一化（默认）
	// 手工排版、带模板占位符或本来就不规范的文件保持逐字节不变
	// [EN] FormatIfClean normalizes only when the original file is already gofmt-clean
	FormatIfClean FormatPolicy = iota
	// FormatNever 永不归一化，除插入点外逐字节不变
	// [EN] FormatNever never normalizes; bytes outside insertion points stay identical
	FormatNever
	// FormatAlways 总是归一化，文件不是合法 Go 源码时直接报错
	// [EN] FormatAlways always normalizes and fails when the file is not valid Go
	FormatAlways
)

// String 返回策略的可读名字
// [EN] String returns the human readable policy name
func (p FormatPolicy) String() string {
	switch p {
	case FormatNever:
		return "never"
	case FormatAlways:
		return "always"
	default:
		return "if-clean"
	}
}

// ParseFormatPolicy 解析命令行传入的格式化策略名
// [EN] ParseFormatPolicy parses the policy name coming from the command line
func ParseFormatPolicy(s string) (FormatPolicy, error) {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "", "if-clean", "ifclean":
		return FormatIfClean, nil
	case "never", "off", "none":
		return FormatNever, nil
	case "always", "on":
		return FormatAlways, nil
	default:
		return FormatIfClean, fmt.Errorf("%w: 未知的格式化策略 %q，可选 if-clean|never|always", ErrInvalidPatch, s)
	}
}

// Insertion 一次纯插入：在 Offset 处插入 Text，不删除任何原有字节
// [EN] Insertion is a pure insertion of Text at Offset; no original byte is removed
type Insertion struct {
	// Offset 插入点的字节偏移
	// [EN] Offset is the byte offset of the insertion point
	Offset int
	// Text 待插入文本
	// [EN] Text is the text to insert
	Text string
}

// Plan 一条补丁在某份源码上的执行计划
// Present 为 true 时表示条目已存在，本次是 no-op，Insertions 为空
// [EN] Plan is the execution plan of one patch against one source buffer
type Plan struct {
	// File 目标文件的相对路径
	// [EN] File is the target file path, relative to the engine root
	File string
	// Label 人类可读的变更标签，用于 dry-run 预览
	// [EN] Label is the human readable change label used by the dry-run preview
	Label string
	// Present 条目已存在，幂等命中
	// [EN] Present reports that the entry already exists, so this patch is a no-op
	Present bool
	// Line 插入点在原文件中的行号，从 1 开始，Present 时为已存在条目所在行
	// [EN] Line is the 1-based line number of the insertion point in the original file
	Line int
	// Snippet 实际插入的代码片段，已去掉缩进
	// [EN] Snippet is the inserted code, without the leading indentation
	Snippet string
	// Insertions 插入点列表，按偏移升序应用
	// [EN] Insertions is the list of insertion points, applied in ascending offset order
	Insertions []Insertion
	// Created 这一步是新建文件而不是往已有文件插入条目
	// [EN] Created marks a step that creates a file rather than patching one
	Created bool
	// Lines 新建文件的总行数，Created 为 false 时无意义
	// [EN] Lines is the new file's total line count, meaningless unless Created
	Lines int
}

// NoOp 判断该计划是否不产生任何字节变化
// [EN] NoOp reports whether the plan changes nothing
func (p *Plan) NoOp() bool {
	if p == nil || p.Present {
		return true
	}
	return !p.Created && len(p.Insertions) == 0
}

// Patch 一条幂等补丁
// 实现者只做两件事：在源码里定位插入点，判断条目是否已存在
// [EN] Patch is one idempotent patch: locate the insertion point and detect prior existence
type Patch interface {
	// File 返回目标文件的相对路径
	// [EN] File returns the target file path
	File() string
	// Label 返回人类可读的变更标签
	// [EN] Label returns the human readable change label
	Label() string
	// Plan 在给定源码上计算插入计划，不做任何写入
	// [EN] Plan computes the insertion plan against src without writing anything
	Plan(src []byte) (*Plan, error)
}

// 哨兵错误，调用方用 errors.Is 判断大类
// [EN] Sentinel errors; callers match them with errors.Is
var (
	// ErrInvalidPatch 补丁定义本身不合法，例如缺少名字或锚点
	// [EN] ErrInvalidPatch means the patch definition itself is invalid
	ErrInvalidPatch = errors.New("patch: invalid patch definition")
	// ErrNotFound 目标锚点在文件里没找到，例如 const 块或函数不存在
	// [EN] ErrNotFound means the anchor could not be located in the file
	ErrNotFound = errors.New("patch: target anchor not found")
	// ErrParse 目标文件不是合法的 Go 源码或合法 JSON
	// [EN] ErrParse means the target file is not valid Go source or valid JSON
	ErrParse = errors.New("patch: source parse failed")
	// ErrFormat 归一化阶段失败
	// [EN] ErrFormat means the gofmt normalization step failed
	ErrFormat = errors.New("patch: gofmt normalization failed")
	// ErrConflict 条目已存在但内容与期望不一致
	// [EN] ErrConflict means the entry exists with a different value than expected
	ErrConflict = errors.New("patch: existing entry conflicts")
)

// LocateError 定位失败的详细错误，带上文件与锚点信息方便排查
// [EN] LocateError carries the file and anchor details of a location failure
type LocateError struct {
	// File 目标文件
	// [EN] File is the target file
	File string
	// Target 要定位的东西，如「const 块」「函数体」
	// [EN] Target is what was being located, e.g. a const block or a function body
	Target string
	// Detail 具体原因
	// [EN] Detail is the concrete reason
	Detail string
	// Err 归类用的哨兵错误
	// [EN] Err is the sentinel error this failure belongs to
	Err error
}

// Error 实现 error 接口
// [EN] Error implements the error interface
func (e *LocateError) Error() string {
	if e.Detail == "" {
		return fmt.Sprintf("%s: 定位 %s 失败 (%v)", e.File, e.Target, e.Err)
	}
	return fmt.Sprintf("%s: 定位 %s 失败: %s (%v)", e.File, e.Target, e.Detail, e.Err)
}

// Unwrap 暴露底层哨兵错误
// [EN] Unwrap exposes the underlying sentinel error
func (e *LocateError) Unwrap() error { return e.Err }

// locateErr 构造一个 LocateError
// [EN] locateErr builds a LocateError
func locateErr(file, target, detail string, err error) error {
	return &LocateError{File: file, Target: target, Detail: detail, Err: err}
}
