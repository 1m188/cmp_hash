// cmp_hash 是一个比较两个文件或目录内容是否相同的命令行工具。
//
// 用法：
//
//	cmp_hash <path1> <path2>
//
// 输出 "Same"（内容相同）或 "Different"（内容不同）。
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

	// 检查两个路径是否存在，并获取文件信息。
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

	// 两个路径必须是同一类型（都是文件或都是目录）。
	if info1.IsDir() != info2.IsDir() {
		fmt.Println("Different: one is a file, the other is a directory")
		os.Exit(1)
	}

	// 根据路径类型分发到文件比较或目录比较。
	var same bool
	if info1.IsDir() {
		same, err = CompareDirs(path1, path2)
	} else {
		same, err = CompareFiles(path1, path2)
	}
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	if same {
		fmt.Println("Same")
	} else {
		fmt.Println("Different")
	}
}
