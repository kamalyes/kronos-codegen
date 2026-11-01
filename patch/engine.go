/*
 * @Author: kamalyes 501893067@qq.com
 * @Date: 2026-11-01 16:20:11
 * @LastEditors: kamalyes 501893067@qq.com
 * @LastEditTime: 2026-11-01 20:10:35
 * @FilePath: \kronos-codegen\patch\engine.go
 * @Description: 补丁引擎 - 在内存里累积所有改动，确认后再原子写盘
 *
 * Copyright (c) 2026 by kamalyes, All Rights Reserved.
 */
package patch

import (
	"bytes"
	"fmt"
	"go/format"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// FileStore 抽象补丁引擎的文件读写，测试里换成内存实现就不用碰磁盘
// [EN] FileStore abstracts file IO so tests can run entirely in memory
type FileStore interface {
	// Read 读取文件内容
	// [EN] Read returns the file content
	Read(path string) ([]byte, error)
	// Write 写入文件内容
	// [EN] Write stores the file content
	Write(path string, data []byte) error
	// Exists 判断文件是否存在
	// [EN] Exists reports whether the file exists
	Exists(path string) bool
}

// DirStore 以某个目录为根的文件存储，路径一律按斜杠书写
// [EN] DirStore is a directory rooted file store; paths are always slash separated
type DirStore struct {
	// Root 项目根目录
	// [EN] Root is the project root directory
	Root string
}

// NewDirStore 构造目录存储
// [EN] NewDirStore builds a directory backed store
func NewDirStore(root string) *DirStore { return &DirStore{Root: root} }

// resolve 把相对路径解析成绝对路径，顺便挡住越过根目录的路径
// [EN] resolve turns a relative path into an absolute one and rejects root escapes
func (s *DirStore) resolve(p string) (string, error) {
	clean := filepath.Clean(filepath.FromSlash(p))
	if filepath.IsAbs(clean) || clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("%w: 路径 %q 越出项目根目录", ErrInvalidPatch, p)
	}
	return filepath.Join(s.Root, clean), nil
}

// Read 实现 FileStore
// [EN] Read implements FileStore
func (s *DirStore) Read(p string) ([]byte, error) {
	abs, err := s.resolve(p)
	if err != nil {
		return nil, err
	}
	return os.ReadFile(abs)
}

// Write 实现 FileStore，先写临时文件再 rename，避免写一半被打断留下坏文件
// [EN] Write implements FileStore via temp file plus rename, so a crash cannot truncate
func (s *DirStore) Write(p string, data []byte) error {
	abs, err := s.resolve(p)
	if err != nil {
		return err
	}
	mode := os.FileMode(0o644)
	if st, err := os.Stat(abs); err == nil {
		mode = st.Mode().Perm()
	}
	if err := os.MkdirAll(filepath.Dir(abs), 0o755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(abs), ".kronos-codegen-*")
	if err != nil {
		return err
	}
	name := tmp.Name()
	defer os.Remove(name)
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Chmod(name, mode); err != nil {
		return err
	}
	return os.Rename(name, abs)
}

// Exists 实现 FileStore
// [EN] Exists implements FileStore
func (s *DirStore) Exists(p string) bool {
	abs, err := s.resolve(p)
	if err != nil {
		return false
	}
	st, err := os.Stat(abs)
	return err == nil && !st.IsDir()
}

// MemStore 内存文件存储，仅用于测试
// [EN] MemStore is an in-memory file store used by tests
type MemStore struct {
	// Files 路径到内容的映射
	// [EN] Files maps paths to contents
	Files map[string][]byte
}

// NewMemStore 用字符串字面量构造内存存储
// [EN] NewMemStore builds an in-memory store from string literals
func NewMemStore(files map[string]string) *MemStore {
	m := &MemStore{Files: make(map[string][]byte, len(files))}
	for k, v := range files {
		m.Files[k] = []byte(v)
	}
	return m
}

// Read 实现 FileStore
// [EN] Read implements FileStore
func (m *MemStore) Read(p string) ([]byte, error) {
	b, ok := m.Files[p]
	if !ok {
		return nil, os.ErrNotExist
	}
	out := make([]byte, len(b))
	copy(out, b)
	return out, nil
}

