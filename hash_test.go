// 本文件包含 cmp_hash 的单元测试，覆盖文件和目录比较的各种场景：
//   - 相同文件 / 不同文件 / 文件不存在
//   - 相同目录 / 不同内容 / 不同结构 / 不同文件数 / 空目录 / 大批量文件
//   - 默认模式（diffAll=false）：首处差异即停止、Partial 标志正确
//   - --diff-all 模式（diffAll=true）：全量差异收集
//
// 所有测试均使用 t.TempDir() 创建临时目录，测试结束后自动清理。
package main

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

func TestCompareFiles_Same(t *testing.T) {
	dir := t.TempDir()
	f1 := filepath.Join(dir, "a.txt")
	f2 := filepath.Join(dir, "b.txt")
	os.WriteFile(f1, []byte("hello"), 0644)
	os.WriteFile(f2, []byte("hello"), 0644)

	diff, err := CompareFiles(f1, f2)
	if err != nil {
		t.Fatal(err)
	}
	if !diff.Same {
		t.Error("expected same")
	}
}

func TestCompareFiles_Different(t *testing.T) {
	dir := t.TempDir()
	f1 := filepath.Join(dir, "a.txt")
	f2 := filepath.Join(dir, "b.txt")
	os.WriteFile(f1, []byte("hello"), 0644)
	os.WriteFile(f2, []byte("world"), 0644)

	diff, err := CompareFiles(f1, f2)
	if err != nil {
		t.Fatal(err)
	}
	if diff.Same {
		t.Error("expected different")
	}
}

// TestCompareFiles_DifferentSize 验证大小快速路径：
// 文件大小不同时应直接判定差异，无需哈希。
func TestCompareFiles_DifferentSize(t *testing.T) {
	dir := t.TempDir()
	f1 := filepath.Join(dir, "a.txt")
	f2 := filepath.Join(dir, "b.txt")
	os.WriteFile(f1, []byte("short"), 0644)
	os.WriteFile(f2, []byte("much longer content"), 0644)

	diff, err := CompareFiles(f1, f2)
	if err != nil {
		t.Fatal(err)
	}
	if diff.Same {
		t.Error("expected different due to size mismatch")
	}
}

func TestCompareFiles_NotExist(t *testing.T) {
	dir := t.TempDir()
	bad := filepath.Join(dir, "nonexistent.txt")
	_, err := CompareFiles(bad, bad)
	if err == nil {
		t.Error("expected error for nonexistent file")
	}
}

func TestCompareDirs_Same(t *testing.T) {
	dir := t.TempDir()
	d1 := filepath.Join(dir, "d1")
	d2 := filepath.Join(dir, "d2")
	os.MkdirAll(filepath.Join(d1, "sub"), 0755)
	os.MkdirAll(filepath.Join(d2, "sub"), 0755)
	os.WriteFile(filepath.Join(d1, "a.txt"), []byte("hello"), 0644)
	os.WriteFile(filepath.Join(d1, "sub", "b.txt"), []byte("world"), 0644)
	os.WriteFile(filepath.Join(d2, "a.txt"), []byte("hello"), 0644)
	os.WriteFile(filepath.Join(d2, "sub", "b.txt"), []byte("world"), 0644)

	diff, err := CompareDirs(d1, d2, true)
	if err != nil {
		t.Fatal(err)
	}
	if !diff.Same {
		t.Error("expected same")
	}
}

// TestCompareDirs_DifferentContent 验证 --diff-all 模式下内容不同的文件被完整收集。
func TestCompareDirs_DifferentContent(t *testing.T) {
	dir := t.TempDir()
	d1 := filepath.Join(dir, "d1")
	d2 := filepath.Join(dir, "d2")
	os.MkdirAll(d1, 0755)
	os.MkdirAll(d2, 0755)
	os.WriteFile(filepath.Join(d1, "a.txt"), []byte("hello"), 0644)
	os.WriteFile(filepath.Join(d2, "a.txt"), []byte("different"), 0644)

	diff, err := CompareDirs(d1, d2, true)
	if err != nil {
		t.Fatal(err)
	}
	if diff.Same {
		t.Error("expected different")
	}
	if len(diff.Differ) != 1 || diff.Differ[0] != "a.txt" {
		t.Errorf("expected Differ=[a.txt], got %v", diff.Differ)
	}
}

