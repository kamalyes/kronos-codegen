/*
 * @Author: kamalyes 501893067@qq.com
 * @Date: 2026-11-01 16:20:11
 * @LastEditors: kamalyes 501893067@qq.com
 * @LastEditTime: 2026-11-01 20:10:35
 * @FilePath: \kronos-codegen\cli.go
 * @Description: CLI 骨架 - 全局 flag、子命令分发与用法说明，只用标准库 flag
 *
 * Copyright (c) 2026 by kamalyes, All Rights Reserved.
 */

// kronos-codegen 的命令行外壳
// 参数解析只用标准库 flag：子命令自己持有一个 FlagSet，全局 flag 在每个 FlagSet 上重复注册
// 因此 --root 既能写在子命令前面，也能写在位置参数后面
// 真正的能力都在可复用的 patch 与 plan 两个包里，这一层只做参数、编排与输出
// [EN] The command line shell of kronos-codegen. Parsing uses only the standard library
// flag package; global flags are registered on every sub-FlagSet so they may appear either
// before the sub-command or after its positional arguments. The reusable capability lives
// in the patch and plan packages; this layer only does flags, orchestration and output.
package main

import (
	"bufio"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/kamalyes/kronos-codegen/patch"
	"github.com/kamalyes/kronos-codegen/plan"
)

// Version 当前版本号
// [EN] Version is the current version string
const Version = "0.4.0-dev"

// 退出码约定
// [EN] exit code contract
const (
	// exitOK 一切正常
	// [EN] exitOK means success
	exitOK = 0
	// exitFailure 执行失败，如锚点找不到
	// [EN] exitFailure means the run failed, e.g. an anchor was not found
	exitFailure = 1
	// exitUsage 用法错误
	// [EN] exitUsage means the command line itself was wrong
	exitUsage = 2
	// exitDrift check 发现了缺失的条目
	// [EN] exitDrift means check found missing entries
	exitDrift = 3
)

// Options 全局选项
// [EN] Options holds the global flags
type Options struct {
	// Root 项目根目录
	// [EN] Root is the project root directory
	Root string
	// DryRun 只输出 diff 不写盘
	// [EN] DryRun prints the diff without touching the working tree
	DryRun bool
	// Yes 零交互，跳过写盘前的确认
	// [EN] Yes skips the pre-write confirmation prompt
	Yes bool
	// PlanFile 补丁计划清单路径，默认取项目根目录下的 .kronos-codegen.json
	// [EN] PlanFile is the manifest path, defaulting to .kronos-codegen.json at the root
	PlanFile string
	// Format gofmt 归一化策略
	// [EN] Format is the gofmt normalization policy
	Format string
	// Vars 命令行附加变量
	// [EN] Vars holds extra template variables from the command line
	Vars varsFlag
}

// varsFlag 支持重复出现的 --var k=v
// [EN] varsFlag collects repeated --var k=v pairs
type varsFlag map[string]string

