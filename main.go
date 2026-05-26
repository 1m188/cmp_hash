// cmp_hash 是一个比较两个文件或目录内容是否相同的命令行工具。
//
// 用法：
//
//	cmp_hash <path1> <path2>
//
// 对于文件比较，输出 "Same" 或 "Different"。
// 对于目录比较，输出 "Same"，或者列出所有差异项：仅在某一侧存在的文件、
// 以及两边路径相同但内容不同的文件。
//
// 两个路径必须同为文件或同为目录，否则直接报错退出。
//
// 本文件为 CLI 入口，负责解析参数、判断路径类型并分发到对应的比较函数。
package main

import (
	"fmt"
	"os"
)

func main() {
	if len(os.Args) != 3 {
		fmt.Fprintf(os.Stderr, "Usage: cmp_hash <path1> <path2>\n")
		os.Exit(1)
	}

	path1, path2 := os.Args[1], os.Args[2]

	info1, err := os.Stat(path1)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
	info2, err := os.Stat(path2)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	if info1.IsDir() != info2.IsDir() {
		fmt.Println("Different: one is a file, the other is a directory")
		os.Exit(1)
	}

	var diff *Diff
	if info1.IsDir() {
		diff, err = CompareDirs(path1, path2)
	} else {
		diff, err = CompareFiles(path1, path2)
	}
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	if diff.Same {
		fmt.Println("Same")
		return
	}

	// 输出详细差异。
	fmt.Println("Different:")

	for _, rel := range diff.OnlyIn1 {
		fmt.Printf("  Only in %s: %s\n", path1, rel)
	}
	for _, rel := range diff.OnlyIn2 {
		fmt.Printf("  Only in %s: %s\n", path2, rel)
	}
	// 文件比较时 Differ 中存放的是两个绝对路径，按其他格式输出。
	if info1.IsDir() {
		for _, rel := range diff.Differ {
			fmt.Printf("  Content differs: %s\n", rel)
		}
	} else if !diff.Same {
		fmt.Println("  Content differs")
	}
}
