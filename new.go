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
	"errors"
	"flag"
	"fmt"
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"unicode"

	"github.com/kamalyes/kronos-codegen/patch"
	"github.com/kamalyes/kronos-codegen/pbfield"
	"github.com/kamalyes/kronos-codegen/plan"
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
	// --pb / --message 指向契约里的消息，字段表从 protoc 生成的 .pb.go 里取
	//
	// 读 .pb.go 而不是 .proto：两者信息一样多，但 .pb.go 里的 Go 类型是 protoc
	// 自己映射的，不会和它不一致，而且不用为了解析 proto 文法引一条重依赖
	// 详见 pbfield 包头的说明
	//
	// 不给这两个参数时字段表为空，模型只生成骨架 —— 字段是实体的实质，
	// 没有契约就不该假装生成得出来
	//
	// [EN] --pb and --message point at a message in the contract, whose field table comes
	// from the .pb.go protoc generated
	// Reading .pb.go rather than .proto carries the same information while the Go types are
	// protoc's own and cannot disagree with it, and it needs no heavy dependency for a proto
	// grammar; see the pbfield package comment
	// Without these flags the field table is empty and the model is a skeleton, since the
	// fields are the substance of an entity and should not be invented without a contract
	pbPath := fs.String("pb", "", "protoc 生成的 .pb.go 路径，字段表从它来")
	message := fs.String("message", "", "契约里的消息名，默认取 <Name>Info")
	indexCols := fs.String("index-columns", "", "显式指定要加索引的列，逗号分隔；留空按「非主键的 *_id 列」判断")
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
	// 用的是哪份清单要先打出来，而不是等展开成功之后
	// 展开失败时这一行同样重要：是内置清单还是项目自己的清单，决定了下一步该改哪儿
	//
	// [EN] Which manifest was used is printed before expansion rather than after it
	// succeeds, since on failure that line decides where to look next: the built-in manifest
	// or the project's own
	fmt.Fprintf(e.out, "项目根目录: %s\n补丁计划: %s\n实体: %s\n", root, planPath, name)

	eng, err := e.newEngine(root)
	if err != nil {
		fmt.Fprintf(e.err, "%v\n", err)
		return exitUsage
	}

	var fields any
	var imports []string
	var pbImport, pbAlias string
	var hasSvc bool
	if *pbPath != "" {
		msg := *message
		if msg == "" {
			msg = name + "Info"
		}
		parsed, err := pbfield.Parse(*pbPath, msg)
		if err != nil {
			fmt.Fprintf(e.err, "取字段表失败: %v\n", err)
			return exitFailure
		}
		var cols []string
		if *indexCols != "" {
			cols = strings.Split(*indexCols, ",")
		}
		modelFields := pbfield.Model(parsed, plan.Vars(name)["Snake"], cols)
		fields = modelFields
		imports = pbfield.ModelImports(modelFields, parsed.Imports)
		fmt.Fprintf(e.out, "字段表: %s 的 %s，%d 个字段\n", *pbPath, msg, len(parsed.Fields))

		// 契约包的导入路径、别名与「有没有同名 gRPC 服务」都从 --pb 推出来
		// 让人再传三个参数只会传错，而错了的后果是生成的文件编译不过
		//
		// [EN] The contract package's import path, alias and whether it has a matching gRPC
		// service all follow from --pb; three more flags would only invite mistakes whose
		// consequence is a generated file that does not compile
		contract, err := pbfield.DescribeContract(*pbPath)
		if err != nil {
			fmt.Fprintf(e.err, "推断契约包失败: %v\n", err)
			return exitFailure
		}
		hasService, err := pbfield.HasGRPCService(*pbPath, name)
		if err != nil {
			fmt.Fprintf(e.err, "探测 gRPC 服务失败: %v\n", err)
			return exitFailure
		}
		pbImport, pbAlias, hasSvc = contract.Import, contract.Alias, hasService
		fmt.Fprintf(e.out, "契约包: %s（别名 %s），%sServiceServer 存在=%v\n",
			contract.Import, contract.Alias, name, hasService)
	}

	vars := entityVars(name, e.opts.Vars, fields)
	// Module 从项目的 go.mod 读出来，不要求调用方传
	//
	// 生成的仓库与服务文件要导入本项目的 models / repository 包，导入路径就是
	// 模块路径加子目录 这个值项目自己的 go.mod 里就有，让人在 --var 里再抄一遍
	// 只会抄错，而且抄错的后果是生成出来的文件编译不过
	// --var Module=... 仍然可以覆盖（多模块仓库里根 go.mod 不是目标模块时要用）
	//
	// [EN] Module is read from the project's go.mod rather than required from the caller
	// The generated repository and service files import this project's models and repository
	// packages, whose paths are the module path plus a subdirectory; the value is already in
	// the project's go.mod, and making someone retype it into --var only invites a typo
	// whose consequence is a generated file that does not compile
	// --var Module=... still overrides it, which a multi-module repository needs when the
	// root go.mod is not the target module
	// go.mod 不在就不设这个键，而不是直接失败
	//
	// 不是每个被补丁的目录都是一个 Go 模块根，用不到 .Module 的清单也不该因此跑不起来
	// 实测：早先把它做成硬性要求，所有没有 go.mod 的项目与测试夹具全部被打断
	// 真用到 .Module 的模板会在渲染时报键不存在，下面把可操作的提示补上
	//
	// [EN] An absent go.mod leaves the key unset rather than failing outright
	// Not every patched directory is a module root, and a manifest that never uses .Module
	// should not be blocked by its absence
	// Measured: making it mandatory broke every project and test fixture without a go.mod
	// A template that does use .Module fails on the missing key at render time, and the hint
	// below makes that actionable
	if _, ok := vars["Module"]; !ok {
		if mod, err := modulePath(root); err == nil {
			vars["Module"] = mod
		}
	}
	// 没有契约时 Imports 为空切片而不是缺席，理由同 entityVars 里 Fields 的说明
	// [EN] Imports is an empty slice rather than absent when there is no contract, for the
	// reason given for Fields in entityVars
	if imports == nil {
		imports = []string{}
	}
	vars["Imports"] = imports
	vars["PbImport"] = pbImport
	vars["PbAlias"] = pbAlias
	vars["HasService"] = hasSvc
	items, err := manifest.Items("entity", vars, root)
	if err != nil {
		fmt.Fprintf(e.err, "%v\n", err)
		if strings.Contains(err.Error(), `key "Module"`) {
			fmt.Fprintf(e.err, `
模板用到了 .Module，但它没有被设上 这个值取自项目根目录的 go.mod
%s 里没有 go.mod 时，用 --var Module=<模块路径> 显式给出
`, root)
		}
		return exitFailure
	}
	if len(items) == 0 {
		fmt.Fprintf(e.err, "补丁计划 %s 的 sections.entity 是空的\n", planPath)
		return exitFailure
	}

	fmt.Fprintf(e.out, "步骤数: %d\n\n", len(items))

	// 逐条执行而不是 eng.Run：optional 的条目在「锚点不存在」时要跳过并汇报
	//
	// 为什么要跳过而不是失败：service/exports.go 的别名块靠标记注释定位，而真实
	// 服务里有一部分还没加标记 硬失败会把整次生成挡住，连本来能接上的那几处
	// 也写不进去 —— 而生成器的价值正是把能接的都接上
	//
	// 为什么跳过是安全的：每一条的结果都打出来，被跳过的还会在末尾单独汇总，
	// 明确写出「需要人工接线」 这不是静默降级
	//
	// 非 optional 的条目仍然硬失败：那说明项目结构与清单的假设真的对不上
	//
	// [EN] Steps run one at a time rather than through eng.Run, so an optional step whose
	// anchor cannot be located is skipped and reported
	// Why skip rather than fail: the alias blocks in service/exports.go are located by a
	// marker comment that some real services have not added, and a hard failure would block
	// the whole generation including the wiring that would have worked, which is exactly the
	// value the generator provides
	// Why skipping is safe: every step's outcome is printed and the skipped ones are
	// summarised at the end as needing manual wiring, so this is not a silent downgrade
	// A step that is not optional still fails hard, since that means the project's structure
	// genuinely disagrees with the manifest
	// 先用一个探测引擎跑一遍，找出哪些 optional 条目定位不到、以及它们属于哪一组
	//
	// 探测引擎的改动只在内存里，不写盘，所以这一遍是免费的
	// 这么做是为了「同组要么全进要么全不进」：直接边跑边跳会让同组里先成功的那几条
	// 已经落地，再想撤回就只能回滚 —— 而部分落地正是上一版生成出编译不过代码的原因
	//
	// [EN] A probe engine runs the steps first to find which optional ones cannot be located
	// and which group they belong to
	// The probe's changes live in memory and are never written, so the pass costs nothing
	// It exists to make a group all-or-nothing: skipping as it goes would leave the earlier
	// steps of a group already applied, and that partial application is exactly what made
	// the previous version emit code that did not compile
	probe, err := e.newEngine(root)
	if err != nil {
		fmt.Fprintf(e.err, "%v\n", err)
		return exitUsage
	}
	broken := map[string]bool{}
	skippedBy := map[int]string{}
	for i, it := range items {
		if _, err := probe.Step(it.Action); err != nil {
			if it.Optional && errors.Is(err, patch.ErrNotFound) {
				skippedBy[i] = err.Error()
				if it.Group != "" {
					broken[it.Group] = true
				}
				continue
			}
			fmt.Fprintf(e.err, "执行失败，已中止，工作区未被修改:\n  %v\n", err)
			return exitFailure
		}
	}

	var skipped []string
	for i, it := range items {
		if msg, ok := skippedBy[i]; ok {
			skipped = append(skipped, msg)
			continue
		}
		if it.Group != "" && broken[it.Group] {
			skipped = append(skipped, fmt.Sprintf("%s: 同组的其他条目定位不到，整组一起跳过（组 %q）", it.Action.File(), it.Group))
			continue
		}
		if _, err := eng.Step(it.Action); err != nil {
			fmt.Fprintf(e.err, "执行失败，已中止，工作区未被修改:\n  %v\n", err)
			return exitFailure
		}
	}

	if len(skipped) > 0 {
		fmt.Fprintf(e.out, "以下 %d 条因锚点不存在被跳过，需要人工接线:\n", len(skipped))
		for _, sk := range skipped {
			fmt.Fprintf(e.out, "  %s\n", sk)
		}
		fmt.Fprintln(e.out)
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

// modulePath 从项目根目录的 go.mod 里读出模块路径
// [EN] modulePath reads the module path out of the project root's go.mod
func modulePath(root string) (string, error) {
	path := filepath.Join(root, "go.mod")
	raw, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("读取 %s 失败: %w", path, err)
	}
	for _, line := range strings.Split(string(raw), "\n") {
		if rest, ok := strings.CutPrefix(strings.TrimSpace(line), "module "); ok {
			if mod := strings.TrimSpace(rest); mod != "" {
				return mod, nil
			}
		}
	}
	return "", fmt.Errorf("%s 里没有 module 声明", path)
}
