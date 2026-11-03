/*
 * @Author: kamalyes 501893067@qq.com
 * @Date: 2026-11-03 16:20:00
 * @LastEditors: kamalyes 501893067@qq.com
 * @LastEditTime: 2026-11-03 16:20:00
 * @FilePath: \kronos-codegen\pbfield\model.go
 * @Description: 把 proto 字段表映射成 GORM 模型字段
 *
 * 映射规则不是臆想的，是照这套生态里现有的模型反推出来的：*_id 用 varchar(36)、
 * 普通字符串给 not null default ''、created_at / updated_at 用
 * autoCreateTime / autoUpdateTime
 *
 * 有两类字段落不到单列上，这里给出的是默认选择而不是唯一答案：
 *  - repeated 落 JSON 列（serializer:json）而不是拆关联表：关联表要额外的表名、
 *    外键与迁移，生成器猜不出业务是否真的需要可查询的关联；JSON 列先把数据存住，
 *    确实要查再手工拆
 *  - enum 保留 protoc 给的枚举类型并存成整数：存字符串可读但改名就破数据
 *
 * 模型文件生成之后归项目所有，这两处逐字段改掉即可
 *
 * [EN] The mapping rules are derived from the models this ecosystem already has rather
 * than invented
 * Two kinds of field do not fit a single column, and what follows is a default rather than
 * the only answer: repeated fields become a JSON column instead of a join table, since a
 * join table needs its own name, foreign keys and migration and the generator cannot tell
 * whether the business actually needs the association to be queryable, while an enum keeps
 * protoc's type and is stored as an integer, since storing the name reads better but breaks
 * the data on a rename
 * The model file belongs to the project once generated, so either choice can be changed
 * field by field
 *
 * Copyright (c) 2026 by kamalyes, All Rights Reserved.
 */

package pbfield

import (
	"sort"
	"strings"
)

// ModelField 一个 GORM 模型字段
// [EN] One GORM model field
type ModelField struct {
	Name    string // Go 字段名，Id 已归一成 ID
	Type    string // Go 类型
	Column  string // 列名
	Tag     string // gorm tag 的内容（不含 gorm:"" 外壳）
	JSONTag string // json tag 的内容
	Comment string // 行尾注释，原样保留
}

// 索引规则刻意不认任何具体列名
//
// 早先这里写死了 tenant_id 与 platform_id —— 那是把某一个项目的租户模型焊进了
// 一个通用工具 别的项目用 org_id、或者压根没有租户维度，它们不该因为工具的
// 出身而多出两个无意义的索引判断
//
// 现在的规则只看名字形态：非主键的 *_id 列加索引 这类列按命名惯例就是外键，
// 而「以 _id 结尾」是名字的属性，不是某个项目的业务约定
// 要精确控制就用 --var IndexColumns=a_id,b_id 显式给一份清单
//
// [EN] The index rule deliberately knows no specific column name
// It used to hardcode tenant_id and platform_id, which welded one project's tenancy model into a
// general tool; a project keyed on org_id, or with no tenancy at all, should not inherit
// two meaningless checks from the tool's origin
// The rule now looks only at the shape of the name: a non-primary column ending in _id
// gets an index, since by naming convention such columns are foreign keys and the suffix
// is a property of the name rather than any project's business convention
// Pass --var IndexColumns=a_id,b_id to give an explicit list instead