// TestCompareDirs_DifferentStructure 测试两个目录文件路径集合不一致的情况。
func TestCompareDirs_DifferentStructure(t *testing.T) {
	dir := t.TempDir()
	d1 := filepath.Join(dir, "d1")
	d2 := filepath.Join(dir, "d2")
	os.MkdirAll(filepath.Join(d1, "sub"), 0755)
	os.MkdirAll(d2, 0755)
	os.WriteFile(filepath.Join(d1, "sub", "b.txt"), []byte("x"), 0644)
	os.WriteFile(filepath.Join(d2, "a.txt"), []byte("x"), 0644)

	diff, err := CompareDirs(d1, d2, true)
	if err != nil {
		t.Fatal(err)
	}
	if diff.Same {
		t.Error("expected different structure")
	}
	expectedOnly1 := filepath.Join("sub", "b.txt")
	if len(diff.OnlyIn1) != 1 || diff.OnlyIn1[0] != expectedOnly1 {
		t.Errorf("expected OnlyIn1=[%s], got %v", expectedOnly1, diff.OnlyIn1)
	}
	if len(diff.OnlyIn2) != 1 || diff.OnlyIn2[0] != "a.txt" {
		t.Errorf("expected OnlyIn2=[a.txt], got %v", diff.OnlyIn2)
	}
}

func TestCompareDirs_Empty(t *testing.T) {
	dir := t.TempDir()
	d1 := filepath.Join(dir, "d1")
	d2 := filepath.Join(dir, "d2")
	os.MkdirAll(d1, 0755)
	os.MkdirAll(d2, 0755)

	diff, err := CompareDirs(d1, d2, true)
	if err != nil {
		t.Fatal(err)
	}
	if !diff.Same {
		t.Error("expected same for empty dirs")
	}
}

func TestCompareDirs_DifferentFileCount(t *testing.T) {
	dir := t.TempDir()
	d1 := filepath.Join(dir, "d1")
	d2 := filepath.Join(dir, "d2")
	os.MkdirAll(d1, 0755)
	os.MkdirAll(d2, 0755)
	os.WriteFile(filepath.Join(d1, "a.txt"), []byte("hello"), 0644)

	diff, err := CompareDirs(d1, d2, true)
	if err != nil {
		t.Fatal(err)
	}
	if diff.Same {
		t.Error("expected different file count")
	}
	if len(diff.OnlyIn1) != 1 || diff.OnlyIn1[0] != "a.txt" {
		t.Errorf("expected OnlyIn1=[a.txt], got %v", diff.OnlyIn1)
	}
}

func TestCompareDirs_ManyFiles(t *testing.T) {
	dir := t.TempDir()
	d1 := filepath.Join(dir, "d1")
	d2 := filepath.Join(dir, "d2")
	os.MkdirAll(d1, 0755)
	os.MkdirAll(d2, 0755)

	const n = 100
	for i := 0; i < n; i++ {
		name := filepath.Join(d1, fmt.Sprintf("file_%d.txt", i))
		os.WriteFile(name, []byte("content"), 0644)
		name = filepath.Join(d2, fmt.Sprintf("file_%d.txt", i))
		os.WriteFile(name, []byte("content"), 0644)
	}

	diff, err := CompareDirs(d1, d2, true)
	if err != nil {
		t.Fatal(err)
	}
	if !diff.Same {
		t.Error("expected same with many files")
	}
}

func TestCompareDirs_MixedDiffs(t *testing.T) {
	dir := t.TempDir()
	d1 := filepath.Join(dir, "d1")
	d2 := filepath.Join(dir, "d2")
	os.MkdirAll(d1, 0755)
	os.MkdirAll(d2, 0755)

	os.WriteFile(filepath.Join(d1, "same.txt"), []byte("hello"), 0644)
	os.WriteFile(filepath.Join(d2, "same.txt"), []byte("hello"), 0644)
	os.WriteFile(filepath.Join(d1, "diff.txt"), []byte("hello"), 0644)
	os.WriteFile(filepath.Join(d2, "diff.txt"), []byte("world"), 0644)
	os.WriteFile(filepath.Join(d1, "extra1.txt"), []byte("x"), 0644)
	os.WriteFile(filepath.Join(d2, "extra2.txt"), []byte("y"), 0644)

	diff, err := CompareDirs(d1, d2, true)
	if err != nil {
		t.Fatal(err)
	}
	if diff.Same {
		t.Fatal("expected different")
	}
	if len(diff.OnlyIn1) != 1 || diff.OnlyIn1[0] != "extra1.txt" {
		t.Errorf("expected OnlyIn1=[extra1.txt], got %v", diff.OnlyIn1)
	}
	if len(diff.OnlyIn2) != 1 || diff.OnlyIn2[0] != "extra2.txt" {
		t.Errorf("expected OnlyIn2=[extra2.txt], got %v", diff.OnlyIn2)
	}
	if len(diff.Differ) != 1 || diff.Differ[0] != "diff.txt" {
		t.Errorf("expected Differ=[diff.txt], got %v", diff.Differ)
	}
}

