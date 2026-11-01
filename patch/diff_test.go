/*
 * @Author: kamalyes 501893067@qq.com
 * @Date: 2026-11-01 18:05:12
 * @LastEditors: kamalyes 501893067@qq.com
 * @LastEditTime: 2026-11-01 20:12:06
 * @FilePath: \kronos-codegen\patch\diff_test.go
 * @Description: dry-run diff 渲染测试
 *
 * Copyright (c) 2026 by kamalyes, All Rights Reserved.
 */
package patch

import (
	"strings"
	"testing"
)

// TestUnifiedDiffInsertion 纯插入场景的 diff 输出，hunk 头要给出正确行号
// [EN] TestUnifiedDiffInsertion checks the hunk header line numbers of a pure insertion
func TestUnifiedDiffInsertion(t *testing.T) {
	store := NewMemStore(map[string]string{"constants/table.go": constFixture})
	eng := NewEngine(store)
	eng.SetFormatPolicy(FormatNever)
	if _, err := eng.Apply(&DeclInsert{Path: "constants/table.go",
		Block: BlockRef{Member: "TableNameSecond"}, Name: "TableNameOrder", Value: `"orders"`}); err != nil {
		t.Fatalf("应用失败: %v", err)
	}
	want := "--- a/constants/table.go\n" +
		"+++ b/constants/table.go\n" +
		"@@ -12,6 +12,7 @@\n" +
		" \t// TableNameExample 示例表\n" +
		" \tTableNameExample = \"examples\"\n" +
		" \tTableNameSecond  = \"seconds\" // 第二张表\n" +
		"+\tTableNameOrder = \"orders\"\n" +
		" )\n" +
		" \n" +
		" // ServiceName 服务名\n"
	got := eng.Diff()
	if got != want {
		t.Errorf("diff 输出不符\n--- got ---\n%s\n--- want ---\n%s", got, want)
	}
}

// TestUnifiedDiffNoChange 内容一致时返回空串
// [EN] TestUnifiedDiffNoChange returns an empty string for identical content
func TestUnifiedDiffNoChange(t *testing.T) {
	if got := UnifiedDiff("a.go", []byte("a\nb\n"), []byte("a\nb\n"), 3); got != "" {
		t.Errorf("无改动时应返回空串，实际 %q", got)
	}
}

// TestUnifiedDiffReplacement 替换与删除也要能正确渲染，diff 渲染本身是通用的
// [EN] TestUnifiedDiffReplacement renders replacements and deletions as well
func TestUnifiedDiffReplacement(t *testing.T) {
	got := UnifiedDiff("a.txt", []byte("one\ntwo\nthree\n"), []byte("one\n2\nthree\nfour\n"), 1)
	for _, want := range []string{"-two", "+2", "+four"} {
		if !strings.Contains(got, want) {
			t.Errorf("diff 里缺少 %q\n%s", want, got)
		}
	}
}

// TestDiffLargeWindowFallback 差异窗口过大时退化成整段替换，不应卡死或爆内存
// [EN] TestDiffLargeWindowFallback degrades to replace-all for an oversized window
func TestDiffLargeWindowFallback(t *testing.T) {
	var a, b strings.Builder
	for i := 0; i < 2200; i++ {
		a.WriteString("a\n")
		b.WriteString("b\n")
	}
	got := UnifiedDiff("big.txt", []byte(a.String()), []byte(b.String()), 0)
	if !strings.Contains(got, "-a") || !strings.Contains(got, "+b") {
		t.Errorf("退化路径输出不符\n%s", firstLines(got, 5))
	}
}
