// 本文件包含 cmp_hash 的核心逻辑：计算文件 SHA-256 哈希、遍历目录、
// 以及文件/目录的内容比较。
//
// 文件比较：分别计算两个文件的 SHA-256 哈希值，比较是否相同。
// 目录比较：递归遍历两个目录，收集所有普通文件的相对路径，先找出仅存在于
// 一侧的文件，再对共同文件使用 worker pool 并发计算哈希进行比对，
// 最终汇总输出所有差异（缺失文件、多余文件、内容不同的文件）。
//
// 目录比较通过 worker pool 实现并发哈希计算，worker 数量等于逻辑 CPU 核数。
// 符号链接（软链接）不会被纳入比较——只比较普通文件的内容。
package main

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"sync"
)

// Diff 记录一次比较的完整结果。Same 为 true 时其余字段均为空。
type Diff struct {
	Same    bool     // 两个目标内容是否完全一致
	OnlyIn1 []string // 仅在 path1 中存在的相对路径（目录比较时）
	OnlyIn2 []string // 仅在 path2 中存在的相对路径（目录比较时）
	Differ  []string // 内容不同的相对路径（目录比较时列出相对路径，文件比较时列出两个绝对路径）
}

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
func CompareFiles(path1, path2 string) (*Diff, error) {
	h1, err := fileHash(path1)
	if err != nil {
		return nil, err
	}
	h2, err := fileHash(path2)
	if err != nil {
		return nil, err
	}
	if h1 == h2 {
		return &Diff{Same: true}, nil
	}
	return &Diff{
		Same:   false,
		Differ: []string{path1, path2},
	}, nil
}

// CompareDirs 比较两个目录的内容是否相同，包括文件树结构和所有文件的内容。
// 返回的 Diff 中详细列出了仅在某一侧出现的文件以及内容不同的文件。
// 所有路径字段均为升序排列，确保输出稳定。
func CompareDirs(dir1, dir2 string) (*Diff, error) {
	files1, err := walkFiles(dir1)
	if err != nil {
		return nil, err
	}
	files2, err := walkFiles(dir2)
	if err != nil {
		return nil, err
	}

	diff := &Diff{Same: true}

	// 找出仅在 dir1 中存在的文件。
	for rel := range files1 {
		if _, ok := files2[rel]; !ok {
			diff.OnlyIn1 = append(diff.OnlyIn1, rel)
			diff.Same = false
		}
	}
	sort.Strings(diff.OnlyIn1)

	// 找出仅在 dir2 中存在的文件。
	for rel := range files2 {
		if _, ok := files1[rel]; !ok {
			diff.OnlyIn2 = append(diff.OnlyIn2, rel)
			diff.Same = false
		}
	}
	sort.Strings(diff.OnlyIn2)

	// 构建共同文件对列表，用于并发比对内容。
	type pair struct {
		rel string
		a   string
		b   string
	}
	pairs := make([]pair, 0, len(files1))
	for rel, abs1 := range files1 {
		if abs2, ok := files2[rel]; ok {
			pairs = append(pairs, pair{rel, abs1, abs2})
		}
	}
	if len(pairs) == 0 {
		sort.Strings(diff.Differ) // 确保空切片而非 nil
		return diff, nil
	}

	// 确定 worker 数量，不超过文件对总数。
	n := runtime.NumCPU()
	if n > len(pairs) {
		n = len(pairs)
	}

	// work 通道缓冲区大小设为文件对总数，避免发送端阻塞。
	work := make(chan pair, len(pairs))

	// 每个 worker 完成任务后向 done 发送其发现的差异文件列表。
	type taskResult struct {
		rel  string
		err  error
		diff bool
	}
	done := make(chan taskResult, n)

	var wg sync.WaitGroup
	wg.Add(n)
	for i := 0; i < n; i++ {
		go func() {
			defer wg.Done()
			for p := range work {
				h1, err := fileHash(p.a)
				if err != nil {
					done <- taskResult{rel: p.rel, err: err}
					return
				}
				h2, err := fileHash(p.b)
				if err != nil {
					done <- taskResult{rel: p.rel, err: err}
					return
				}
				if h1 != h2 {
					done <- taskResult{rel: p.rel, diff: true}
					return
				}
			}
		}()
	}

	for _, p := range pairs {
		work <- p
	}
	close(work)
	wg.Wait()
	close(done)

	// 汇总所有内容差异的文件。
	for r := range done {
		if r.err != nil {
			return nil, r.err
		}
		if r.diff {
			diff.Differ = append(diff.Differ, r.rel)
			diff.Same = false
		}
	}
	sort.Strings(diff.Differ)

	return diff, nil
}
