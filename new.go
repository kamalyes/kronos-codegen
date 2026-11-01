/*
 * @Author: kamalyes 501893067@qq.com
 * @Date: 2026-11-01 16:20:11
 * @LastEditors: kamalyes 501893067@qq.com
 * @LastEditTime: 2026-11-01 20:10:35
 * @FilePath: \kronos-codegen\new.go
 * @Description: new entity 命令 - 把计划里的条目幂等插入到已存在的文件里
 *
 * Copyright (c) 2026 by kamalyes, All Rights Reserved.
 */
package main

import (
	"flag"
	"fmt"
	"go/token"
	"unicode"
)

// runNew 处理 new 命令
// 位置参数先吃掉，剩下的交给 FlagSet，这样 flag 可以写在实体名后面
// [EN] runNew handles the new command; positional arguments are consumed first so flags
// may follow the entity name
func (e *env) runNew(args []string) int {
	if len(args) == 0 {
		fmt.Fprintf(e.err, "new 需要一个子目标，目前支持: entity\n用法: kronos-codegen new entity <Name>\n")
		return exitUsage
	}
	switch args[0] {
	case "entity":
		return e.runNewEntity(args[1:])
	default:
		fmt.Fprintf(e.err, "未知的 new 子目标 %q，目前支持: entity\n", args[0])
		return exitUsage
	}
}

// runNewEntity 处理 new entity 命令
// [EN] runNewEntity handles the new entity command
func (e *env) runNewEntity(args []string) int {
	if len(args) == 0 {
		fmt.Fprintf(e.err, "用法: kronos-codegen new entity <Name> [flag]\n")
		return exitUsage
	}
	name := args[0]
	if err := validateEntityName(name); err != nil {
		fmt.Fprintf(e.err, "实体名不合法: %v\n", err)
		return exitUsage
	}

	fs := flag.NewFlagSet("new entity", flag.ContinueOnError)
	fs.SetOutput(e.err)
	registerGlobal(fs, e.opts)
	if err := fs.Parse(args[1:]); err != nil {
		return exitUsage
	}
	if extra := fs.Args(); len(extra) > 0 {
		fmt.Fprintf(e.err, "多余的参数: %v\n", extra)
		return exitUsage
	}

	root, err := e.resolveRoot()
	if err != nil {
		fmt.Fprintf(e.err, "%v\n", err)
		return exitUsage
	}
	manifest, planPath, err := e.loadPlan(root)
	if err != nil {
		fmt.Fprintf(e.err, "%v\n", err)
		return exitFailure
	}
	eng, err := e.newEngine(root)
	if err != nil {
		fmt.Fprintf(e.err, "%v\n", err)
		return exitUsage
	}

	vars := entityVars(name, e.opts.Vars)
	actions, err := manifest.Actions("entity", vars, root)
	if err != nil {
		fmt.Fprintf(e.err, "%v\n", err)
		return exitFailure
	}
	if len(actions) == 0 {
		fmt.Fprintf(e.err, "补丁计划 %s 的 sections.entity 是空的\n", planPath)
		return exitFailure
	}

	fmt.Fprintf(e.out, "项目根目录: %s\n补丁计划: %s\n实体: %s\n步骤数: %d\n\n",
		root, planPath, name, len(actions))

	if _, err := eng.Run(actions); err != nil {
		fmt.Fprintf(e.err, "执行失败，已中止，工作区未被修改:\n  %v\n", err)
		return exitFailure
	}

	fmt.Fprintf(e.out, "变更摘要:\n%s\n", eng.Summary())
	diff := eng.Diff()
	if diff == "" {
		fmt.Fprintf(e.out, "所有条目都已存在，无需改动\n")
		return exitOK
	}
	fmt.Fprintf(e.out, "变更预览:\n%s\n", diff)

	if e.opts.DryRun {
		fmt.Fprintf(e.out, "dry-run 模式，未写盘\n")
		return exitOK
	}
	if !e.confirm("确认写入以上改动?") {
		fmt.Fprintf(e.out, "已取消，工作区未被修改\n")
		return exitOK
	}
	written, err := eng.Write()
	if err != nil {
		fmt.Fprintf(e.err, "%v\n", err)
		return exitFailure
	}
	for _, f := range written {
		fmt.Fprintf(e.out, "已写入 %s\n", f)
	}
	fmt.Fprintf(e.out, "共写入 %d 个文件\n", len(written))
	return exitOK
}

// validateEntityName 实体名必须是导出的 Go 标识符
// [EN] validateEntityName requires an exported Go identifier
func validateEntityName(name string) error {
	if !token.IsIdentifier(name) {
		return fmt.Errorf("%q 不是合法的 Go 标识符", name)
	}
	if !unicode.IsUpper([]rune(name)[0]) {
		return fmt.Errorf("%q 必须以大写字母开头", name)
	}
	return nil
}
