// 本文件包含 cmp_hash 的单元测试，覆盖文件和目录比较的各种场景：
//   - 相同文件 / 不同文件 / 文件不存在
//   - 相同目录 / 不同内容 / 不同结构 / 不同文件数 / 空目录 / 大批量文件
//   - 差异详情验证：仅在一侧存在的文件列表、内容不同的文件列表
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

	diff, err := CompareDirs(d1, d2)
	if err != nil {
		t.Fatal(err)
	}
	if !diff.Same {
		t.Error("expected same")
	}
}

// TestCompareDirs_DifferentContent 测试目录结构相同但某个文件内容不同的情况，
// 同时验证 Differ 列表中包含正确的文件路径。
func TestCompareDirs_DifferentContent(t *testing.T) {
	dir := t.TempDir()
	d1 := filepath.Join(dir, "d1")
	d2 := filepath.Join(dir, "d2")
	os.MkdirAll(d1, 0755)
	os.MkdirAll(d2, 0755)
	os.WriteFile(filepath.Join(d1, "a.txt"), []byte("hello"), 0644)
	os.WriteFile(filepath.Join(d2, "a.txt"), []byte("different"), 0644)

	diff, err := CompareDirs(d1, d2)
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

// TestCompareDirs_DifferentStructure 测试两个目录文件路径集合不一致的情况，
// 同时验证 OnlyIn1 / OnlyIn2 列表正确。
func TestCompareDirs_DifferentStructure(t *testing.T) {
	dir := t.TempDir()
	d1 := filepath.Join(dir, "d1")
	d2 := filepath.Join(dir, "d2")
	os.MkdirAll(filepath.Join(d1, "sub"), 0755)
	os.MkdirAll(d2, 0755)
	os.WriteFile(filepath.Join(d1, "sub", "b.txt"), []byte("x"), 0644)
	os.WriteFile(filepath.Join(d2, "a.txt"), []byte("x"), 0644)

	diff, err := CompareDirs(d1, d2)
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

// TestCompareDirs_Empty 测试两个空目录应视为相同。
func TestCompareDirs_Empty(t *testing.T) {
	dir := t.TempDir()
	d1 := filepath.Join(dir, "d1")
	d2 := filepath.Join(dir, "d2")
	os.MkdirAll(d1, 0755)
	os.MkdirAll(d2, 0755)

	diff, err := CompareDirs(d1, d2)
	if err != nil {
		t.Fatal(err)
	}
	if !diff.Same {
		t.Error("expected same for empty dirs")
	}
}

// TestCompareDirs_DifferentFileCount 测试文件数量不同的两个目录，
// 验证缺失文件被正确归入 OnlyIn1。
func TestCompareDirs_DifferentFileCount(t *testing.T) {
	dir := t.TempDir()
	d1 := filepath.Join(dir, "d1")
	d2 := filepath.Join(dir, "d2")
	os.MkdirAll(d1, 0755)
	os.MkdirAll(d2, 0755)
	os.WriteFile(filepath.Join(d1, "a.txt"), []byte("hello"), 0644)
	// d2 没有文件。

	diff, err := CompareDirs(d1, d2)
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

// TestCompareDirs_ManyFiles 测试含 100 个文件的目录并发比较，
// 验证 worker pool 在较多文件时的正确性。
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

	diff, err := CompareDirs(d1, d2)
	if err != nil {
		t.Fatal(err)
	}
	if !diff.Same {
		t.Error("expected same with many files")
	}
}

// TestCompareDirs_MixedDiffs 测试混合差异场景：既有仅在一侧存在的文件，
// 又有内容不同的文件，验证所有差异类型同时被正确记录。
func TestCompareDirs_MixedDiffs(t *testing.T) {
	dir := t.TempDir()
	d1 := filepath.Join(dir, "d1")
	d2 := filepath.Join(dir, "d2")
	os.MkdirAll(d1, 0755)
	os.MkdirAll(d2, 0755)

	// 内容相同的文件。
	os.WriteFile(filepath.Join(d1, "same.txt"), []byte("hello"), 0644)
	os.WriteFile(filepath.Join(d2, "same.txt"), []byte("hello"), 0644)

	// 内容不同的文件。
	os.WriteFile(filepath.Join(d1, "diff.txt"), []byte("hello"), 0644)
	os.WriteFile(filepath.Join(d2, "diff.txt"), []byte("world"), 0644)

	// 仅在 d1 中的文件。
	os.WriteFile(filepath.Join(d1, "extra1.txt"), []byte("x"), 0644)

	// 仅在 d2 中的文件。
	os.WriteFile(filepath.Join(d2, "extra2.txt"), []byte("y"), 0644)

	diff, err := CompareDirs(d1, d2)
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