// Write 实现 FileStore
// [EN] Write implements FileStore
func (m *MemStore) Write(p string, data []byte) error {
	if m.Files == nil {
		m.Files = map[string][]byte{}
	}
	m.Files[p] = append([]byte(nil), data...)
	return nil
}

// Exists 实现 FileStore
// [EN] Exists implements FileStore
func (m *MemStore) Exists(p string) bool {
	_, ok := m.Files[p]
	return ok
}

// FileChange 一个文件在本次运行里的全部改动
// [EN] FileChange is the accumulated change of one file in this run
type FileChange struct {
	// Path 文件相对路径
	// [EN] Path is the relative file path
	Path string
	// Before 原始内容
	// [EN] Before is the original content
	Before []byte
	// After 应用全部补丁后的内容
	// [EN] After is the content after every patch
	After []byte
	// Plans 作用在该文件上的所有计划，含已跳过的
	// [EN] Plans holds every plan targeting this file, skipped ones included
	Plans []*Plan
	// Created 该文件是本次运行新建的，Before 为空
	// [EN] Created marks a file this run creates from scratch, so Before is empty
	Created bool
	// gofmtClean 原始内容本身是否已经是 gofmt 干净的
	// [EN] gofmtClean records whether the original content was already gofmt-clean
	gofmtClean bool
}

// Changed 判断文件内容是否真的发生了变化
// [EN] Changed reports whether the content actually changed
func (c *FileChange) Changed() bool { return !bytes.Equal(c.Before, c.After) }

// Engine 幂等补丁引擎
// 所有改动先在内存里累积，Diff 确认之后再由 Write 一次性落盘
// [EN] Engine accumulates every change in memory; Write flushes them after confirmation
type Engine struct {
	store  FileStore
	policy FormatPolicy
	order  []string
	files  map[string]*FileChange
}

// NewEngine 构造补丁引擎
// [EN] NewEngine builds a patch engine
func NewEngine(store FileStore) *Engine {
	return &Engine{store: store, policy: FormatIfClean, files: map[string]*FileChange{}}
}

// SetFormatPolicy 设置格式化策略
// [EN] SetFormatPolicy sets the normalization policy
func (e *Engine) SetFormatPolicy(p FormatPolicy) { e.policy = p }

// load 读入文件并缓存，同一文件在一次运行里只读一次
// [EN] load reads a file once per run and caches it
func (e *Engine) load(path string) (*FileChange, error) {
	if fc, ok := e.files[path]; ok {
		return fc, nil
	}
	src, err := e.store.Read(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, locateErr(path, "目标文件", "文件不存在，补丁只负责往已存在的文件里插入条目", ErrNotFound)
		}
		return nil, locateErr(path, "目标文件", err.Error(), ErrNotFound)
	}
	fc := &FileChange{Path: path, Before: src, After: src}
	if isGoFile(path) {
		if formatted, err := format.Source(src); err == nil && bytes.Equal(formatted, src) {
			fc.gofmtClean = true
		}
	}
	e.files[path] = fc
	e.order = append(e.order, path)
	return fc, nil
}

// Apply 应用一条补丁，条目已存在时返回的计划 Present 为 true 且不产生任何字节变化
// [EN] Apply runs one patch; an already-present entry yields a plan with Present set
func (e *Engine) Apply(p Patch) (*Plan, error) {
	fc, err := e.load(p.File())
	if err != nil {
		return nil, err
	}
	pl, err := p.Plan(fc.After)
	if err != nil {
		return nil, err
	}
	fc.Plans = append(fc.Plans, pl)
	if pl.NoOp() {
		return pl, nil
	}
	next := applyInsertions(fc.After, pl.Insertions)
	if isGoFile(fc.Path) {
		next, err = e.normalize(fc, next)
		if err != nil {
			return nil, err
		}
	}
	fc.After = next
	return pl, nil
}

