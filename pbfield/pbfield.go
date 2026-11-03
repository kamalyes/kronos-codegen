/*
 * @Author: kamalyes 501893067@qq.com
 * @Date: 2026-11-03 16:00:00
 * @LastEditors: kamalyes 501893067@qq.com
 * @LastEditTime: 2026-11-03 16:00:00
 * @FilePath: \kronos-codegen\pbfield\pbfield.go
 * @Description: 从 protoc 生成的 .pb.go 里提取消息字段，作为生成模型的字段真相源
 *
 * 为什么读 .pb.go 而不是 .proto
 *
 * 两者携带的信息一样多：字段名、字段号、repeated、注释，在 .pb.go 里分别落在
 * 结构体字段名、protobuf tag 与行尾注释上 但代价差很多：
 *
 *  1. 解析 .proto 要一套 proto 文法 —— import、option、oneof、map、嵌套消息、
 *     保留字段，手写解析器会被其中任何一样打穿；引第三方解析器则给这个零依赖
 *     的工具加上一条重依赖
 *  2. 更要紧的是类型映射 读 .proto 就得自己实现并长期维护一套 proto→Go 的映射
 *     （google.protobuf.Timestamp 映射到什么、enum 映射到什么、repeated 怎么表达），
 *     而 protoc 已经把这件事做完了 读 .pb.go 等于直接用 protoc 的答案，
 *     不可能和它不一致
 *
 * 代价是 .pb.go 必须已经生成 这个前提在本生态里成立：契约仓库里 proto 与 pb
 * 是一起提交的
 *
 * [EN] Why this reads .pb.go rather than .proto
 * Both carry the same information: field names, numbers, repeatedness and comments, which
 * in .pb.go live in the struct field names, the protobuf tag and the trailing comments
 * The costs differ sharply: parsing .proto needs a proto grammar, where imports, options,
 * oneof, maps, nested messages and reserved fields will each break a hand-rolled parser,
 * while a third-party parser adds a heavy dependency to a tool that currently has none
 * More importantly, reading .proto means implementing and maintaining a proto-to-Go type
 * mapping, which protoc has already done; reading .pb.go uses protoc's own answer and
 * cannot disagree with it
 * The cost is that .pb.go must already exist, which holds here since the contract
 * repository commits proto and pb together
 *
 * Copyright (c) 2026 by kamalyes, All Rights Reserved.
 */

package pbfield

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"reflect"
	"strings"
)

// Field 一个消息字段
// [EN] One message field
type Field struct {
	// GoName protoc 生成的 Go 字段名，如 RoleId
	// [EN] GoName is the Go field name protoc produced, such as RoleId
	GoName string
	// GoType protoc 给出的 Go 类型，如 string、int32、enums.DomainType、*timestamppb.Timestamp
	// [EN] GoType is the Go type protoc produced
	GoType string
	// ProtoName proto 里的字段名，如 role_id —— 这正好是数据库列名该用的形态
	// [EN] ProtoName is the proto field name, which is the shape a database column wants
	ProtoName string
	// Number proto 字段号
	// [EN] Number is the proto field number
	Number int
	// Repeated 是否 repeated 字段
	// [EN] Repeated reports whether the field is repeated
	Repeated bool
	// Comment 行尾注释，原样保留（本生态的注释是双语的，不该在生成时被丢掉）
	// [EN] Comment is the trailing comment kept verbatim, since this ecosystem's comments
	// are bilingual and should not be dropped during generation
	Comment string
}

// Message 一个消息的字段表
// [EN] The field table of one message
type Message struct {
	Name   string
	Fields []Field
	// Imports .pb.go 自己的导入表，别名 -> 包路径
	//
	// 字段类型里的 enums.DomainType 这类限定名，别名解析成哪个包只有这张表知道
	// 生成模型时要照抄这条导入，否则生成出来的文件编译不过
	// 猜路径是不行的：同一个 enums 别名在不同契约仓库指向不同的包
	//
	// [EN] Imports is the .pb.go file's own import table, mapping alias to package path
	// Only this table knows which package an alias in a qualified field type such as
	// enums.DomainType resolves to, and the generated model has to carry the same import
	// or it will not compile
	// Guessing the path does not work, since the same enums alias points at different
	// packages in different contract repositories
	Imports map[string]string
}

// skipFields protoc 自己的内部字段，不属于业务字段
// [EN] protoc's own internal fields, which are not business fields
var skipFields = map[string]bool{
	"state":         true,
	"sizeCache":     true,
	"unknownFields": true,
	"XXX_sizecache": true,
}

