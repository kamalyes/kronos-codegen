/*
 * @Author: kamalyes 501893067@qq.com
 * @Date: 2026-11-03 17:50:00
 * @LastEditors: kamalyes 501893067@qq.com
 * @LastEditTime: 2026-11-03 17:50:00
 * @FilePath: \kronos-codegen\pbfield\contract.go
 * @Description: 从 .pb.go 的位置推出契约包的导入路径、别名，以及它有没有对应的 gRPC 服务
 *
 * 这三样都是生成服务文件必需的，而且都能从 --pb 这一个入参推出来，不该再要人传：
 *
 *  - 导入路径：往上找到契约仓库的 go.mod，模块路径加上 .pb.go 所在的相对目录
 *  - 别名：目录名加 pb，与这套生态里 commonpb / campaignpb / tenantpb 的写法一致
 *  - 有没有 gRPC 服务：同目录的 *_grpc.pb.go 里有没有 Unimplemented<名字>ServiceServer
 *
 * 最后一项决定生成的服务要不要嵌那个未实现桩 嵌了它，服务从生成出来的那一刻
 * 就满足 gRPC 接口、可以直接注册，未实现的方法返回 Unimplemented；契约里没有
 * 对应服务时嵌它则编译不过 所以这件事必须探，不能假设
 *
 * [EN] Derives the contract package's import path and alias, and whether it has a matching
 * gRPC service, all three of which a generated service file needs and all three of which
 * follow from the single --pb argument rather than more flags
 * The import path walks up to the contract repository's go.mod and joins the module path
 * with the .pb.go's relative directory; the alias is the directory name plus pb, matching
 * how this ecosystem writes commonpb, campaignpb and tenantpb; and the gRPC service is
 * detected by looking for Unimplemented<Name>ServiceServer in a *_grpc.pb.go beside it
 * That last one decides whether the generated service embeds the unimplemented stub, which
 * makes it satisfy the gRPC interface and be registrable from the moment it is generated
 * with unimplemented methods returning Unimplemented, while embedding it when the contract
 * has no such service would not compile, so it has to be detected rather than assumed
 *
 * Copyright (c) 2026 by kamalyes, All Rights Reserved.
 */

package pbfield

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Contract 契约包的位置信息
// [EN] Where the contract package lives
type Contract struct {
	// Import 契约包的导入路径
	// [EN] Import is the contract package's import path
	Import string
	// Alias 导入别名，形如 commonpb
	// [EN] Alias is the import alias, shaped like commonpb
	Alias string
}

// DescribeContract 从 .pb.go 的路径推出契约包的导入路径与别名
// [EN] DescribeContract derives the contract package's import path and alias from a .pb.go path
func DescribeContract(pbPath string) (*Contract, error) {
	abs, err := filepath.Abs(pbPath)
	if err != nil {
		return nil, err
	}
	dir := filepath.Dir(abs)

	// 往上找 go.mod：契约通常是独立仓库，模块路径只有它自己的 go.mod 知道
	// [EN] Walk up for go.mod, since the contract is usually its own repository and only
	// its go.mod knows the module path
	modRoot := dir
	for {
		if _, err := os.Stat(filepath.Join(modRoot, "go.mod")); err == nil {
			break
		}
		parent := filepath.Dir(modRoot)
		if parent == modRoot {
			return nil, fmt.Errorf("从 %s 往上没找到 go.mod，无法推出契约包的导入路径", dir)
		}
		modRoot = parent
	}

	raw, err := os.ReadFile(filepath.Join(modRoot, "go.mod"))
	if err != nil {
		return nil, err
	}
	module := ""
	for _, line := range strings.Split(string(raw), "\n") {
		if rest, ok := strings.CutPrefix(strings.TrimSpace(line), "module "); ok {
			module = strings.TrimSpace(rest)
			break
		}
	}
	if module == "" {
		return nil, fmt.Errorf("%s 里没有 module 声明", filepath.Join(modRoot, "go.mod"))
	}

	rel, err := filepath.Rel(modRoot, dir)
	if err != nil {
		return nil, err
	}
	importPath := module
	if rel != "." {
		importPath = module + "/" + filepath.ToSlash(rel)
	}

	base := filepath.Base(dir)
	alias := strings.ReplaceAll(base, "_", "") + "pb"
	return &Contract{Import: importPath, Alias: alias}, nil
}

// HasGRPCService 报告契约里有没有名为 <name>Service 的 gRPC 服务
//
// 判据是同目录的 *_grpc.pb.go 里有没有 Unimplemented<name>ServiceServer
// 这个类型是 protoc-gen-go-grpc 为每个 service 生成的，有它就说明服务存在
//
// [EN] HasGRPCService reports whether the contract defines a gRPC service named
// <name>Service, judged by the presence of Unimplemented<name>ServiceServer in a
// *_grpc.pb.go beside it, a type protoc-gen-go-grpc emits for every service
func HasGRPCService(pbPath, name string) (bool, error) {
	dir := filepath.Dir(pbPath)
	entries, err := os.ReadDir(dir)
	if err != nil {
		return false, err
	}
	needle := "Unimplemented" + name + "ServiceServer"
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), "_grpc.pb.go") {
			continue
		}
		raw, err := os.ReadFile(filepath.Join(dir, entry.Name()))
		if err != nil {
			return false, err
		}
		if strings.Contains(string(raw), "type "+needle+" struct") {
			return true, nil
		}
	}
	return false, nil
}