// Step 执行一步，按类型分发到新建文件或插入条目
// [EN] Step executes one action, dispatching on whether it creates or patches
func (e *Engine) Step(a Action) (*Plan, error) {
	switch v := a.(type) {
	case Creator:
		return e.create(v)
	case Patch:
		return e.Apply(v)
	default:
		return nil, fmt.Errorf("%w: 未知的步骤类型 %T", ErrInvalidPatch, a)
	}
}

// Run 顺序执行一批步骤，新建与插入进同一个事务
// 任何一步失败就整体中止，因为改动都只在内存里，磁盘上一个字节都没动
// [EN] Run executes a batch of actions in one transaction; any failure aborts the whole
// batch, and since every change lives in memory the working tree stays untouched
func (e *Engine) Run(actions []Action) ([]*Plan, error) {
	out := make([]*Plan, 0, len(actions))
	for _, a := range actions {
		pl, err := e.Step(a)
		if err != nil {
			return out, err
		}
		out = append(out, pl)
	}
	return out, nil
}

// create 新建一个文件，已存在就跳过
// 幂等语义与插入层一致：目标已存在即 Present，**绝不覆盖**
// [EN] create adds one new file and skips an existing target; the idempotency semantics
// match the insertion layer, so an existing file is reported Present and never overwritten
func (e *Engine) create(c Creator) (*Plan, error) {
	path := c.File()
	out := &Plan{File: path, Label: c.Label(), Created: true, Line: 1}

	// 同一次运行里已经处理过这个路径
	// [EN] this run already touched the path
	if fc, ok := e.files[path]; ok {
		out.Present = true
		fc.Plans = append(fc.Plans, out)
		return out, nil
	}
	// 已存在就跳过，但仍要把文件读进来：计划挂到它的 FileChange 上才会出现在摘要里，
	// 而且后面若有补丁要往这个文件里插条目，走的也是同一份缓存内容
	// [EN] an existing target is skipped, yet still loaded: attaching the plan to its
	// FileChange is what makes it show up in the summary, and a later patch targeting the
	// same file then works off the very same cached content
	if e.store.Exists(path) {
		fc, err := e.load(path)
		if err != nil {
			return nil, err
		}
		out.Present = true
		fc.Plans = append(fc.Plans, out)
		return out, nil
	}
	content, err := c.Render()
	if err != nil {
		return nil, err
	}
	fc := &FileChange{Path: path, After: content, Created: true, gofmtClean: true}
	fc.Plans = append(fc.Plans, out)
	e.files[path] = fc
	e.order = append(e.order, path)

	out.Lines = countLines(content)
	out.Snippet = previewLines(content, previewLineCount)
	return out, nil
}

// ApplyAll 顺序应用一批补丁，任何一条失败就整体中止，不会留下半套改动
// 因为改动都在内存里，中止时磁盘上的文件还是原样
// [EN] ApplyAll runs patches in order and aborts on the first failure; since every change
// lives in memory, an abort leaves the working tree untouched
func (e *Engine) ApplyAll(patches []Patch) ([]*Plan, error) {
	out := make([]*Plan, 0, len(patches))
	for _, p := range patches {
		pl, err := e.Apply(p)
		if err != nil {
			return out, err
		}
		out = append(out, pl)
	}
	return out, nil
}

// normalize 按策略决定是否对整文件做 gofmt 归一化
// 插入一个更长的名字会打破块内列对齐，归一化负责补回来
// 原文件本身不干净时跳过，避免把别人手写的排版整体重排
// [EN] normalize applies the gofmt pass according to the policy
func (e *Engine) normalize(fc *FileChange, src []byte) ([]byte, error) {
	switch e.policy {
	case FormatNever:
		return src, nil
	case FormatIfClean:
		if !fc.gofmtClean {
			return src, nil
		}
	}
	out, err := format.Source(src)
	if err != nil {
		return nil, locateErr(fc.Path, "gofmt 归一化", err.Error(), ErrFormat)
	}
	return out, nil
}

// Changes 返回所有被触达的文件，按首次触达顺序排列
// [EN] Changes returns every touched file in first-touch order
func (e *Engine) Changes() []*FileChange {
	out := make([]*FileChange, 0, len(e.order))
	for _, p := range e.order {
		out = append(out, e.files[p])
	}
	return out
}