// String 实现 flag.Value
// [EN] String implements flag.Value
func (v varsFlag) String() string {
	keys := make([]string, 0, len(v))
	for k := range v {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	pairs := make([]string, 0, len(keys))
	for _, k := range keys {
		pairs = append(pairs, k+"="+v[k])
	}
	return strings.Join(pairs, ",")
}

// Set 实现 flag.Value
// [EN] Set implements flag.Value
func (v varsFlag) Set(s string) error {
	k, val, ok := strings.Cut(s, "=")
	if !ok || k == "" {
		return fmt.Errorf("--var 需要 key=value 形式，收到 %q", s)
	}
	v[k] = val
	return nil
}

// env CLI 运行时的输入输出，便于测试注入
// [EN] env bundles the CLI streams so tests can inject their own
type env struct {
	out  io.Writer
	err  io.Writer
	in   io.Reader
	opts *Options
}

// Run 执行一次命令行调用，返回进程退出码
// 流全部由参数注入，测试因此不必碰真实的 os.Stdin / os.Stdout
// [EN] Run performs one command line invocation and returns the process exit code; the
// streams are injected so tests never touch the real standard streams
func Run(args []string, stdout, stderr io.Writer, stdin io.Reader) int {
	// Format 的默认值在这里给，registerGlobal 会把它当成各级 FlagSet 的默认值
	// [EN] the Format default is seeded here and reused as every FlagSet's default
	opts := &Options{Vars: varsFlag{}, Format: patch.FormatIfClean.String()}
	e := &env{out: stdout, err: stderr, in: stdin, opts: opts}

	// 先吃掉子命令之前的全局 flag
	// [EN] consume the global flags appearing before the sub-command
	root := flag.NewFlagSet("kronos-codegen", flag.ContinueOnError)
	root.SetOutput(stderr)
	registerGlobal(root, opts)
	root.Usage = func() { usage(stderr) }
	if err := root.Parse(args); err != nil {
		return exitUsage
	}
	rest := root.Args()
	if len(rest) == 0 {
		usage(stderr)
		return exitUsage
	}

	switch rest[0] {
	case "new":
		return e.runNew(rest[1:])
	case "check":
		return e.runCheck(rest[1:])
	case "version":
		fmt.Fprintf(stdout, "kronos-codegen %s\n", Version)
		return exitOK
	case "help", "-h", "--help":
		usage(stdout)
		return exitOK
	default:
		fmt.Fprintf(stderr, "未知命令 %q\n\n", rest[0])
		usage(stderr)
		return exitUsage
	}
}

// registerGlobal 把全局 flag 注册到给定的 FlagSet 上
//
// 默认值一律取 Options 里的当前值：子命令的 FlagSet 是在全局 FlagSet 之后创建的，
// 如果这里写死零值，flag.StringVar 会在注册的那一刻把子命令前面已经解析好的值覆盖掉
// [EN] Defaults are read back from Options on purpose: a sub-command FlagSet is created
// after the global one, and StringVar writes its default immediately, which would wipe a
// value already parsed from before the sub-command
func registerGlobal(fs *flag.FlagSet, o *Options) {
	fs.StringVar(&o.Root, "root", o.Root, "项目根目录，默认当前目录")
	fs.BoolVar(&o.DryRun, "dry-run", o.DryRun, "只输出变更预览，不写盘")
	fs.BoolVar(&o.Yes, "yes", o.Yes, "零交互，跳过写盘前的确认")
	fs.StringVar(&o.PlanFile, "plan", o.PlanFile, "补丁计划清单路径，默认 <root>/"+plan.DefaultFileName)
	fs.StringVar(&o.Format, "format", o.Format, "gofmt 归一化策略: if-clean|never|always")
	fs.Var(o.Vars, "var", "附加模板变量，形如 key=value，可重复")
}

// usage 打印用法说明
// [EN] usage prints the help text
func usage(w io.Writer) {
	fmt.Fprint(w, `kronos-codegen - Kronos 代码生成器 / Kronos code generator

用法 / Usage:
  kronos-codegen [全局flag] <命令> [参数] [全局flag]

命令 / Commands:
  new entity <Name>    往已存在的文件里幂等插入一个实体所需的全部条目
  check [--entity N]   一致性校验，报告计划里哪些条目还没落地
  version              打印版本号
  help                 打印本说明

全局 flag / Global flags:
  --root <dir>         项目根目录，默认当前目录
  --dry-run            只输出变更预览，不写盘
  --yes                零交互，跳过写盘前的确认
  --plan <file>        补丁计划清单路径，默认 <root>/.kronos-codegen.json
  --format <policy>    gofmt 归一化策略: if-clean|never|always
  --var key=value      附加模板变量，可重复

示例 / Examples:
  kronos-codegen new entity Order --dry-run
  kronos-codegen --root ../apex-notify-service new entity Order --yes
  kronos-codegen check --entity Order
`)
}

// resolveRoot 解析项目根目录
// [EN] resolveRoot resolves the project root directory
func (e *env) resolveRoot() (string, error) {
	root := e.opts.Root
	if root == "" {
		wd, err := os.Getwd()
		if err != nil {
			return "", err
		}
		root = wd
	}
	abs, err := filepath.Abs(root)
	if err != nil {
		return "", err
	}
	st, err := os.Stat(abs)
	if err != nil || !st.IsDir() {
		return "", fmt.Errorf("项目根目录 %s 不存在或不是目录", abs)
	}
	return abs, nil
}

// loadPlan 载入补丁计划清单，缺失时给出明确的下一步提示
// [EN] loadPlan loads the manifest and explains the next step when it is missing
func (e *env) loadPlan(root string) (*plan.Manifest, string, error) {
	path := e.opts.PlanFile
	if path == "" {
		path = filepath.Join(root, plan.DefaultFileName)
	}
	if _, err := os.Stat(path); err != nil {
		return nil, path, fmt.Errorf(`找不到补丁计划清单 %s

补丁计划描述「往哪个文件的哪个锚点插什么」，业务模板的具体形态由各项目自己的清单给出
用 --plan <file> 指定清单路径，或在项目根目录放一份 %s`, path, plan.DefaultFileName)
	}
	m, err := plan.Load(path)
	if err != nil {
		return nil, path, err
	}
	return m, path, nil
}

// newEngine 按全局选项构造补丁引擎
// [EN] newEngine builds the patch engine from the global options
func (e *env) newEngine(root string) (*patch.Engine, error) {
	policy, err := patch.ParseFormatPolicy(e.opts.Format)
	if err != nil {
		return nil, err
	}
	eng := patch.NewEngine(patch.NewDirStore(root))
	eng.SetFormatPolicy(policy)
	return eng, nil
}

// confirm 写盘前的确认，--yes 时直接通过
// [EN] confirm asks before writing unless --yes was given
func (e *env) confirm(prompt string) bool {
	if e.opts.Yes {
		return true
	}
	fmt.Fprintf(e.out, "%s [y/N]: ", prompt)
	r := bufio.NewReader(e.in)
	line, err := r.ReadString('\n')
	if err != nil && line == "" {
		return false
	}
	switch strings.ToLower(strings.TrimSpace(line)) {
	case "y", "yes":
		return true
	default:
		return false
	}
}
