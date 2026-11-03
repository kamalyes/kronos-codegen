/*
 * @Author: kamalyes 501893067@qq.com
 * @Date: 2026-11-03 20:10:00
 * @LastEditors: kamalyes 501893067@qq.com
 * @LastEditTime: 2026-11-03 20:10:00
 * @FilePath: \kronos-codegen\probe.go
 * @Description: 探测项目里有没有某个类型，供模板按探测结果取舍
 *
 * 为什么要探而不是假设
 *
 * 内置服务模板一度写死嵌 *commonsvc.BaseServiceImpl —— 那是线上某个服务的共享基座
 * 实测在 kronos 服务模板上生成出来的服务编译不过：undefined: commonsvc.BaseServiceImpl
 * 它的 service/common 里只有 Clients 与 GatewayConfigProvider，没有这个类型
 *
 * 同一个原则已经用在契约那边了：嵌 Unimplemented<名字>ServiceServer 之前先看
 * *_grpc.pb.go 里有没有这个类型 项目侧也该一样 —— 生成器对项目的任何假设，
 * 都应该先确认再用，否则它生成的是「在我这台机器上的那个项目里能编译的代码」
 *
 * [EN] Why this probes rather than assumes
 * The built-in service template once hardcoded an embedded *commonsvc.BaseServiceImpl, the
 * shared base of one production service, and the service generated onto the kronos service
 * template did not compile: undefined: commonsvc.BaseServiceImpl, because its service/common
 * holds only Clients and GatewayConfigProvider
 * The same principle already applies on the contract side, where the unimplemented stub is
 * embedded only after confirming the type exists in a *_grpc.pb.go, and the project side
 * deserves the same: any assumption the generator makes about a project should be confirmed
 * before use, or what it generates is code that compiles only against the project on the
 * machine where it was written
 *
 * Copyright (c) 2026 by kamalyes, All Rights Reserved.
 */

package main

import (
	"os"
	"path/filepath"
	"strings"
)

// hasTypeInPackage 报告项目的某个包里有没有声明某个类型
//
// 用文本查找而不是解析 AST：判据是 "type <名字> struct" 这一行，而生成器只需要
// 知道「这个类型在不在」 包里有语法错误的文件时解析会整体失败，而那种情况下
// 更该给出一个保守答案而不是报错中止
//
// [EN] hasTypeInPackage reports whether a package in the project declares a type
// The lookup is textual rather than AST-based, since the question is only whether the type
// exists and parsing would fail wholesale on a package containing a file with a syntax
// error, where a conservative answer serves better than aborting
func hasTypeInPackage(root, pkgDir, typeName string) bool {
	dir := filepath.Join(root, pkgDir)
	entries, err := os.ReadDir(dir)
	if err != nil {
		return false
	}
	needle := "type " + typeName + " struct"
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		raw, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil {
			continue
		}
		if strings.Contains(string(raw), needle) {
			return true
		}
	}
	return false
}
