// cmp_hash 是一个比较两个文件或目录内容是否相同的命令行工具。
//
// 用法：
//
//	cmp_hash [--diff-all] <path1> <path2>
//
// 默认模式（无 --diff-all）：进行最快比较——先比文件大小再比哈希值，
// 发现第一处文件内容差异后立即停止，输出差异信息并提示可能存在更多差异。
//
// --diff-all 模式：全量比较所有文件，输出完整差异列表。
//
// 两个路径必须同为文件或同为目录，否则直接报错退出。
//
// 本文件为 CLI 入口，负责解析参数和标志、判断路径类型并分发到对应的比较函数。
package main

import (
	"flag"
	"fmt"
	"os"
)

func main() {
	diffAll := flag.Bool("diff-all", false, "compare all files and report every difference")
	flag.Parse()

	if flag.NArg() != 2 {
		fmt.Fprintf(os.Stderr, "Usage: cmp_hash [--diff-all] <path1> <path2>\n")
		os.Exit(1)
	}

	path1, path2 := flag.Arg(0), flag.Arg(1)

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
		diff, err = CompareDirs(path1, path2, *diffAll)
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

	if info1.IsDir() {
		for _, rel := range diff.Differ {
			fmt.Printf("  Content differs: %s\n", rel)
		}
	} else {
		fmt.Println("  Content differs")
	}

	// 默认模式下内容比较可能被截断，提示用户可能存在更多差异。
	if diff.Partial {
		fmt.Println("\n注意：仅发现第一处不同，可能存在更多差异。使用 --diff-all 查看完整差异。")
	}
}
