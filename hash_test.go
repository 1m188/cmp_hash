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

func TestCompareDirs_DifferentFileCount(t *testing.T) {
	dir := t.TempDir()
	d1 := filepath.Join(dir, "d1")
	d2 := filepath.Join(dir, "d2")
	os.MkdirAll(d1, 0755)
	os.MkdirAll(d2, 0755)
	os.WriteFile(filepath.Join(d1, "a.txt"), []byte("hello"), 0644)
	// d2 has no files

	same, err := CompareDirs(d1, d2)
	if err != nil {
		t.Fatal(err)
	}
	if same {
		t.Error("expected different file count")
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

	same, err := CompareDirs(d1, d2)
	if err != nil {
		t.Fatal(err)
	}
	if !same {
		t.Error("expected same with many files")
	}
}