// Plans 返回所有计划，按文件顺序铺平
// [EN] Plans returns every plan, flattened in file order
func (e *Engine) Plans() []*Plan {
	var out []*Plan
	for _, c := range e.Changes() {
		out = append(out, c.Plans...)
	}
	return out
}

// Diff 渲染所有改动的 unified diff，无改动时返回空串
// [EN] Diff renders the unified diff of every change, empty when nothing changed
func (e *Engine) Diff() string {
	var b strings.Builder
	for _, c := range e.Changes() {
		if !c.Changed() {
			continue
		}
		if c.Created {
			// 新建文件不铺全文，只给路径、行数与前若干行
			// [EN] a new file gets path, line count and the first lines, not the whole body
			b.WriteString(createPreview(c.Path, c.After, previewLineCount))
			continue
		}
		b.WriteString(UnifiedDiff(c.Path, c.Before, c.After, 3))
	}
	return b.String()
}

// Summary 渲染变更摘要，每行给出文件、行号与条目标签
// 已存在的条目用 skip 标出，让用户看清哪些是幂等命中
// [EN] Summary renders a per-entry overview with file, line number and label
func (e *Engine) Summary() string {
	var b strings.Builder
	added, created, skipped := 0, 0, 0
	for _, c := range e.Changes() {
		for _, pl := range c.Plans {
			switch {
			case pl.Present && pl.Created:
				skipped++
				fmt.Fprintf(&b, "  skip   %s  %s  (文件已存在)\n", pl.File, pl.Label)
				continue
			case pl.Present:
				skipped++
				fmt.Fprintf(&b, "  skip   %s:%d  %s  (已存在)\n", pl.File, pl.Line, pl.Label)
				continue
			case pl.Created:
				created++
				fmt.Fprintf(&b, "  create %s  (%d 行)\n", pl.File, pl.Lines)
			default:
				added++
				fmt.Fprintf(&b, "  add    %s:%d  %s\n", pl.File, pl.Line, pl.Label)
			}
			for _, ln := range strings.Split(pl.Snippet, "\n") {
				fmt.Fprintf(&b, "           | %s\n", ln)
			}
		}
	}
	fmt.Fprintf(&b, "  共 %d 个新建文件，%d 处新增，%d 处已存在跳过，涉及 %d 个文件\n",
		created, added, skipped, e.changedCount())
	return b.String()
}

// changedCount 统计真正发生变化的文件数
// [EN] changedCount counts the files that actually changed
func (e *Engine) changedCount() int {
	n := 0
	for _, c := range e.Changes() {
		if c.Changed() {
			n++
		}
	}
	return n
}

// Write 把所有发生变化的文件写回存储，返回写入的文件列表
// [EN] Write flushes every changed file and returns the written paths
func (e *Engine) Write() ([]string, error) {
	var written []string
	for _, c := range e.Changes() {
		if !c.Changed() {
			continue
		}
		if err := e.store.Write(c.Path, c.After); err != nil {
			return written, fmt.Errorf("写入 %s 失败: %w", c.Path, err)
		}
		written = append(written, c.Path)
	}
	sort.Strings(written)
	return written, nil
}

// isGoFile 判断是否是 Go 源文件，只有 Go 文件才走 gofmt 归一化
// [EN] isGoFile reports whether the path is a Go source file
func isGoFile(path string) bool { return strings.HasSuffix(path, ".go") }

// ApplyToSource 把一条补丁应用到内存中的源码，供测试与单文件预览使用
// [EN] ApplyToSource applies one patch to an in-memory buffer, for tests and previews
func ApplyToSource(src []byte, p Patch, policy FormatPolicy) ([]byte, *Plan, error) {
	store := NewMemStore(map[string]string{p.File(): string(src)})
	eng := NewEngine(store)
	eng.SetFormatPolicy(policy)
	pl, err := eng.Apply(p)
	if err != nil {
		return nil, nil, err
	}
	return eng.files[p.File()].After, pl, nil
}
