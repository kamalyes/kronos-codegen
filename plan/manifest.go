/*
 * @Author: kamalyes 501893067@qq.com
 * @Date: 2026-11-01 16:20:11
 * @LastEditors: kamalyes 501893067@qq.com
 * @LastEditTime: 2026-11-01 20:10:35
 * @FilePath: \kronos-codegen\plan\manifest.go
 * @Description: 声明式补丁计划 - 把 JSON 清单展开成 patch.Patch 列表
 *
 * Copyright (c) 2026 by kamalyes, All Rights Reserved.
 */

// Package plan 把「往哪个文件的哪个锚点插什么」这件事从 Go 代码里挪到 JSON 清单里
// 这样业务模板的形态可以单独演进，补丁引擎本身不需要知道任何业务细节
//
// 用法 / Usage:
//
//	m, err := plan.Load("/path/to/project/.kronos-codegen.json")
//	if err != nil {
//		return err
//	}
//	vars := plan.Vars("OrderItem") // Name/Lower/Snake/Kebab/Upper/Flat/Delimited
//	actions, err := m.Actions("entity", vars, "/path/to/project")
//
// 清单里每个步骤的所有字符串字段都会先过一遍 text/template，变量来自清单的 vars、
// plan.Vars 的命名派生与命令行的 --var；引用了不存在的变量会直接报错而不是悄悄留空
//
// [EN] Package plan moves the "insert what, where" decision into a JSON manifest so the
// business templates can evolve independently of the patch engine. Every string field of
// a step is expanded as a text/template first, with variables coming from the manifest's
// own vars, plan.Vars naming derivations and the command line; an unknown variable fails
// loudly instead of silently expanding to nothing.
package plan

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"text/template"

	"github.com/kamalyes/kronos-codegen/patch"
)

// ManifestVersion 当前支持的清单版本
// [EN] ManifestVersion is the manifest schema version understood by this build
const ManifestVersion = 1

// DefaultFileName 项目根目录下的默认清单文件名
// [EN] DefaultFileName is the default manifest file name at the project root
const DefaultFileName = ".kronos-codegen.json"

// Manifest 一份补丁计划清单
// [EN] Manifest is one declarative patch plan
type Manifest struct {
	// Version 清单版本
	// [EN] Version is the manifest schema version
	Version int `json:"version"`
	// Vars 清单级默认变量，会被命令行的 --var 覆盖
	// [EN] Vars holds manifest level defaults, overridden by --var on the command line
	Vars map[string]string `json:"vars,omitempty"`
	// Sections 按命令分组的步骤，如 entity
	// [EN] Sections groups steps per command, e.g. entity
	Sections map[string][]Step `json:"sections"`
}

