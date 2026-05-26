# cmp_hash

一个基于 xxHash 快速比较两个文件或目录内容是否相同的命令行工具。

## 特性

- **xxHash 高速哈希** — 比 SHA-256 快 10-20 倍，CPU 不再成为瓶颈
- **大小快速路径** — 文件大小不同时直接判定差异，跳过哈希
- **并发比较** — worker pool 并行处理文件，worker 数等于逻辑 CPU 核数
- **默认快速模式** — 发现第一处文件内容差异即停止，输出提示信息
- **--diff-all 全量模式** — 完整列出所有差异项
- **详细差异输出** — 列出缺失文件、多余文件、内容不同的文件
- **大缓冲区 I/O** — 1 MiB 缓冲区 + sync.Pool 复用，减少系统调用
- **符号链接安全** — 不追随符号链接，仅比较普通文件内容

## 安装

```bash
go build -o cmp_hash .
```

交叉编译到其他平台：设置 `GOOS` 和 `GOARCH` 环境变量为目标平台后执行 `go build` 即可。Go 原生支持交叉编译，无需额外工具链。

例如编译 Windows 版本，在任一平台上设置环境变量后构建：

```
GOOS=windows
GOARCH=amd64
```

然后执行：

```bash
go build -o cmp_hash.exe .
```

常见平台组合：

| GOOS    | GOARCH | 产物           |
| ------- | ------ | -------------- |
| windows | amd64  | cmp_hash.exe   |
| darwin  | amd64  | cmp_hash       |
| darwin  | arm64  | cmp_hash       |
| linux   | amd64  | cmp_hash       |
| linux   | arm64  | cmp_hash       |

（环境变量的设置方式因平台而异：Unix 类系统用 `export` 或行内前缀，Windows CMD 用 `set`，PowerShell 用 `$env:`。Go 只关心变量最终的值，不关心设置方式。）

## 用法

```bash
cmp_hash [--diff-all] <path1> <path2>
```

两个路径必须同为文件或同为目录。

### 文件比较

```bash
$ cmp_hash a.txt b.txt
Same

$ cmp_hash a.txt c.txt
Different:
  Content differs
```

### 目录比较 — 默认快速模式

发现第一处文件内容差异后立即停止：

```bash
$ cmp_hash dir1/ dir2/
Different:
  Only in dir1/: only_in_dir1.txt
  Only in dir2/: only_in_dir2.txt
  Content differs: common.txt

注意：仅发现第一处文件内容不同，可能存在更多差异。使用 --diff-all 查看完整差异。
```

- `Only in` — 该文件仅在一侧存在（始终全部列出）
- `Content differs` — 两侧路径相同但内容不同（默认仅列出第一处）
- 末尾提示告诉用户结果不完整

### 目录比较 — 全量模式

```bash
$ cmp_hash --diff-all dir1/ dir2/
Different:
  Only in dir1/: only_in_dir1.txt
  Only in dir2/: only_in_dir2.txt
  Content differs: common.txt
  Content differs: other.txt
```

列出所有差异项，无提示信息。

## 原理

1. **递归遍历** — `filepath.WalkDir` 收集两个目录下所有普通文件的相对路径和大小
2. **结构比较** — 对比文件路径集合，找出仅在某一侧存在的文件（始终全量）
3. **内容比较** — 对共同文件使用 worker pool 并发处理：
   - 先比文件大小：不同则直接记为差异，跳过哈希
   - 大小相同则计算 xxHash（64 位，碰撞概率 ~1/2^64）
4. **结果输出** — 差异项按相对路径字母排序

## 性能

xxHash 单核吞吐约 15-20 GB/s，远超主流 SSD 读取速度。1 MiB I/O 缓冲区将系统调用减少为默认 Go 的 1/32。默认模式下发现差异后立即停止，对"不同"场景可提前结束。

## 注意事项

- 仅比较普通文件，目录、符号链接、设备文件等会被忽略
- 目录比较时会遍历所有子目录，比较完整的文件树
- xxHash 是非加密级哈希，碰撞概率极低但非零。若需加密级保证请使用 SHA-256 版本
