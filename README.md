# cmp_hash

一个基于 SHA-256 哈希比较两个文件或目录内容是否相同的命令行工具。

## 特性

- **SHA-256 流式哈希** — 不将文件整体加载到内存，支持大文件
- **并发目录比较** — 使用 worker pool 并行计算文件哈希，worker 数量等于逻辑 CPU 核数
- **详细差异输出** — 目录比较时列出具体差异：缺失文件、多余文件、内容不同的文件
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
cmp_hash <path1> <path2>
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

### 目录比较

相同目录：

```bash
$ cmp_hash dir1/ dir2/
Same
```

存在差异时，输出具体到每个文件：

```bash
$ cmp_hash dir1/ dir2/
Different:
  Only in dir1/: only_in_dir1.txt
  Only in dir2/: only_in_dir2.txt
  Content differs: common.txt
```

- `Only in` — 该文件仅在一侧存在
- `Content differs` — 两侧路径相同但内容不同

## 原理

1. **文件比较** — 分别计算两个文件的 SHA-256 哈希，比对是否相同
2. **目录比较**：
   - 递归遍历两个目录，收集所有普通文件的相对路径
   - 比较文件数量及路径集合，找出仅在某一侧存在的文件
   - 对共同文件使用 worker pool 并发计算哈希并逐一比对
   - 汇总输出所有差异项（按相对路径字母排序）

## 注意事项

- 仅比较普通文件，目录、符号链接、设备文件等会被忽略
- 目录比较时会遍历所有子目录，比较完整的文件树
- 哈希计算使用 SHA-256，理论上不存在碰撞