// Model 把消息字段表映射成模型字段
// entitySnake 是实体名的 snake 形态，用来认出主键列（{entity}_id）
// indexColumns 为空时按「非主键的 *_id 列」加索引，非空时只认这份清单
//
// [EN] Model maps the message's field table to model fields, with entitySnake naming the
// entity so the primary key column {entity}_id can be recognised
func Model(msg *Message, entitySnake string, indexColumns []string) []ModelField {
	explicit := map[string]bool{}
	for _, c := range indexColumns {
		if c = strings.TrimSpace(c); c != "" {
			explicit[c] = true
		}
	}
	out := make([]ModelField, 0, len(msg.Fields))
	primaryTaken := false
	primaryColumn := entitySnake + "_id"
	for _, f := range msg.Fields {
		mf := ModelField{
			Name:    goFieldName(f.GoName),
			Column:  f.ProtoName,
			JSONTag: f.ProtoName,
			Comment: f.Comment,
		}
		parts := []string{"column:" + f.ProtoName}

		switch {
		case f.Repeated:
			mf.Type = f.GoType
			parts = append(parts, "serializer:json")

		case f.GoType == "*timestamppb.Timestamp":
			mf.Type = "time.Time"
			switch f.ProtoName {
			case "created_at":
				parts = append(parts, "autoCreateTime")
			case "updated_at":
				parts = append(parts, "autoUpdateTime")
			default:
				parts = append(parts, "type:datetime")
			}

		case f.GoType == "string":
			if f.ProtoName == entitySnake+"_id" && !primaryTaken {
				parts = append(parts, "type:varchar(36)", "primaryKey")
				primaryTaken = true
				primaryColumn = f.ProtoName
			} else if strings.HasSuffix(f.ProtoName, "_id") {
				parts = append(parts, "type:varchar(36)", "not null", "default:''")
			} else {
				parts = append(parts, "type:varchar(255)", "not null", "default:''")
			}
			mf.Type = "string"

		case f.GoType == "bool":
			mf.Type = "bool"
			parts = append(parts, "not null", "default:false")

		case strings.Contains(f.GoType, "."):
			// 带包名的类型，在本生态里就是 protoc 生成的枚举
			// [EN] A qualified type, which in this ecosystem means a protoc enum
			mf.Type = f.GoType
			parts = append(parts, "type:int", "not null", "default:0")

		default:
			// int32 / int64 / float 之类的定宽标量
			// [EN] Fixed-width scalars such as int32, int64 and the float types
			mf.Type = f.GoType
			parts = append(parts, "not null", "default:0")
		}

		if wantIndex(f, explicit, primaryColumn) {
			parts = append(parts, "index")
		}
		mf.Tag = strings.Join(parts, ";")
		out = append(out, mf)
	}
	return out
}

// goFieldName 把 protoc 的字段名归一成本生态的写法
//
// protoc 产出 RoleId / TenantId，而这套生态的模型写的是 RoleID / TenantID
// 不归一的话同一个实体在 pb 与 model 两侧字段名不一致，go-pbmo 的 EqualFold
// 映射仍然能对上，但读代码的人会以为是两个字段
//
// [EN] protoc produces RoleId and TenantId while this ecosystem's models write RoleID
// and TenantID
// Leaving them unnormalized keeps go-pbmo's EqualFold mapping working but makes a reader
// think the pb and model sides are different fields
func goFieldName(s string) string {
	// 显式给出每个后缀的目标写法，而不是对后缀整体 ToUpper
	// ToUpper("Ids") 是 "IDS"，于是 PermissionIds 会变成 PermissionIDS
	//
	// [EN] Each suffix states its target spelling rather than being upper-cased wholesale,
	// since ToUpper("Ids") is "IDS" and would turn PermissionIds into PermissionIDS
	for _, r := range []struct{ from, to string }{
		{"Ids", "IDs"},
		{"Id", "ID"},
		{"Urls", "URLs"},
		{"Url", "URL"},
		{"Uri", "URI"},
		{"Api", "API"},
		{"Http", "HTTP"},
		{"Json", "JSON"},
	} {
		if strings.HasSuffix(s, r.from) {
			return s[:len(s)-len(r.from)] + r.to
		}
	}
	return s
}

// wantIndex 判断一个字段要不要加索引
// [EN] wantIndex decides whether a field gets an index
func wantIndex(f Field, explicit map[string]bool, primaryColumn string) bool {
	if len(explicit) > 0 {
		return explicit[f.ProtoName]
	}
	if f.Repeated || f.ProtoName == primaryColumn {
		return false
	}
	return strings.HasSuffix(f.ProtoName, "_id")
}

// ModelImports 算出模型文件需要的导入
//
// 只看字段类型，不猜：time.Time 来自标准库 time，带包名的类型（本生态里是
// protoc 生成的枚举）从消息所在 .pb.go 的导入表里查出真实包路径
// 查不到的限定名不编进导入，让它在编译期暴露出来 —— 悄悄省掉一条导入，
// 错误会以「undefined」的形态出现在别处，更难定位
//
// [EN] ModelImports computes the imports the model file needs by looking only at the field
// types: time.Time comes from the standard library, while a qualified type, which in this
// ecosystem means a protoc enum, is resolved to its real package path through the import
// table of the .pb.go the message came from
// A qualified name that cannot be resolved is left out so the compiler reports it, since
// silently dropping an import makes the failure surface elsewhere as "undefined"
func ModelImports(fields []ModelField, msgImports map[string]string) []string {
	seen := map[string]bool{}
	var out []string
	add := func(path string) {
		if path == "" || seen[path] {
			return
		}
		seen[path] = true
		out = append(out, path)
	}
	for _, f := range fields {
		t := strings.TrimPrefix(strings.TrimPrefix(f.Type, "[]"), "*")
		if t == "time.Time" {
			add("time")
			continue
		}
		if i := strings.Index(t, "."); i > 0 {
			add(msgImports[t[:i]])
		}
	}
	sort.Strings(out)
	return out
}
