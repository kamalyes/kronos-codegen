/*
 * @Author: kamalyes 501893067@qq.com
 * @Date: 2026-11-01 16:20:11
 * @LastEditors: kamalyes 501893067@qq.com
 * @LastEditTime: 2026-11-01 20:10:35
 * @FilePath: \kronos-codegen\check.go
 * @Description: check 命令 - 一致性校验，复用补丁引擎的幂等判定报告缺失条目
 *
 * Copyright (c) 2026 by kamalyes, All Rights Reserved.
 */
package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"github.com/kamalyes/kronos-codegen/plan"
)

// runCheck 处理 check 命令
// 给了 --entity 就逐条报告该实体的条目是否已落地，没给就只校验清单与目标文件是否存在
// 这里只读不写，缺失条目用退出码 3 报出来，便于接 CI
// [EN] runCheck handles the check command; with --entity it reports which entries of that
// entity are missing, otherwise it only validates the manifest and its target files. It
// never writes and reports drift with exit code 3 so CI can gate on it.
func (e *env) runCheck(args []string) int {
	fs := flag.NewFlagSet("check", flag.ContinueOnError)
	fs.SetOutput(e.err)
	entity := fs.String("entity", "", "要校验的实体名，留空则只校验补丁计划本身")
	registerGlobal(fs, e.opts)
	if err := fs.Parse(args); err != nil {
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
	fmt.Fprintf(e.out, "项目根目录: %s\n补丁计划: %s\n分组: %v\n\n",
		root, planPath, manifest.SectionNames())

	if *entity == "" {
		return e.checkManifestOnly(manifest, root)
	}
	if err := validateEntityName(*entity); err != nil {
		fmt.Fprintf(e.err, "实体名不合法: %v\n", err)
		return exitUsage
	}
	return e.checkEntity(manifest, root, *entity)
}

// checkManifestOnly 没给实体名时，只校验清单合法性与目标文件是否存在
// [EN] checkManifestOnly validates the manifest and the existence of its target files
func (e *env) checkManifestOnly(m *plan.Manifest, root string) int {
	missing := 0
	for _, section := range m.SectionNames() {
		steps := m.Sections[section]
		fmt.Fprintf(e.out, "sections.%s: %d 条步骤\n", section, len(steps))
		for i, s := range steps {
			if s.File == "" {
				continue
			}
			// 带模板变量的路径这里无法展开，跳过存在性检查
			// [EN] a templated path cannot be resolved here, so skip its existence check
			if containsTemplate(s.File) {
				fmt.Fprintf(e.out, "  [%d] %s -> %s (含模板变量，跳过存在性检查)\n", i, s.Kind, s.File)
				continue
			}
			if _, err := os.Stat(filepath.Join(root, filepath.FromSlash(s.File))); err != nil {
				if s.Kind == "create_file" {
					// create_file 的目标本来就该是不存在的，这不是缺失
					// [EN] a create_file target is supposed to be absent, so this is not drift
					fmt.Fprintf(e.out, "  [%d] %s -> %s  (待新建)\n", i, s.Kind, s.File)
					continue
				}
				missing++
				fmt.Fprintf(e.out, "  [%d] %s -> %s  目标文件不存在\n", i, s.Kind, s.File)
				continue
			}
			fmt.Fprintf(e.out, "  [%d] %s -> %s  ok\n", i, s.Kind, s.File)
		}
	}
	if missing > 0 {
		fmt.Fprintf(e.out, "\n有 %d 个目标文件缺失\n", missing)
		return exitDrift
	}
	fmt.Fprintf(e.out, "\n补丁计划自检通过\n")
	return exitOK
}

// checkEntity 对单个实体做一致性校验，复用补丁引擎的幂等判定
// 计划为 Present 说明条目已落地，否则就是漂移
// [EN] checkEntity reuses the engine's idempotency detection: a Present plan means the
// entry is already in place, anything else is drift
func (e *env) checkEntity(m *plan.Manifest, root, entity string) int {
	eng, err := e.newEngine(root)
	if err != nil {
		fmt.Fprintf(e.err, "%v\n", err)
		return exitUsage
	}
	actions, err := m.Actions("entity", entityVars(entity, e.opts.Vars), root)
	if err != nil {
		fmt.Fprintf(e.err, "%v\n", err)
		return exitFailure
	}
	present, drift := 0, 0
	for _, a := range actions {
		pl, err := eng.Step(a)
		if err != nil {
			drift++
			fmt.Fprintf(e.out, "  error  %s  %s\n           %v\n", a.File(), a.Label(), err)
			continue
		}
		switch {
		case pl.Present:
			present++
			fmt.Fprintf(e.out, "  ok     %s:%d  %s\n", pl.File, pl.Line, pl.Label)
		case pl.Created:
			// 该建但还没建的文件同样算漂移
			// [EN] a file that should exist but does not is drift as well
			drift++
			fmt.Fprintf(e.out, "  miss   %s  %s  (文件不存在)\n", pl.File, pl.Label)
		default:
			drift++
			fmt.Fprintf(e.out, "  miss   %s:%d  %s\n", pl.File, pl.Line, pl.Label)
		}
	}
	fmt.Fprintf(e.out, "\n实体 %s: %d 条已落地，%d 条缺失\n", entity, present, drift)
	if drift > 0 {
		return exitDrift
	}
	return exitOK
}

// containsTemplate 判断字符串里是否含模板占位符
// [EN] containsTemplate reports whether the string holds a template placeholder
func containsTemplate(s string) bool {
	for i := 0; i+1 < len(s); i++ {
		if s[i] == '{' && s[i+1] == '{' {
			return true
		}
	}
	return false
}

// entityVars 合并实体派生变量与命令行附加变量，命令行优先
// [EN] entityVars merges the derived entity variables with the command line ones
func entityVars(name string, extra map[string]string) map[string]string {
	vars := plan.Vars(name)
	for k, v := range extra {
		vars[k] = v
	}
	return vars
}