// Step 一条补丁步骤，字段是所有补丁类型的并集，由 Kind 决定取哪些
// 所有字符串字段都会先过一遍 text/template，变量来自 Vars 与命令行
// [EN] Step is one patch step; its fields are the union of every patch kind and Kind
// selects which ones apply. Every string field is expanded as a text/template first.
type Step struct {
	// Kind 步骤类型：create_file|const|var|type|import|struct_field|composite_field|slice_append|stmt|json_key
	// [EN] Kind is the step kind
	Kind string `json:"kind"`
	// File 目标文件相对路径
	// [EN] File is the target file path
	File string `json:"file,omitempty"`
	// Files json_key 专用，支持 glob，用来一次覆盖所有语言文件
	// [EN] Files is json_key only and accepts globs to fan out across locale files
	Files []string `json:"files,omitempty"`
	// Marker 标记注释锚点
	// [EN] Marker is the marker comment anchor
	Marker string `json:"marker,omitempty"`
	// Member 块内已有成员名锚点
	// [EN] Member is the existing-member anchor
	Member string `json:"member,omitempty"`
	// After stmt 专用：为 true 时紧跟 near 那条语句之后插入，而不是追加到块尾
	// [EN] After is stmt only: insert right below the near statement instead of at block end
	After bool `json:"after,omitempty"`
	// Near 已有兄弟条目锚点，用于 slice_append、stmt 与 import
	// import 下它是同组里已有的某个导入路径，新条目落到那一组的末尾
	// [EN] Near is the existing-sibling anchor for slice_append, stmt and import; for
	// import it names an existing path whose blank-line group the new entry joins
	Near string `json:"near,omitempty"`
	// Import 导入路径，import 专用
	// [EN] Import is the import path, used by kind=import only
	Import string `json:"import,omitempty"`
	// Alias 导入别名，import 专用
	// [EN] Alias is the import alias, used by kind=import only
	Alias string `json:"alias,omitempty"`
	// Struct 结构体名
	// [EN] Struct is the struct type name
	Struct string `json:"struct,omitempty"`
	// Func 函数名
	// [EN] Func is the function name
	Func string `json:"func,omitempty"`
	// Recv 方法接收者类型名
	// [EN] Recv is the receiver type name
	Recv string `json:"recv,omitempty"`
	// Var 包级变量名
	// [EN] Var is the package level variable name
	Var string `json:"var,omitempty"`
	// Type 类型文本，用于复合字面量消歧或字段类型
	// [EN] Type is a type expression used to disambiguate or to type a field
	Type string `json:"type,omitempty"`
	// Parent json_key 的父对象路径
	// [EN] Parent is the json_key parent object path
	Parent []string `json:"parent,omitempty"`
	// Name 常量名或字段名
	// [EN] Name is the constant or field name
	Name string `json:"name,omitempty"`
	// Key 复合字面量的键或 JSON 键
	// [EN] Key is a composite literal key or a JSON key
	Key string `json:"key,omitempty"`
	// Value 值表达式或 JSON 字面量
	// [EN] Value is a value expression or a JSON literal
	Value string `json:"value,omitempty"`
	// Element 切片元素表达式
	// [EN] Element is a slice element expression
	Element string `json:"element,omitempty"`
	// Stmt 语句源码
	// [EN] Stmt is statement source
	Stmt string `json:"stmt,omitempty"`
	// Tag 结构体 tag
	// [EN] Tag is a struct tag
	Tag string `json:"tag,omitempty"`
	// Doc 插入内容上方的注释
	// [EN] Doc is the comment placed above the inserted content
	Doc string `json:"doc,omitempty"`
	// Template create_file 专用：项目内模板文件的相对路径，内容会用同一套变量渲染
	// [EN] Template is create_file only: a project-relative template path rendered with the same vars
	Template string `json:"template,omitempty"`
	// Content create_file 专用：直接给出新文件的内容，与 template 二选一
	// [EN] Content is create_file only: inline content for the new file, exclusive with template
	Content string `json:"content,omitempty"`
	// Values json_key 按语言给出的译文，键是文件名去掉扩展名后的语言码
	// [EN] Values maps a locale code to its translation for json_key
	Values map[string]string `json:"values,omitempty"`
	// Fallback Values 里缺失的语言用的兜底译文
	// [EN] Fallback is used for locales missing from Values
	Fallback string `json:"fallback,omitempty"`
}

// Load 从磁盘读入清单并做基础校验
// [EN] Load reads a manifest from disk and validates it
func Load(path string) (*Manifest, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("读取补丁计划 %s 失败: %w", path, err)
	}
	var m Manifest
	dec := json.NewDecoder(strings.NewReader(string(raw)))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&m); err != nil {
		return nil, fmt.Errorf("解析补丁计划 %s 失败: %w", path, err)
	}
	if m.Version != ManifestVersion {
		return nil, fmt.Errorf("补丁计划 %s 的 version 是 %d，当前只支持 %d", path, m.Version, ManifestVersion)
	}
	if len(m.Sections) == 0 {
		return nil, fmt.Errorf("补丁计划 %s 里没有任何 sections", path)
	}
	for name, steps := range m.Sections {
		for i, s := range steps {
			if err := s.validate(); err != nil {
				return nil, fmt.Errorf("补丁计划 %s 的 sections.%s[%d] 不合法: %w", path, name, i, err)
			}
		}
	}
	return &m, nil
}

