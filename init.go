/*
 * @Author: kamalyes 501893067@qq.com
 * @Date: 2026-11-03 17:30:00
 * @LastEditors: kamalyes 501893067@qq.com
 * @LastEditTime: 2026-11-03 17:30:00
 * @FilePath: \kronos-codegen\init.go
 * @Description: init 命令 - 把内置清单与模板取进项目供定制
 *
 * 内置清单让约定形态的服务零配置可用，但形态不一样的项目需要一条「取出来改」
 * 的路：让人对着一份跑得通的清单改，而不是对着参考手册从零编一份
 *
 * 这一步会把清单里的 builtin: 模板引用改写成项目内的相对路径，否则取出来的
 * 清单仍然指向二进制里的模板，改了模板不生效
 *
 * [EN] The init command copies the built-in manifest and templates into a project
 * The built-in manifest makes a conventional service work with no configuration, but a
 * project of a different shape needs a way to take it out and edit it, so the starting
 * point is a manifest that already runs rather than a reference manual
 * The built-in template references are rewritten to project-relative paths, since a copied
 * manifest still pointing into the binary would ignore edits to the copied templates
 *
 * Copyright (c) 2026 by kamalyes, All Rights Reserved.
 */

package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/kamalyes/kronos-codegen/builtin"
	"github.com/kamalyes/kronos-codegen/plan"
)

// templateDir 取出来的模板放在项目的这个子目录下
// [EN] templateDir is where copied templates live inside the project
const templateDir = ".kronos-codegen"

// runInit 处理 init 命令
// [EN] runInit handles the init command
func (e *env) runInit(args []string) int {
	fs := flag.NewFlagSet("init", flag.ContinueOnError)
	fs.SetOutput(e.err)
	registerGlobal(fs, e.opts)
	if err := fs.Parse(args); err != nil {
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

	names, err := builtin.Files()
	if err != nil {
		fmt.Fprintf(e.err, "列出内置文件失败: %v\n", err)
		return exitFailure
	}

	type item struct {
		target string
		data   []byte
	}
	var items []item
	for _, name := range names {
		raw, err := builtin.Read(name)
		if err != nil {
			fmt.Fprintf(e.err, "读取内置文件 %s 失败: %v\n", name, err)
			return exitFailure
		}
		target := filepath.Join(templateDir, name)
		if name == builtin.ManifestName {
			// 清单落在默认位置，这样 new / check 不给 --plan 也能直接认到
			// 同时把 builtin: 引用改成取出来的模板路径
			//
			// [EN] The manifest lands at the default location so new and check find it
			// without --plan, and its builtin: references are rewritten to the copied
			// templates
			target = plan.DefaultFileName
			raw = []byte(strings.ReplaceAll(string(raw), builtin.Prefix, templateDir+"/"))
		}
		items = append(items, item{target: target, data: raw})
	}

	fmt.Fprintf(e.out, "项目根目录: %s\n\n将写入:\n", root)
	var existing []string
	for _, it := range items {
		abs := filepath.Join(root, it.target)
		mark := ""
		if _, err := os.Stat(abs); err == nil {
			mark = "  (已存在，会被覆盖)"
			existing = append(existing, it.target)
		}
		fmt.Fprintf(e.out, "  %s  (%d 字节)%s\n", it.target, len(it.data), mark)
	}

	if e.opts.DryRun {
		fmt.Fprintf(e.out, "\n--dry-run，未写盘\n")
		return exitOK
	}
	if len(existing) > 0 && !e.confirm(fmt.Sprintf("\n其中 %d 个文件已存在并会被覆盖，继续吗", len(existing))) {
		fmt.Fprintf(e.out, "已取消\n")
		return exitOK
	}

	for _, it := range items {
		abs := filepath.Join(root, it.target)
		if err := os.MkdirAll(filepath.Dir(abs), 0o755); err != nil {
			fmt.Fprintf(e.err, "建目录失败: %v\n", err)
			return exitFailure
		}
		if err := os.WriteFile(abs, it.data, 0o644); err != nil {
			fmt.Fprintf(e.err, "写入 %s 失败: %v\n", it.target, err)
			return exitFailure
		}
		fmt.Fprintf(e.out, "已写入 %s\n", it.target)
	}
	fmt.Fprintf(e.out, `
清单已落在 %s，改它即可整体覆盖内置的那份
模板里的变量见 docs/MANIFEST.md，字段表来自 --pb 指定的 .pb.go
`, plan.DefaultFileName)
	return exitOK
}