// Parse 从 .pb.go 文件里取出指定消息的字段表
// [EN] Parse extracts the named message's field table from a .pb.go file
func Parse(path, message string) (*Message, error) {
	fset := token.NewFileSet()
	// 带注释解析：行尾注释是字段语义的唯一来源，丢了就只剩名字
	// [EN] Comments are parsed because the trailing comment is the only carrier of a
	// field's meaning; dropping it leaves nothing but the name
	file, err := parser.ParseFile(fset, path, nil, parser.ParseComments)
	if err != nil {
		return nil, fmt.Errorf("解析 %s 失败: %w", path, err)
	}

	var st *ast.StructType
	for _, d := range file.Decls {
		gd, ok := d.(*ast.GenDecl)
		if !ok || gd.Tok != token.TYPE {
			continue
		}
		for _, spec := range gd.Specs {
			ts, ok := spec.(*ast.TypeSpec)
			if !ok || ts.Name.Name != message {
				continue
			}
			s, ok := ts.Type.(*ast.StructType)
			if !ok {
				return nil, fmt.Errorf("%s 里的 %s 不是结构体", path, message)
			}
			st = s
		}
	}
	if st == nil {
		return nil, fmt.Errorf("%s 里没有消息 %s", path, message)
	}

	out := &Message{Name: message, Imports: fileImports(file)}
	for _, f := range st.Fields.List {
		if len(f.Names) != 1 {
			continue
		}
		name := f.Names[0].Name
		if skipFields[name] || !ast.IsExported(name) {
			continue
		}
		tag := ""
		if f.Tag != nil {
			tag = strings.Trim(f.Tag.Value, "`")
		}
		protoName, number, repeated := parseProtobufTag(tag)
		if protoName == "" {
			// 没有 protobuf tag 的导出字段不是 proto 字段
			// [EN] An exported field without a protobuf tag is not a proto field
			continue
		}
		out.Fields = append(out.Fields, Field{
			GoName:    name,
			GoType:    typeString(f.Type),
			ProtoName: protoName,
			Number:    number,
			Repeated:  repeated,
			Comment:   trailingComment(f),
		})
	}
	if len(out.Fields) == 0 {
		return nil, fmt.Errorf("%s 的 %s 没有任何 proto 字段", path, message)
	}
	return out, nil
}

// parseProtobufTag 从 protobuf tag 里取字段名、字段号与 repeated
// tag 形如 bytes,10,rep,name=permission_ids,json=permissionIds,proto3
//
// [EN] parseProtobufTag reads the field name, number and repeatedness out of the
// protobuf tag, which looks like bytes,10,rep,name=permission_ids,json=permissionIds,proto3
func parseProtobufTag(tag string) (protoName string, number int, repeated bool) {
	raw := reflect.StructTag(tag).Get("protobuf")
	if raw == "" {
		return "", 0, false
	}
	for i, part := range strings.Split(raw, ",") {
		switch {
		case i == 1:
			_, _ = fmt.Sscanf(part, "%d", &number)
		case part == "rep":
			repeated = true
		case strings.HasPrefix(part, "name="):
			protoName = strings.TrimPrefix(part, "name=")
		}
	}
	return protoName, number, repeated
}

// typeString 把类型表达式还原成源码形态
// [EN] typeString renders a type expression back to source form
func typeString(e ast.Expr) string {
	switch t := e.(type) {
	case *ast.Ident:
		return t.Name
	case *ast.StarExpr:
		return "*" + typeString(t.X)
	case *ast.ArrayType:
		return "[]" + typeString(t.Elt)
	case *ast.SelectorExpr:
		return typeString(t.X) + "." + t.Sel.Name
	case *ast.MapType:
		return "map[" + typeString(t.Key) + "]" + typeString(t.Value)
	}
	return "any"
}

// trailingComment 取字段的行尾注释，去掉前导斜杠与空白
// [EN] trailingComment returns the field's trailing comment without the slashes
func trailingComment(f *ast.Field) string {
	if f.Comment == nil || len(f.Comment.List) == 0 {
		return ""
	}
	var parts []string
	for _, c := range f.Comment.List {
		parts = append(parts, strings.TrimSpace(strings.TrimPrefix(strings.TrimPrefix(c.Text, "//"), "/*")))
	}
	return strings.TrimSpace(strings.TrimSuffix(strings.Join(parts, " "), "*/"))
}

// fileImports 取文件的导入表，别名 -> 包路径
// 没写别名时用包路径的最后一段，这与 Go 的默认行为一致
//
// [EN] fileImports returns the file's import table as alias to package path
// An import without an explicit alias uses the last path segment, matching Go's default
func fileImports(file *ast.File) map[string]string {
	out := map[string]string{}
	for _, im := range file.Imports {
		path := strings.Trim(im.Path.Value, `"`)
		alias := path
		if i := strings.LastIndex(path, "/"); i >= 0 {
			alias = path[i+1:]
		}
		if im.Name != nil {
			alias = im.Name.Name
		}
		out[alias] = path
	}
	return out
}
