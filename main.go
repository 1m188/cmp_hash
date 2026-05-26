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