// SectionNames 返回所有分组名，按字母序
// [EN] SectionNames returns every section name in alphabetical order
func (m *Manifest) SectionNames() []string {
	out := make([]string, 0, len(m.Sections))
	for k := range m.Sections {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// validate 校验步骤的必填字段
// [EN] validate checks the mandatory fields of a step
func (s Step) validate() error {
	switch s.Kind {
	case "const", "var":
		if s.Name == "" || s.Value == "" {
			return fmt.Errorf("kind=%s 需要 name 与 value", s.Kind)
		}
	case "type":
		if s.Name == "" {
			return fmt.Errorf("kind=type 需要 name")
		}
		if (s.Value == "") == (s.Type == "") {
			return fmt.Errorf("kind=type 需要 value（别名）或 type（类型定义）之一，且不能同时给")
		}
	case "import":
		if s.Import == "" {
			return fmt.Errorf("kind=import 需要 import")
		}
	case "create_file":
		if (s.Template == "") == (s.Content == "") {
			return fmt.Errorf("kind=create_file 需要 template（项目内模板路径）或 content（内联内容）之一，且不能同时给")
		}
	case "struct_field":
		if s.Struct == "" || s.Name == "" || s.Type == "" {
			return fmt.Errorf("kind=struct_field 需要 struct、name 与 type")
		}
	case "composite_field":
		if s.Func == "" || s.Key == "" || s.Value == "" {
			return fmt.Errorf("kind=composite_field 需要 func、key 与 value")
		}
	case "slice_append":
		if s.Element == "" {
			return fmt.Errorf("kind=slice_append 需要 element")
		}
	case "stmt":
		if s.Stmt == "" {
			return fmt.Errorf("kind=stmt 需要 stmt")
		}
	case "json_key":
		if s.Key == "" {
			return fmt.Errorf("kind=json_key 需要 key")
		}
		if s.File == "" && len(s.Files) == 0 {
			return fmt.Errorf("kind=json_key 需要 file 或 files")
		}
		if s.Value == "" && s.Fallback == "" && len(s.Values) == 0 {
			return fmt.Errorf("kind=json_key 需要 value（原始 JSON 字面量）或 values / fallback（按语言的译文），否则会插入空字符串")
		}
	case "":
		return fmt.Errorf("缺少 kind")
	default:
		return fmt.Errorf("未知的 kind %q，可选 create_file|const|var|type|import|struct_field|composite_field|slice_append|stmt|json_key", s.Kind)
	}
	if s.Kind != "json_key" && s.File == "" {
		return fmt.Errorf("kind=%s 需要 file", s.Kind)
	}
	return nil
}

// Actions 把某个分组展开成步骤列表，vars 用于模板替换，root 用于解析 glob 与读模板文件
//
// create_file 步骤**总是排在最前面**，与它在清单里的书写位置无关（同类之间保持书写顺序）
// 这样 dry-run 读起来是「先建了哪些文件，再往哪些文件插了什么」，而不是交错着看
// [EN] Actions expands one section into steps. create_file steps are always hoisted to the
// front regardless of where they appear in the manifest (order is kept within each group),
// so a dry-run reads as "these files were created, then these entries were inserted".
func (m *Manifest) Actions(section string, vars map[string]string, root string) ([]patch.Action, error) {
	steps, ok := m.Sections[section]
	if !ok {
		return nil, fmt.Errorf("补丁计划里没有 sections.%s，现有分组: %s", section, strings.Join(m.SectionNames(), ", "))
	}
	merged := map[string]string{}
	for k, v := range m.Vars {
		merged[k] = v
	}
	for k, v := range vars {
		merged[k] = v
	}

	var creates, patches []patch.Action
	for i, raw := range steps {
		s, err := expandStep(raw, merged)
		if err != nil {
			return nil, fmt.Errorf("sections.%s[%d] 模板展开失败: %w", section, i, err)
		}
		as, err := s.actions(root, merged)
		if err != nil {
			return nil, fmt.Errorf("sections.%s[%d] 展开失败: %w", section, i, err)
		}
		if s.Kind == "create_file" {
			creates = append(creates, as...)
			continue
		}
		patches = append(patches, as...)
	}
	return append(creates, patches...), nil
}

// actions 把一条步骤展开成一条或多条引擎步骤
// [EN] actions expands one step into one or more engine actions
func (s Step) actions(root string, vars map[string]string) ([]patch.Action, error) {
	switch s.Kind {
	case "create_file":
		content, origin, err := s.newFileContent(root, vars)
		if err != nil {
			return nil, err
		}
		return []patch.Action{&patch.FileCreate{
			Path: s.File, Content: content, Origin: origin,
		}}, nil
	case "import":
		return []patch.Action{&patch.ImportInsert{
			Path: s.File, Import: s.Import, Alias: s.Alias, Near: s.Near, Doc: s.Doc,
		}}, nil
	case "const", "var", "type":
		return []patch.Action{&patch.DeclInsert{
			Path:  s.File,
			Decl:  patch.DeclKind(s.Kind),
			Block: patch.BlockRef{Marker: s.Marker, Member: s.Member},
			Name:  s.Name,
			Type:  s.Type,
			Value: s.Value,
			Doc:   s.Doc,
		}}, nil
	case "struct_field":
		return []patch.Action{&patch.StructFieldInsert{
			Path: s.File, Struct: s.Struct, Name: s.Name, Type: s.Type, Tag: s.Tag, Doc: s.Doc,
		}}, nil
	case "composite_field":
		return []patch.Action{&patch.CompositeFieldInsert{
			Path: s.File, Func: s.Func, Recv: s.Recv, Type: s.Type,
			Key: s.Key, Value: s.Value, Doc: s.Doc,
		}}, nil
	case "slice_append":
		return []patch.Action{&patch.SliceAppend{
			Path: s.File, Marker: s.Marker, Near: s.Near, Var: s.Var, Func: s.Func, Recv: s.Recv,
			Type: s.Type, Element: s.Element, Doc: s.Doc,
		}}, nil
	case "stmt":
		return []patch.Action{&patch.StmtInsert{
			Path: s.File, Marker: s.Marker, Near: s.Near, After: s.After,
			Func: s.Func, Recv: s.Recv, Stmt: s.Stmt, Doc: s.Doc,
		}}, nil
	case "json_key":
		files, err := s.resolveFiles(root)
		if err != nil {
			return nil, err
		}
		if s.Value != "" {
			out := make([]patch.Action, 0, len(files))
			for _, f := range files {
				out = append(out, &patch.JSONKeyInsert{Path: f, Parent: s.Parent, Key: s.Key, Value: s.Value})
			}
			return out, nil
		}
		locale := patch.LocaleKeys(files, s.Parent, s.Key, s.Values, s.Fallback)
		out := make([]patch.Action, 0, len(locale))
		for _, p := range locale {
			out = append(out, p)
		}
		return out, nil
	default:
		return nil, fmt.Errorf("未知的 kind %q", s.Kind)
	}
}

// newFileContent 取出 create_file 的新文件内容
// template 走项目内的模板文件，内容用同一套变量渲染；content 则是已经展开好的内联内容
// 模板文件的读取复用 patch.DirStore，顺带拿到它挡 ../ 越界的那套校验
// [EN] newFileContent produces the content of a create_file step. A template is read from
// the project and rendered with the same variables; inline content is already expanded.
// Reading goes through patch.DirStore, which brings its root-escape guard along.
func (s Step) newFileContent(root string, vars map[string]string) (content []byte, origin string, err error) {
	if s.Content != "" {
		return []byte(s.Content), "内联内容", nil
	}
	raw, err := patch.NewDirStore(root).Read(s.Template)
	if err != nil {
		return nil, "", fmt.Errorf("读取模板 %s 失败: %w", s.Template, err)
	}
	tpl, err := template.New(path.Base(s.Template)).Option("missingkey=error").Parse(string(raw))
	if err != nil {
		return nil, "", fmt.Errorf("模板 %s 不合法: %w", s.Template, err)
	}
	var b bytes.Buffer
	if err := tpl.Execute(&b, vars); err != nil {
		return nil, "", fmt.Errorf("模板 %s 渲染失败: %w", s.Template, err)
	}
	return b.Bytes(), s.Template, nil
}

// resolveFiles 解析 json_key 的目标文件集合，files 支持 glob
// [EN] resolveFiles resolves the json_key target files, expanding globs
func (s Step) resolveFiles(root string) ([]string, error) {
	if s.File != "" {
		return []string{s.File}, nil
	}
	var out []string
	for _, pattern := range s.Files {
		matches, err := filepath.Glob(filepath.Join(root, filepath.FromSlash(pattern)))
		if err != nil {
			return nil, fmt.Errorf("glob %q 不合法: %w", pattern, err)
		}
		if len(matches) == 0 {
			return nil, fmt.Errorf("glob %q 没有匹配到任何文件", pattern)
		}
		for _, abs := range matches {
			rel, err := filepath.Rel(root, abs)
			if err != nil {
				return nil, err
			}
			out = append(out, filepath.ToSlash(rel))
		}
	}
	sort.Strings(out)
	return out, nil
}

// expandStep 把步骤里所有字符串过一遍 text/template
// 用反射遍历是为了以后加字段不会漏掉展开
// [EN] expandStep runs every string field through text/template; reflection is used so a
// newly added field cannot be forgotten
func expandStep(s Step, vars map[string]string) (Step, error) {
	out := s
	v := reflect.ValueOf(&out).Elem()
	for i := 0; i < v.NumField(); i++ {
		f := v.Field(i)
		switch {
		case f.Kind() == reflect.String:
			got, err := expandString(f.String(), vars)
			if err != nil {
				return out, fmt.Errorf("字段 %s: %w", v.Type().Field(i).Name, err)
			}
			f.SetString(got)
		case f.Kind() == reflect.Slice && f.Type().Elem().Kind() == reflect.String:
			for j := 0; j < f.Len(); j++ {
				got, err := expandString(f.Index(j).String(), vars)
				if err != nil {
					return out, fmt.Errorf("字段 %s[%d]: %w", v.Type().Field(i).Name, j, err)
				}
				f.Index(j).SetString(got)
			}
		case f.Kind() == reflect.Map && f.Type().Elem().Kind() == reflect.String:
			for _, k := range f.MapKeys() {
				got, err := expandString(f.MapIndex(k).String(), vars)
				if err != nil {
					return out, fmt.Errorf("字段 %s[%q]: %w", v.Type().Field(i).Name, k.String(), err)
				}
				f.SetMapIndex(k, reflect.ValueOf(got))
			}
		}
	}
	return out, nil
}

// expandString 展开单个模板字符串，缺变量直接报错而不是悄悄留空
// [EN] expandString expands one template string and fails loudly on a missing variable
func expandString(s string, vars map[string]string) (string, error) {
	if !strings.Contains(s, "{{") {
		return s, nil
	}
	tpl, err := template.New("step").Option("missingkey=error").Parse(s)
	if err != nil {
		return "", fmt.Errorf("模板 %q 不合法: %w", s, err)
	}
	var b strings.Builder
	if err := tpl.Execute(&b, vars); err != nil {
		return "", fmt.Errorf("模板 %q 展开失败: %w", s, err)
	}
	return b.String(), nil
}
