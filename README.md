# cmp_hash

一个基于 xxHash 快速比较两个文件或目录内容是否相同的命令行工具。

## 特性

- **xxHash 高速哈希** — 64 位非加密哈希，比 SHA-256 快 10-20 倍
- **大小快速路径** — 文件大小不同时跳过哈希，直接判定差异
- **默认串行快速模式** — 结构检查 → 大小比对 → 哈希比对，发现第一处差异即返回
- **--diff-all 并发全量模式** — worker pool 并行处理，收集全部差异
- **1 MiB I/O 缓冲区** — sync.Pool 复用，系统调用减少为默认 Go 的 1/32
- **符号链接安全** — 不追随符号链接，仅比较普通文件内容

## 安装

```bash
go build -o cmp_hash .
```

交叉编译到其他平台：设置 `GOOS` 和 `GOARCH` 环境变量为目标平台后执行 `go build` 即可。Go 原生支持交叉编译，无需额外工具链。

常见平台组合：

| GOOS    | GOARCH | 产物           |
| ------- | ------ | -------------- |
| windows | amd64  | cmp_hash.exe   |
| darwin  | amd64  | cmp_hash       |
| darwin  | arm64  | cmp_hash       |
| linux   | amd64  | cmp_hash       |
| linux   | arm64  | cmp_hash       |

环境变量的设置方式因平台而异（Unix 类系统用 `export`、Windows CMD 用 `set`、PowerShell 用 `$env:`），Go 只关心变量最终的值。

## 用法

```bash
cmp_hash [--diff-all] <path1> <path2>
```

两个路径必须同为文件或同为目录。

### 文件比较

先比大小，相同则比 xxHash：

```bash
$ cmp_hash a.txt b.txt
Same

$ cmp_hash a.txt c.txt
Different:
  Content differs
```

### 目录比较 — 默认模式

完全串行：结构 → 大小 → 哈希，**第一处差异即停止**。

结构差异（文件仅在某一侧存在）：

```bash
$ cmp_hash dir1/ dir2/
Different:
  Only in dir1/: extra.txt

注意：仅发现第一处不同，可能存在更多差异。使用 --diff-all 查看完整差异。
```

内容差异（结构相同但某个文件大小或哈希不同）：

```bash
$ cmp_hash dir1/ dir2/
Different:
  Content differs: data.bin

注意：仅发现第一处不同，可能存在更多差异。使用 --diff-all 查看完整差异。
```

### 目录比较 — --diff-all 全量模式

并发 worker pool 处理，**列出全部差异**：

```bash
$ cmp_hash --diff-all dir1/ dir2/
Different:
  Only in dir1/: extra.txt
  Only in dir2/: missing.txt
  Content differs: data.bin
  Content differs: config.json
```

## 原理

**默认模式（串行，首差异即退）：**

1. `filepath.WalkDir` 递归收集两个目录下所有普通文件的路径和大小
2. 检查文件路径集合 —— 首个仅在 dir1 或仅在 dir2 的文件即返回
3. 共同文件按路径排序，串行迭代：先比大小，不同则返回；相同则计算 xxHash 确认

**--diff-all 模式（并发，全量收集）：**

1. 同默认模式收集文件信息
2. 结构差异全量收集
3. 共同文件使用 worker pool（worker 数 = 逻辑 CPU 核数）并发计算 xxHash
4. 差异项按路径字母排序输出

## 性能

- xxHash 单核吞吐约 15-20 GB/s，远超主流 SSD 读取速度
- 1 MiB I/O 缓冲区将系统调用减少为默认 Go 的 1/32
- 默认模式串行执行，无 goroutine/channel 开销，发现第一处差异立即退出
- --diff-all 模式并发哈希，充分利用多核 CPU

## 注意事项

- 仅比较普通文件，目录、符号链接、设备文件等会被忽略
- 目录比较时会遍历所有子目录，比较完整的文件树
- 默认模式 map 迭代顺序随机，具体"第一处"差异的路径可能因运行而异
- xxHash 是非加密级哈希，64 位碰撞概率约 1/2^64，对内容比对完全足够
