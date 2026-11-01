/*
 * @Author: kamalyes 501893067@qq.com
 * @Date: 2026-11-01 16:20:11
 * @LastEditors: kamalyes 501893067@qq.com
 * @LastEditTime: 2026-11-01 20:10:35
 * @FilePath: \kronos-codegen\plan\naming.go
 * @Description: 实体名的大小写派生 - 为补丁模板提供通用命名变量
 *
 * Copyright (c) 2026 by kamalyes, All Rights Reserved.
 */
package plan

import (
	"strings"
	"unicode"
)

// Vars 由实体名派生出一组通用命名变量，供补丁计划里的模板引用
// 只做纯粹的大小写与分隔符转换，不含任何业务约定
// [EN] Vars derives the generic naming variables of one entity name; it only performs
// case and separator conversion and encodes no business convention
func Vars(name string) map[string]string {
	words := splitWords(name)
	return map[string]string{
		"Name":      PascalCase(words),
		"Lower":     LowerCamelCase(words),
		"Snake":     strings.Join(lowerAll(words), "_"),
		"Kebab":     strings.Join(lowerAll(words), "-"),
		"Upper":     strings.ToUpper(strings.Join(lowerAll(words), "_")),
		"Flat":      strings.Join(lowerAll(words), ""),
		"Delimited": strings.Join(lowerAll(words), "."),
	}
}

// splitWords 把标识符拆成词，支持 PascalCase、camelCase、snake_case 与 kebab-case
// 连续大写被当成缩写整体保留，如 HTTPServer 拆成 HTTP + Server
// [EN] splitWords splits an identifier into words, keeping acronyms intact
func splitWords(s string) []string {
	var words []string
	var cur []rune
	flush := func() {
		if len(cur) > 0 {
			words = append(words, string(cur))
			cur = nil
		}
	}
	runes := []rune(s)
	for i, r := range runes {
		switch {
		case r == '_' || r == '-' || r == ' ' || r == '.':
			flush()
		case unicode.IsUpper(r):
			// 大写字母开新词，但缩写内部不断开
			// [EN] an upper rune starts a new word unless it sits inside an acronym
			prevLower := i > 0 && !unicode.IsUpper(runes[i-1]) && runes[i-1] != '_' && runes[i-1] != '-'
			nextLower := i+1 < len(runes) && unicode.IsLower(runes[i+1])
			if prevLower || (nextLower && len(cur) > 1) {
				flush()
			}
			cur = append(cur, r)
		default:
			cur = append(cur, r)
		}
	}
	flush()
	return words
}

// lowerAll 把所有词转成小写
// [EN] lowerAll lowercases every word
func lowerAll(words []string) []string {
	out := make([]string, 0, len(words))
	for _, w := range words {
		out = append(out, strings.ToLower(w))
	}
	return out
}

// PascalCase 拼成大驼峰
// [EN] PascalCase joins words in upper camel case
func PascalCase(words []string) string {
	var b strings.Builder
	for _, w := range words {
		b.WriteString(titleWord(w))
	}
	return b.String()
}

// LowerCamelCase 拼成小驼峰
// [EN] LowerCamelCase joins words in lower camel case
func LowerCamelCase(words []string) string {
	var b strings.Builder
	for i, w := range words {
		if i == 0 {
			b.WriteString(strings.ToLower(w))
			continue
		}
		b.WriteString(titleWord(w))
	}
	return b.String()
}

// titleWord 首字母大写，其余保持原样以保留缩写
// [EN] titleWord uppercases the first rune and keeps the rest, preserving acronyms
func titleWord(w string) string {
	if w == "" {
		return w
	}
	r := []rune(w)
	if len(r) > 1 && isAllUpper(r) {
		return string(r)
	}
	return string(unicode.ToUpper(r[0])) + string(r[1:])
}

// isAllUpper 判断是否全是大写字母
// [EN] isAllUpper reports whether every rune is upper case
func isAllUpper(r []rune) bool {
	for _, c := range r {
		if !unicode.IsUpper(c) {
			return false
		}
	}
	return true
}
