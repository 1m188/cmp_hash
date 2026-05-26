// 本文件包含 cmp_hash 的核心逻辑：计算文件 SHA-256 哈希、遍历目录、
// 以及文件/目录的内容比较。
//
// 文件比较：分别计算两个文件的 SHA-256 哈希值，比较是否相同。
// 目录比较：递归遍历两个目录，收集所有普通文件的相对路径，先比较文件数量
// 和路径集合是否一致，再使用 worker pool 并发计算每对文件的哈希值进行比对。
//
// 目录比较通过 worker pool 实现并发哈希计算，worker 数量等于逻辑 CPU 核数。
// 符号链接（软链接）不会被纳入比较——只比较普通文件的内容。
package main

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"sync"
)

// errDiff 是文件内容不一致时返回的哨兵错误，
// CompareDirs 用它区分"发现差异"和"发生 I/O 错误"两种情况。
var errDiff = errors.New("content differs")

// fileHash 计算指定文件的 SHA-256 哈希值，返回十六进制编码的字符串。
// 文件以流式方式读取，不会将整个文件加载到内存中。
func fileHash(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", fmt.Errorf("open %s: %w", path, err)
	}
	defer f.Close()

	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", fmt.Errorf("read %s: %w", path, err)
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

// walkFiles 递归遍历 root 目录，返回一个 map，键为文件相对于 root 的路径，
// 值为文件的绝对路径。只收集普通文件，目录和符号链接会被忽略。
func walkFiles(root string) (map[string]string, error) {
	files := make(map[string]string)
	err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		// 跳过非普通文件（目录、符号链接等），
		// 只有普通文件的内容才能被哈希比较。
		if !d.Type().IsRegular() {
			return nil
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		files[rel] = path
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("walk %s: %w", root, err)
	}
	return files, nil
}

// CompareFiles 比较两个文件的内容是否相同。
// 分别计算两个文件的 SHA-256 哈希值，若相同则返回 true。
func CompareFiles(path1, path2 string) (bool, error) {
	h1, err := fileHash(path1)
	if err != nil {
		return false, err
	}
	h2, err := fileHash(path2)
	if err != nil {
		return false, err
	}
	return h1 == h2, nil
}

// CompareDirs 比较两个目录的内容是否相同，包括文件树结构和所有文件的内容。
// 首先确保两个目录包含相同数量和相同相对路径的普通文件集合，
// 然后使用 worker pool 并发计算每对文件的哈希值进行逐对比较。
// 只要发现有内容不同的文件对即返回不同，不继续处理后续文件对。
func CompareDirs(dir1, dir2 string) (bool, error) {
	files1, err := walkFiles(dir1)
	if err != nil {
		return false, err
	}
	files2, err := walkFiles(dir2)
	if err != nil {
		return false, err
	}

	// 先比较文件数量，不等则内容必定不同。
	if len(files1) != len(files2) {
		return false, nil
	}

	// 构建文件对列表，同时检查相对路径集合是否一致。
	type pair struct{ a, b string }
	pairs := make([]pair, 0, len(files1))
	for rel, abs1 := range files1 {
		abs2, ok := files2[rel]
		if !ok {
			return false, nil
		}
		pairs = append(pairs, pair{abs1, abs2})
	}

	// 两个目录都是空的，直接返回相同。
	if len(pairs) == 0 {
		return true, nil
	}

	// 确定 worker 数量，不超过文件对总数。
	n := runtime.NumCPU()
	if n > len(pairs) {
		n = len(pairs)
	}

	// work 通道缓冲区大小等于文件对总数，避免发送端阻塞，
	// 让所有任务一次性放入通道后即可关闭。
	work := make(chan pair, len(pairs))

	// result 通道缓冲区大小等于 worker 数量，
	// 确保每个 worker 在发现差异时都能非阻塞地发送结果。
	result := make(chan error, n)

	var wg sync.WaitGroup
	wg.Add(n)
	for i := 0; i < n; i++ {
		go func() {
			defer wg.Done()
			for p := range work {
				h1, err := fileHash(p.a)
				if err != nil {
					result <- err
					return
				}
				h2, err := fileHash(p.b)
				if err != nil {
					result <- err
					return
				}
				if h1 != h2 {
					result <- errDiff
					return
				}
			}
		}()
	}

	// 将所有文件对发送到工作通道，然后关闭通道通知 worker 结束。
	for _, p := range pairs {
		work <- p
	}
	close(work)

	// 等待所有 worker 处理完毕，然后关闭结果通道。
	wg.Wait()
	close(result)

	// 读取第一个结果（如果存在）。
	// 由于每个 worker 最多发送一个错误，且缓冲区足够大，
	// 不会发生丢失结果的情况。
	if err, ok := <-result; ok {
		if errors.Is(err, errDiff) {
			return false, nil
		}
		return false, err
	}
	return true, nil
}
