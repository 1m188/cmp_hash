// 本文件包含 cmp_hash 的核心逻辑：计算文件 xxHash 哈希、遍历目录、
// 以及文件/目录的内容比较。
//
// 文件比较：先比较大小，相同则用 xxHash 比对内容。
// 目录比较：
//   - 默认模式（串行）：结构检查 → 大小比对 → 哈希比对，第一处差异即返回。
//   - --diff-all 模式（并发）：worker pool 全量收集所有差异。
//
// worker 数量等于逻辑 CPU 核数。文件读取使用 1 MiB 缓冲区（sync.Pool 复用），
// 减少系统调用开销。符号链接不会被纳入比较。
package main

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"sync"

	"github.com/cespare/xxhash/v2"
)

// bufPool 复用 I/O 缓冲区，每个文件哈希时从中获取并归还，
// 避免频繁分配 1 MiB 内存。
var bufPool = sync.Pool{
	New: func() any { return make([]byte, 1<<20) },
}

// fileEntry 记录一个普通文件的绝对路径和大小，
// 供后续快速判断是否需要哈希比对。
type fileEntry struct {
	path string
	size int64
}

// pair 表示一对需要比对内容的共同文件（仅并发模式使用）。
type pair struct {
	rel          string
	a, b         string
	sizeA, sizeB int64
}

// taskResult 是单个文件对的比对结果。
type taskResult struct {
	rel  string
	err  error
	diff bool
}

// Diff 记录一次比较的完整结果。
// Same 为 true 时其余字段均为空；Partial 为 true 时表示内容比较被提前截断，
// Differ 中仅包含第一处内容差异（默认目录比较模式）。
type Diff struct {
	Same    bool
	Partial bool     // 内容比较是否被截断（仅默认模式）
	OnlyIn1 []string // 仅在 path1 中存在的相对路径
	OnlyIn2 []string // 仅在 path2 中存在的相对路径
	Differ  []string // 内容不同的相对路径；文件比较时存放两个绝对路径
}

// fileHash 使用 xxHash 计算文件的 64 位哈希值。
// 从 bufPool 获取 I/O 缓冲区，流式读取文件，不会将整个文件加载到内存。
func fileHash(path string) (uint64, error) {
	f, err := os.Open(path)
	if err != nil {
		return 0, fmt.Errorf("open %s: %w", path, err)
	}
	defer f.Close()

	h := xxhash.New()
	buf := bufPool.Get().([]byte)
	_, err = io.CopyBuffer(h, f, buf)
	bufPool.Put(buf)
	if err != nil {
		return 0, fmt.Errorf("read %s: %w", path, err)
	}
	return h.Sum64(), nil
}

// walkFiles 递归遍历 root 目录，返回一个 map，键为文件相对于 root 的路径，
// 值为 fileEntry（包含绝对路径和文件大小）。只收集普通文件，目录和符号链接会被忽略。
func walkFiles(root string) (map[string]fileEntry, error) {
	files := make(map[string]fileEntry)
	err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.Type().IsRegular() {
			return nil
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		files[rel] = fileEntry{path: path, size: info.Size()}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("walk %s: %w", root, err)
	}
	return files, nil
}

// CompareFiles 比较两个文件的内容是否相同。
// 先比较文件大小，不同则直接判定内容不同；相同则用 xxHash 做二次确认。
func CompareFiles(path1, path2 string) (*Diff, error) {
	info1, err := os.Stat(path1)
	if err != nil {
		return nil, err
	}
	info2, err := os.Stat(path2)
	if err != nil {
		return nil, err
	}
	if info1.Size() != info2.Size() {
		return &Diff{Same: false, Differ: []string{path1, path2}}, nil
	}
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
	return &Diff{Same: false, Differ: []string{path1, path2}}, nil
}

// CompareDirs 比较两个目录的内容是否相同。
//
// diffAll 为 false（默认模式）：完全串行——结构检查 → 大小比对 → 哈希比对，
// 发现第一处差异即返回，Diff.Partial 设为 true（哈希阶段发现差异时）。
//
// diffAll 为 true：并发 worker pool 全量收集所有差异。
func CompareDirs(dir1, dir2 string, diffAll bool) (*Diff, error) {
	files1, err := walkFiles(dir1)
	if err != nil {
		return nil, err
	}
	files2, err := walkFiles(dir2)
	if err != nil {
		return nil, err
	}

	if diffAll {
		return compareDirsAll(files1, files2)
	}
	return compareDirsDefault(files1, files2)
}

