/*
 * @Author: kamalyes 501893067@qq.com
 * @Date: 2026-11-03 16:40:00
 * @LastEditors: kamalyes 501893067@qq.com
 * @LastEditTime: 2026-11-03 16:40:00
 * @FilePath: \kronos-codegen\builtin\builtin.go
 * @Description: 内置的补丁计划与模板 - 让 kronos 形态的服务零配置可用
 *
 * 为什么要内置
 *
 * 原先每个项目都得自己写一份 .kronos-codegen.json，而要写出它，人得先读完
 * 清单的参考手册、再自己判断锚点落在自己代码的哪一行 实测这一步就是「看着
 * 很好用但不知道怎么用」的来源：整个工作区里一份清单都没有
 *
 * 内置清单能成立，靠的是这套服务的锚点全是约定名而不是项目特有的名字：
 * 仓库工厂一律叫 RepositoryFactory / NewRepositoryFactory，迁移清单一律叫
 * GetAllModels，models/table_names.go 一律只有一个 var 块
 * 这三条是照三个线上服务与 kronos-service-template 逐个核对出来的，不是假设
 *
 * 清单里刻意不含任何具体业务列名 早先的模型模板写死过 tenant_id，那是把某个
 * 产品的租户模型焊进了一个通用工具 —— 别的项目用 org_id、或者没有租户维度
 *
 * 项目形态不一样时，放一份自己的 .kronos-codegen.json 就能整体覆盖内置的
 *
 * [EN] Why this is built in
 * Every project used to need its own .kronos-codegen.json, and writing one meant reading
 * the manifest reference and then deciding where each anchor lands in your own code
 * That step is where "looks useful but I cannot tell how to use it" came from: there was
 * not a single manifest anywhere in the workspace
 * A built-in manifest is possible because this service shape anchors on convention names
 * rather than project-specific ones: across three services and the template the repository
 * factory is always RepositoryFactory and NewRepositoryFactory, the migration list is
 * always GetAllModels, and models/table_names.go always holds a single var block, all
 * verified rather than assumed
 * A project with a different shape overrides the whole thing with its own manifest
 *
 * Copyright (c) 2026 by kamalyes, All Rights Reserved.
 */

package builtin

import (
	"embed"
	"io/fs"
	"strings"
)

//go:embed kronos-service.json templates/*.tmpl
var files embed.FS

const (
	// Prefix 清单里引用内置模板的前缀，如 builtin:templates/model.go.tmpl
	// [EN] Prefix marks a built-in template reference in a manifest
	Prefix = "builtin:"
	// ManifestName 内置清单的文件名
	// [EN] ManifestName is the built-in manifest's file name
	ManifestName = "kronos-service.json"
	// Origin 内置清单在提示信息里的名字，不是磁盘路径
	// [EN] Origin is how the built-in manifest is named in messages; it is not a disk path
	Origin = "内置 kronos-service"
)

// Manifest 返回内置清单的内容
// [EN] Manifest returns the built-in manifest's bytes
func Manifest() ([]byte, error) { return files.ReadFile(ManifestName) }

// IsRef 报告一个模板路径是否指向内置模板
// [EN] IsRef reports whether a template path refers to a built-in template
func IsRef(p string) bool { return strings.HasPrefix(p, Prefix) }

// Read 读一个内置模板，入参可带或不带前缀
// [EN] Read reads a built-in template, with or without the prefix
func Read(p string) ([]byte, error) { return files.ReadFile(strings.TrimPrefix(p, Prefix)) }

// Files 返回内置的全部文件名，供 init 命令写进项目
// [EN] Files lists every built-in file name, for the init command to write into a project
func Files() ([]string, error) {
	var out []string
	err := fs.WalkDir(files, ".", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.IsDir() {
			out = append(out, path)
		}
		return nil
	})
	return out, err
}
