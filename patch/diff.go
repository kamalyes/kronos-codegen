/*
 * @Author: kamalyes 501893067@qq.com
 * @Date: 2026-11-01 16:20:11
 * @LastEditors: kamalyes 501893067@qq.com
 * @LastEditTime: 2026-11-01 20:10:35
 * @FilePath: \kronos-codegen\patch\diff.go
 * @Description: dry-run 用的 unified diff 渲染 - 先剪公共前后缀再做 LCS，只依赖标准库
 *
 * Copyright (c) 2026 by kamalyes, All Rights Reserved.
 */
package patch

import (
	"fmt"
	"strings"
)

// maxDPCells LCS 动态规划的规模上限，超过就退化成整段替换
// 补丁引擎只做插入，剪掉公共前后缀之后差异窗口通常只有几行，这个上限基本碰不到
// [EN] maxDPCells caps the LCS table size; beyond it the window degrades to replace-all
const maxDPCells = 4_000_000

// diffOp 一条 diff 操作，kind 取 -1 删除、0 相同、1 新增
// [EN] diffOp is one diff operation: -1 delete, 0 equal, 1 insert
type diffOp struct {
	kind int
	text string
}

// splitLines 按行切分，末尾换行不产生空行
// [EN] splitLines splits into lines without producing a trailing empty line
func splitLines(b []byte) []string {
	if len(b) == 0 {
		return nil
	}
	s := string(b)
	hadNewline := strings.HasSuffix(s, "\n")
	if hadNewline {
		s = s[:len(s)-1]
	}
	return strings.Split(s, "\n")
}

// UnifiedDiff 渲染 before 到 after 的 unified diff，ctx 是上下文行数
// 返回空串表示两者一致
// [EN] UnifiedDiff renders a unified diff from before to after with ctx context lines
func UnifiedDiff(path string, before, after []byte, ctx int) string {
	a, b := splitLines(before), splitLines(after)
	ops := diffLines(a, b)
	body := renderHunks(ops, ctx)
	if body == "" {
		return ""
	}
	var out strings.Builder
	fmt.Fprintf(&out, "--- a/%s\n+++ b/%s\n", path, path)
	out.WriteString(body)
	return out.String()
}

// diffLines 先剪掉公共前后缀，再对中间窗口做 LCS
// [EN] diffLines trims the common prefix and suffix, then runs LCS on the middle window
func diffLines(a, b []string) []diffOp {
	p := 0
	for p < len(a) && p < len(b) && a[p] == b[p] {
		p++
	}
	s := 0
	for s < len(a)-p && s < len(b)-p && a[len(a)-1-s] == b[len(b)-1-s] {
		s++
	}
	ops := make([]diffOp, 0, len(a)+len(b))
	for _, l := range a[:p] {
		ops = append(ops, diffOp{0, l})
	}
	ops = append(ops, lcsOps(a[p:len(a)-s], b[p:len(b)-s])...)
	for _, l := range a[len(a)-s:] {
		ops = append(ops, diffOp{0, l})
	}
	return ops
}

// lcsOps 用最长公共子序列算出窗口内的 diff 操作序列
// [EN] lcsOps derives the diff operations of a window via longest common subsequence
func lcsOps(a, b []string) []diffOp {
	var ops []diffOp
	switch {
	case len(a) == 0 && len(b) == 0:
		return nil
	case len(a) == 0 || len(b) == 0 || len(a)*len(b) > maxDPCells:
		for _, l := range a {
			ops = append(ops, diffOp{-1, l})
		}
		for _, l := range b {
			ops = append(ops, diffOp{1, l})
		}
		return ops
	}
	n, m := len(a), len(b)
	dp := make([]int, (n+1)*(m+1))
	at := func(i, j int) int { return i*(m+1) + j }
	for i := n - 1; i >= 0; i-- {
		for j := m - 1; j >= 0; j-- {
			switch {
			case a[i] == b[j]:
				dp[at(i, j)] = dp[at(i+1, j+1)] + 1
			case dp[at(i+1, j)] >= dp[at(i, j+1)]:
				dp[at(i, j)] = dp[at(i+1, j)]
			default:
				dp[at(i, j)] = dp[at(i, j+1)]
			}
		}
	}
	i, j := 0, 0
	for i < n && j < m {
		switch {
		case a[i] == b[j]:
			ops = append(ops, diffOp{0, a[i]})
			i++
			j++
		case dp[at(i+1, j)] >= dp[at(i, j+1)]:
			ops = append(ops, diffOp{-1, a[i]})
			i++
		default:
			ops = append(ops, diffOp{1, b[j]})
			j++
		}
	}
	for ; i < n; i++ {
		ops = append(ops, diffOp{-1, a[i]})
	}
	for ; j < m; j++ {
		ops = append(ops, diffOp{1, b[j]})
	}
	return ops
}

// renderHunks 把操作序列分组成带上下文的 hunk
// [EN] renderHunks groups the operation stream into hunks with context lines
func renderHunks(ops []diffOp, ctx int) string {
	if ctx < 0 {
		ctx = 0
	}
	var changed []int
	for i, op := range ops {
		if op.kind != 0 {
			changed = append(changed, i)
		}
	}
	if len(changed) == 0 {
		return ""
	}
	// 前缀计数：ops[0:i) 消耗掉的 a 行数与 b 行数
	// [EN] prefix counts: how many a-lines and b-lines ops[0:i) consumes
	aPrefix := make([]int, len(ops)+1)
	bPrefix := make([]int, len(ops)+1)
	for i, op := range ops {
		aPrefix[i+1] = aPrefix[i]
		bPrefix[i+1] = bPrefix[i]
		if op.kind <= 0 {
			aPrefix[i+1]++
		}
		if op.kind >= 0 {
			bPrefix[i+1]++
		}
	}

	var out strings.Builder
	for k := 0; k < len(changed); {
		lo := changed[k]
		hi := changed[k]
		k++
		for k < len(changed) && changed[k]-hi <= 2*ctx+1 {
			hi = changed[k]
			k++
		}
		if lo-ctx > 0 {
			lo -= ctx
		} else {
			lo = 0
		}
		if hi+ctx < len(ops)-1 {
			hi += ctx
		} else {
			hi = len(ops) - 1
		}
		aCount := aPrefix[hi+1] - aPrefix[lo]
		bCount := bPrefix[hi+1] - bPrefix[lo]
		aStart, bStart := aPrefix[lo], bPrefix[lo]
		if aCount > 0 {
			aStart++
		}
		if bCount > 0 {
			bStart++
		}
		fmt.Fprintf(&out, "@@ -%d,%d +%d,%d @@\n", aStart, aCount, bStart, bCount)
		for _, op := range ops[lo : hi+1] {
			switch op.kind {
			case -1:
				out.WriteString("-")
			case 1:
				out.WriteString("+")
			default:
				out.WriteString(" ")
			}
			out.WriteString(op.text)
			out.WriteString("\n")
		}
	}
	return out.String()
}
