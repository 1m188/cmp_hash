# cmp_hash web 版

cmp_hash 的浏览器版本：选择两个文件或两个目录，使用 **xxHash64（XXH64）** 计算哈希并比较内容是否相同。与 CLI 版使用相同算法，比较语义对齐 CLI 的 `--diff-all` 模式。

所有计算均在浏览器本地完成（Web Worker + 分块流式读取），**文件不会离开你的设备**。

## 开发

```bash
cd web
npm install
npm run dev        # 开发服务器
npm test           # Vitest 单元测试
npm run build      # 类型检查 + 构建到 dist/
npm run preview    # 本地预览构建产物
```

## 浏览器端行为与 CLI 的对应关系

| CLI（Go） | Web（TS） |
| --- | --- |
| `filepath.WalkDir` 收集普通文件 | `<input webkitdirectory>` 的 `webkitRelativePath`（去掉所选根目录名） |
| 结构对比：相对路径集合差 | 同（`onlyIn1` / `onlyIn2`，全量收集，排序输出） |
| 大小快速路径：大小不同免哈希 | 同（`File.size`） |
| xxHash64 流式哈希（1 MiB 缓冲） | hash-wasm XXH64 分块流式（8 MiB 块，Web Worker） |
| `--diff-all` worker pool（`runtime.NumCPU()`） | Web Worker 池（`navigator.hardwareConcurrency`，上限 8） |
| 仅比较普通文件，忽略目录/符号链接 | 浏览器只列出普通文件，行为一致 |

### 已知差异

- **空目录**：浏览器选择目录时拿不到空目录（没有文件的目录不会出现在文件列表里），空目录不参与比较——与 CLI 忽略目录本身的行为一致。
- **读取错误**：CLI 遇到读取错误直接退出；web 版会继续比较其余文件，把失败项单独列在「读取失败」区，并在结果标注「无法完全确定」。
- **默认模式（首差异即停）**：web 版只做全量模式（用户在浏览器中等待结果，全量输出更符合预期），未实现串行首差异模式。

### 兼容性

- 目录选择使用 `webkitdirectory`：Chrome / Edge / Firefox / Safari 均支持。
- 现代浏览器（ES2022 + module worker）：Chrome/Edge ≥ 90、Firefox ≥ 114、Safari ≥ 15。
- 文件模式（单文件对）与目录模式共用同一套 worker 基础设施。

## 目录结构

```
web/
├── index.html            # 单页入口
├── src/
│   ├── main.ts           # UI：选择、调度、进度、渲染
│   ├── compare.ts        # 核心对比逻辑（结构差 → 大小 → 哈希，纯逻辑可单测）
│   ├── compare.test.ts   # 对比逻辑测试（对齐 CLI --diff-all 语义）
│   ├── hashStream.ts     # 流式分块 XXH64
│   ├── hashStream.test.ts# 哈希测试（含 Go cespare/xxhash 参考向量校验）
│   ├── pool.ts           # 主线程 Web Worker 池
│   ├── hashWorker.ts     # worker：分块读取 + 哈希 + 进度上报
│   └── style.css
└── vite.config.ts        # base './'（Pages 子路径友好）
```
