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
	"io/fs"
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
	modRoot, module, err := moduleOf(dir)
	if err != nil {
		return nil, err
	}
	c, _, err := contractOf(modRoot, module, dir)
	return c, err
}

// moduleOf 往上找 go.mod，返回模块根目录与模块路径
// 契约通常是独立仓库，模块路径只有它自己的 go.mod 知道
//
// [EN] moduleOf walks up for go.mod and returns the module root and path, since the
// contract is usually its own repository and only its go.mod knows the module path
func moduleOf(dir string) (string, string, error) {
	modRoot := dir
	for {
		if _, err := os.Stat(filepath.Join(modRoot, "go.mod")); err == nil {
			break
		}
		parent := filepath.Dir(modRoot)
		if parent == modRoot {
			return "", "", fmt.Errorf("从 %s 往上没找到 go.mod，无法推出契约包的导入路径", dir)
		}
		modRoot = parent
	}
	raw, err := os.ReadFile(filepath.Join(modRoot, "go.mod"))
	if err != nil {
		return "", "", err
	}
	for _, line := range strings.Split(string(raw), "\n") {
		if rest, ok := strings.CutPrefix(strings.TrimSpace(line), "module "); ok {
			if m := strings.TrimSpace(rest); m != "" {
				return modRoot, m, nil
			}
		}
	}
	return "", "", fmt.Errorf("%s 里没有 module 声明", filepath.Join(modRoot, "go.mod"))
}

// contractOf 把一个包目录换算成导入路径与别名
// [EN] contractOf turns a package directory into its import path and alias
func contractOf(modRoot, module, dir string) (*Contract, bool, error) {
	rel, err := filepath.Rel(modRoot, dir)
	if err != nil {
		return nil, false, err
	}
	importPath := module
	if rel != "." {
		importPath = module + "/" + filepath.ToSlash(rel)
	}
	alias := strings.ReplaceAll(filepath.Base(dir), "_", "") + "pb"
	return &Contract{Import: importPath, Alias: alias}, true, nil
}

// FindGRPCService 在整个契约里找名为 <name>Service 的 gRPC 服务，并给出它所在的包
//
// 判据是 *_grpc.pb.go 里有没有 Unimplemented<name>ServiceServer —— 这个类型是
// protoc-gen-go-grpc 为每个 service 生成的
//
// 为什么要搜整棵树而不是只看 --pb 同目录
//
// 真实契约里消息与服务常常不在一个包 实测渲染出来的契约:ProductInfo 在
// pb/common/catalog.pb.go，而 UnimplementedProductServiceServer 在
// pb/catalog/product_service_grpc.pb.go 只看同目录会判成「没有服务」，于是生成的
// 服务不嵌未实现桩 —— 明明契约里有
//
// 返回的包也必须是**服务所在**的那个，而不是消息所在的那个：嵌桩要导入的是前者
// 这两者在上面那个例子里分别是 pb/catalog 与 pb/common，拿错了编译不过
//
// 搜索范围限定在 pb 目录（找不到 pb 这一层就退回契约模块根）：契约仓库里只有这一
// 棵树是 protoc 的产出，扫整个仓库会把测试夹具之类一起算进来
//
// [EN] FindGRPCService locates a gRPC service named <name>Service anywhere in the contract
// and reports which package holds it, judged by Unimplemented<name>ServiceServer in a
// *_grpc.pb.go, the type protoc-gen-go-grpc emits per service
// The whole tree is searched rather than just the --pb directory because a real contract
// often keeps messages and services in different packages: measured on a rendered contract,
// ProductInfo sits in pb/common/catalog.pb.go while UnimplementedProductServiceServer sits
// in pb/catalog/product_service_grpc.pb.go, so looking only beside the message would report
// no service and the generated service would omit the stub the contract does define
// The package returned is the one holding the service rather than the one holding the
// message, since the stub is imported from the former, and those are pb/catalog and
// pb/common in that example, so getting it wrong does not compile
// The search is confined to the pb directory, falling back to the contract module root when
// there is no such level, because that tree is protoc's output and scanning the whole
// repository would pull in fixtures and the like
func FindGRPCService(pbPath, name string) (*Contract, bool, error) {
	abs, err := filepath.Abs(pbPath)
	if err != nil {
		return nil, false, err
	}
	modRoot, module, err := moduleOf(filepath.Dir(abs))
	if err != nil {
		return nil, false, err
	}

	searchRoot := modRoot
	if pbDir := pbRootOf(filepath.Dir(abs), modRoot); pbDir != "" {
		searchRoot = pbDir
	}

	needle := "type Unimplemented" + name + "ServiceServer struct"
	var found string
	err = filepath.WalkDir(searchRoot, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if found != "" || d.IsDir() || !strings.HasSuffix(d.Name(), "_grpc.pb.go") {
			return nil
		}
		raw, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		if strings.Contains(string(raw), needle) {
			found = filepath.Dir(path)
		}
		return nil
	})
	if err != nil {
		return nil, false, err
	}
	if found == "" {
		return nil, false, nil
	}
	return contractOf(modRoot, module, found)
}

// pbRootOf 从 .pb.go 所在目录往上找名为 pb 的那一层，找不到返回空串
// [EN] pbRootOf walks up from the .pb.go's directory for a level named pb, or returns empty
func pbRootOf(dir, modRoot string) string {
	for d := dir; strings.HasPrefix(d, modRoot); d = filepath.Dir(d) {
		if filepath.Base(d) == "pb" {
			return d
		}
		if d == modRoot {
			break
		}
	}
	return ""
}