// compareDirsDefault 串行比较，发现第一处差异即返回。
func compareDirsDefault(files1, files2 map[string]fileEntry) (*Diff, error) {
	// 结构检查：首个仅在 dir1 中的文件。
	for rel := range files1 {
		if _, ok := files2[rel]; !ok {
			return &Diff{Same: false, OnlyIn1: []string{rel}}, nil
		}
	}
	// 结构检查：首个仅在 dir2 中的文件。
	for rel := range files2 {
		if _, ok := files1[rel]; !ok {
			return &Diff{Same: false, OnlyIn2: []string{rel}}, nil
		}
	}

	// 共同文件按相对路径排序，串行比对大小和哈希。
	common := make([]string, 0, len(files1))
	for rel := range files1 {
		if _, ok := files2[rel]; ok {
			common = append(common, rel)
		}
	}
	sort.Strings(common)

	for _, rel := range common {
		e1, e2 := files1[rel], files2[rel]

		// 大小不同则内容必定不同。
		if e1.size != e2.size {
			return &Diff{Same: false, Partial: true, Differ: []string{rel}}, nil
		}

		// 大小相同，计算哈希确认。
		h1, err := fileHash(e1.path)
		if err != nil {
			return nil, err
		}
		h2, err := fileHash(e2.path)
		if err != nil {
			return nil, err
		}
		if h1 != h2 {
			return &Diff{Same: false, Partial: true, Differ: []string{rel}}, nil
		}
	}

	return &Diff{Same: true}, nil
}

// compareDirsAll 使用 worker pool 并发比对，全量收集所有差异。
func compareDirsAll(files1, files2 map[string]fileEntry) (*Diff, error) {
	diff := &Diff{Same: true}

	// 结构差异全量收集。
	for rel := range files1 {
		if _, ok := files2[rel]; !ok {
			diff.OnlyIn1 = append(diff.OnlyIn1, rel)
			diff.Same = false
		}
	}
	sort.Strings(diff.OnlyIn1)

	for rel := range files2 {
		if _, ok := files1[rel]; !ok {
			diff.OnlyIn2 = append(diff.OnlyIn2, rel)
			diff.Same = false
		}
	}
	sort.Strings(diff.OnlyIn2)

	// 构建共同文件对列表。
	pairs := make([]pair, 0, len(files1))
	for rel, e1 := range files1 {
		if e2, ok := files2[rel]; ok {
			pairs = append(pairs, pair{rel, e1.path, e2.path, e1.size, e2.size})
		}
	}
	if len(pairs) == 0 {
		return diff, nil
	}

	n := runtime.NumCPU()
	if n > len(pairs) {
		n = len(pairs)
	}

	work := make(chan pair, n)
	// diffAll 模式 buffer 设为文件对总数，确保 non-blocking send 不丢结果。
	doneCh := make(chan taskResult, len(pairs))
	doneSend := (chan<- taskResult)(doneCh)

	var wg sync.WaitGroup
	wg.Add(n)
	for i := 0; i < n; i++ {
		go func() {
			defer wg.Done()
			for p := range work {
				if p.sizeA != p.sizeB {
					select {
					case doneSend <- taskResult{rel: p.rel, diff: true}:
					default:
					}
					continue
				}
				h1, err := fileHash(p.a)
				if err != nil {
					select {
					case doneSend <- taskResult{rel: p.rel, err: err}:
					default:
					}
					continue
				}
				h2, err := fileHash(p.b)
				if err != nil {
					select {
					case doneSend <- taskResult{rel: p.rel, err: err}:
					default:
					}
					continue
				}
				if h1 != h2 {
					select {
					case doneSend <- taskResult{rel: p.rel, diff: true}:
					default:
					}
				}
			}
		}()
	}

	// 发放所有任务。
	go func() {
		defer close(work)
		for _, p := range pairs {
			work <- p
		}
	}()

	// 等待 worker 完成后关闭结果通道。
	go func() {
		wg.Wait()
		close(doneCh)
	}()

	// 收集全部结果。
	for r := range doneCh {
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
