// 本文件包含 cmp_hash 的核心逻辑：计算文件 xxHash 哈希、遍历目录、
// 以及文件/目录的内容比较。
//
// 文件比较：先比较大小，相同则用 xxHash 比对内容。
// 目录比较：递归遍历两个目录，收集所有普通文件的相对路径及大小，
// 先找出仅存在于一侧的文件（始终全量收集），再对共同文件使用 worker pool
// 并发比对。默认模式下发现第一处文件内容差异即停止；--diff-all 模式下
// 全量收集所有差异。
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

// pair 表示一对需要比对内容的共同文件。
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

// CompareDirs 比较两个目录的内容是否相同，包括文件树结构和所有文件的内容。
//
// diffAll 为 false（默认模式）：结构差异全量收集，但内容比较在发现第一处差异后
// 即停止，Diff.Partial 设为 true。
//
// diffAll 为 true：全量收集所有差异，Diff.Partial 始终为 false。
//
// 所有路径字段均为升序排列，确保输出稳定。
func CompareDirs(dir1, dir2 string, diffAll bool) (*Diff, error) {
	files1, err := walkFiles(dir1)
	if err != nil {
		return nil, err
	}
	files2, err := walkFiles(dir2)
	if err != nil {
		return nil, err
	}

	diff := &Diff{Same: true}

	// 结构差异始终全量收集（仅 map 遍历，无 I/O）。
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

	// 默认模式下，若已发现结构差异则无需哈希共同文件。
	if !diffAll && !diff.Same {
		return diff, nil
	}

	n := runtime.NumCPU()
	if n > len(pairs) {
		n = len(pairs)
	}

	// 默认模式：quit channel 用于 worker 提前退出。
	var quit chan struct{}
	var quitOnce sync.Once
	cancel := func() {}
	if !diffAll {
		quit = make(chan struct{})
		cancel = func() { quitOnce.Do(func() { close(quit) }) }
	}
	defer cancel()

	work := make(chan pair, n)
	dn := n
	if diffAll {
		dn = len(pairs)
	}
	doneCh := make(chan taskResult, dn)
	doneSend := (chan<- taskResult)(doneCh)

	var wg sync.WaitGroup
	wg.Add(n)
	for i := 0; i < n; i++ {
		go func() {
			defer wg.Done()
			for {
				if quit != nil {
					select {
					case <-quit:
						return
					case p, ok := <-work:
						if !ok {
							return
						}
						processPair(p, doneSend, cancel)
					}
				} else {
					p, ok := <-work
					if !ok {
						return
					}
					processPair(p, doneSend, nil)
				}
			}
		}()
	}

	// 在独立 goroutine 中发放任务，以便响应 quit 信号。
	go func() {
		defer close(work)
		for _, p := range pairs {
			if quit != nil {
				select {
				case <-quit:
					return
				case work <- p:
				}
			} else {
				work <- p
			}
		}
	}()

	// 等待所有 worker 完成后关闭结果通道。
	go func() {
		wg.Wait()
		close(doneCh)
	}()

	// 收集结果。
	for r := range doneCh {
		if r.err != nil {
			return nil, r.err
		}
		if r.diff {
			diff.Differ = append(diff.Differ, r.rel)
			diff.Same = false
			if !diffAll {
				diff.Partial = true
				cancel() // 通知 worker 和 feeder 停止
				break    // 默认模式不再等待更多结果
			}
		}
	}
	sort.Strings(diff.Differ)

	return diff, nil
}

// processPair 处理单个文件对：先比较大小，大小不同直接记差异；
// 大小相同则计算 xxHash 比对。
func processPair(p pair, done chan<- taskResult, cancel func()) {
	if p.sizeA != p.sizeB {
		select {
		case done <- taskResult{rel: p.rel, diff: true}:
		default:
		}
		if cancel != nil {
			cancel()
		}
		return
	}

	h1, err := fileHash(p.a)
	if err != nil {
		select {
		case done <- taskResult{rel: p.rel, err: err}:
		default:
		}
		if cancel != nil {
			cancel()
		}
		return
	}
	h2, err := fileHash(p.b)
	if err != nil {
		select {
		case done <- taskResult{rel: p.rel, err: err}:
		default:
		}
		if cancel != nil {
			cancel()
		}
		return
	}
	if h1 != h2 {
		select {
		case done <- taskResult{rel: p.rel, diff: true}:
		default:
		}
		if cancel != nil {
			cancel()
		}
	}
}