// TestCompareDirs_DefaultMode_FirstDiff 验证默认模式（diffAll=false）
// 在发现第一处内容差异后即停止，Partial 标志为 true。
func TestCompareDirs_DefaultMode_FirstDiff(t *testing.T) {
	dir := t.TempDir()
	d1 := filepath.Join(dir, "d1")
	d2 := filepath.Join(dir, "d2")
	os.MkdirAll(d1, 0755)
	os.MkdirAll(d2, 0755)

	// 创建多个内容不同的文件。
	os.WriteFile(filepath.Join(d1, "a.txt"), []byte("hello"), 0644)
	os.WriteFile(filepath.Join(d2, "a.txt"), []byte("different_a"), 0644)
	os.WriteFile(filepath.Join(d1, "b.txt"), []byte("world"), 0644)
	os.WriteFile(filepath.Join(d2, "b.txt"), []byte("different_b"), 0644)

	diff, err := CompareDirs(d1, d2, false)
	if err != nil {
		t.Fatal(err)
	}
	if diff.Same {
		t.Error("expected different")
	}
	if !diff.Partial {
		t.Error("expected Partial=true in default mode on content diff")
	}
	// 默认模式可能在发现第一处差异后即停止，
	// Differ 至少包含 1 个但可能不包含全部 2 个。
	if len(diff.Differ) < 1 {
		t.Error("expected at least 1 Differ entry")
	}
}

// TestCompareDirs_DiffAll_MultipleDiffs 验证 --diff-all 模式
// 完整收集所有内容差异。
func TestCompareDirs_DiffAll_MultipleDiffs(t *testing.T) {
	dir := t.TempDir()
	d1 := filepath.Join(dir, "d1")
	d2 := filepath.Join(dir, "d2")
	os.MkdirAll(d1, 0755)
	os.MkdirAll(d2, 0755)

	os.WriteFile(filepath.Join(d1, "a.txt"), []byte("hello"), 0644)
	os.WriteFile(filepath.Join(d2, "a.txt"), []byte("different_a"), 0644)
	os.WriteFile(filepath.Join(d1, "b.txt"), []byte("world"), 0644)
	os.WriteFile(filepath.Join(d2, "b.txt"), []byte("different_b"), 0644)

	diff, err := CompareDirs(d1, d2, true)
	if err != nil {
		t.Fatal(err)
	}
	if diff.Same {
		t.Fatal("expected different")
	}
	if diff.Partial {
		t.Error("expected Partial=false in --diff-all mode")
	}
	if len(diff.Differ) != 2 {
		t.Errorf("expected 2 Differ entries, got %d: %v", len(diff.Differ), diff.Differ)
	}
}

// TestCompareDirs_DefaultMode_Same 验证默认模式下无差异时 Partial 为 false。
func TestCompareDirs_DefaultMode_Same(t *testing.T) {
	dir := t.TempDir()
	d1 := filepath.Join(dir, "d1")
	d2 := filepath.Join(dir, "d2")
	os.MkdirAll(d1, 0755)
	os.MkdirAll(d2, 0755)
	os.WriteFile(filepath.Join(d1, "a.txt"), []byte("hello"), 0644)
	os.WriteFile(filepath.Join(d2, "a.txt"), []byte("hello"), 0644)

	diff, err := CompareDirs(d1, d2, false)
	if err != nil {
		t.Fatal(err)
	}
	if !diff.Same {
		t.Error("expected same")
	}
	if diff.Partial {
		t.Error("expected Partial=false when directories are same")
	}
}

// TestCompareDirs_DifferentSize_NoHash 验证默认模式下文件大小不同时
// 跳过哈希直接报告差异。
func TestCompareDirs_DifferentSize_NoHash(t *testing.T) {
	dir := t.TempDir()
	d1 := filepath.Join(dir, "d1")
	d2 := filepath.Join(dir, "d2")
	os.MkdirAll(d1, 0755)
	os.MkdirAll(d2, 0755)
	os.WriteFile(filepath.Join(d1, "x.txt"), []byte("short"), 0644)
	os.WriteFile(filepath.Join(d2, "x.txt"), []byte("much longer content here"), 0644)

	diff, err := CompareDirs(d1, d2, false)
	if err != nil {
		t.Fatal(err)
	}
	if diff.Same {
		t.Error("expected different due to size mismatch")
	}
	if len(diff.Differ) != 1 || diff.Differ[0] != "x.txt" {
		t.Errorf("expected Differ=[x.txt], got %v", diff.Differ)
	}
	if !diff.Partial {
		t.Error("expected Partial=true")
	}
}
