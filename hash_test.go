// 本文件包含 cmp_hash 的单元测试，覆盖文件和目录比较的各种场景：
//   - 相同文件 / 不同文件 / 文件不存在
//   - 相同目录 / 不同内容 / 不同结构 / 不同文件数 / 空目录 / 大批量文件
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

	same, err := CompareFiles(f1, f2)
	if err != nil {
		t.Fatal(err)
	}
	if !same {
		t.Error("expected same")
	}
}

func TestCompareFiles_Different(t *testing.T) {
	dir := t.TempDir()
	f1 := filepath.Join(dir, "a.txt")
	f2 := filepath.Join(dir, "b.txt")
	os.WriteFile(f1, []byte("hello"), 0644)
	os.WriteFile(f2, []byte("world"), 0644)

	same, err := CompareFiles(f1, f2)
	if err != nil {
		t.Fatal(err)
	}
	if same {
		t.Error("expected different")
	}
}

func TestCompareFiles_NotExist(t *testing.T) {
	_, err := CompareFiles("/nonexistent/path", "/another/bad/path")
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

	same, err := CompareDirs(d1, d2)
	if err != nil {
		t.Fatal(err)
	}
	if !same {
		t.Error("expected same")
	}
}

// TestCompareDirs_DifferentContent 测试目录结构相同但某个文件内容不同的情况。
func TestCompareDirs_DifferentContent(t *testing.T) {
	dir := t.TempDir()
	d1 := filepath.Join(dir, "d1")
	d2 := filepath.Join(dir, "d2")
	os.MkdirAll(d1, 0755)
	os.MkdirAll(d2, 0755)
	os.WriteFile(filepath.Join(d1, "a.txt"), []byte("hello"), 0644)
	os.WriteFile(filepath.Join(d2, "a.txt"), []byte("different"), 0644)

	same, err := CompareDirs(d1, d2)
	if err != nil {
		t.Fatal(err)
	}
	if same {
		t.Error("expected different")
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

	same, err := CompareDirs(d1, d2)
	if err != nil {
		t.Fatal(err)
	}
	if same {
		t.Error("expected different structure")
	}
}

// TestCompareDirs_Empty 测试两个空目录应视为相同。
func TestCompareDirs_Empty(t *testing.T) {
	dir := t.TempDir()
	d1 := filepath.Join(dir, "d1")
	d2 := filepath.Join(dir, "d2")
	os.MkdirAll(d1, 0755)
	os.MkdirAll(d2, 0755)

	same, err := CompareDirs(d1, d2)
	if err != nil {
		t.Fatal(err)
	}
	if !same {
		t.Error("expected same for empty dirs")
	}
}

// TestCompareDirs_DifferentFileCount 测试文件数量不同的两个目录。
func TestCompareDirs_DifferentFileCount(t *testing.T) {
	dir := t.TempDir()
	d1 := filepath.Join(dir, "d1")
	d2 := filepath.Join(dir, "d2")
	os.MkdirAll(d1, 0755)
	os.MkdirAll(d2, 0755)
	os.WriteFile(filepath.Join(d1, "a.txt"), []byte("hello"), 0644)
	// d2 没有文件，文件数量与 d1 不同。

	same, err := CompareDirs(d1, d2)
	if err != nil {
		t.Fatal(err)
	}
	if same {
		t.Error("expected different file count")
	}
}

// TestCompareDirs_ManyFiles 测试含 100 个文件的目录并发比较，
// 用于验证 worker pool 在较多文件时的正确性。
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

	same, err := CompareDirs(d1, d2)
	if err != nil {
		t.Fatal(err)
	}
	if !same {
		t.Error("expected same with many files")
	}
}
