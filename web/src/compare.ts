// 目录对比核心逻辑（纯逻辑，无 I/O 依赖），与 CLI 版 hash.go 中
// --diff-all 模式的语义对齐：
//   1. 结构对比：相对路径集合差 → onlyIn1 / onlyIn2（全量收集，排序输出）
//   2. 大小快速路径：同路径文件大小不同 → 直接判差，不做哈希
//   3. 内容对比：剩余共同文件通过 comparePair 回调比对，并发受 concurrency 限制
//
// 读取错误不会中断整体比较，而是收集在 readErrors 中单独报告；
// 仅当存在结构或内容差异时 same 为 false（只有读取错误时视为"未发现差异"）。

export interface Entry {
  readonly rel: string
  readonly size: number
}

export type PairOutcome = 'same' | 'differ'

export type ComparePairFn = (rel: string) => Promise<PairOutcome>

export interface ReadError {
  readonly rel: string
  readonly message: string
}

export interface TreeDiff {
  readonly same: boolean
  readonly onlyIn1: readonly string[]
  readonly onlyIn2: readonly string[]
  readonly differ: readonly string[]
  readonly readErrors: readonly ReadError[]
}

/** 与 CLI 的 runtime.NumCPU() worker pool 对应，限制在 [1, 8]。 */
export function defaultConcurrency(): number {
  const hc = typeof navigator !== 'undefined' ? navigator.hardwareConcurrency : 0
  return Math.max(1, Math.min(hc || 4, 8))
}

export async function compareTrees(
  entries1: readonly Entry[],
  entries2: readonly Entry[],
  comparePair: ComparePairFn,
  concurrency: number = defaultConcurrency(),
): Promise<TreeDiff> {
  const map1 = new Map(entries1.map((e) => [e.rel, e]))
  const map2 = new Map(entries2.map((e) => [e.rel, e]))

  const onlyIn1 = entries1.filter((e) => !map2.has(e.rel)).map((e) => e.rel).sort()
  const onlyIn2 = entries2.filter((e) => !map1.has(e.rel)).map((e) => e.rel).sort()

  // 大小快速路径：大小不同直接判差；其余进入哈希队列。
  const needHash: string[] = []
  const differ: string[] = []
  for (const e1 of entries1) {
    const e2 = map2.get(e1.rel)
    if (!e2) continue
    if (e1.size !== e2.size) {
      differ.push(e1.rel)
    } else {
      needHash.push(e1.rel)
    }
  }
  needHash.sort()

  const readErrors: ReadError[] = []
  let next = 0
  let inFlight = 0

  await new Promise<void>((resolve) => {
    const pump = (): void => {
      if (next >= needHash.length && inFlight === 0) {
        resolve()
        return
      }
      while (inFlight < concurrency && next < needHash.length) {
        const rel = needHash[next++]
        inFlight++
        comparePair(rel)
          .then((outcome) => {
            if (outcome === 'differ') differ.push(rel)
          })
          .catch((err: unknown) => {
            readErrors.push({ rel, message: errorMessage(err) })
          })
          .finally(() => {
            inFlight--
            pump()
          })
      }
    }
    pump()
  })

  differ.sort()
  readErrors.sort((a, b) => (a.rel < b.rel ? -1 : a.rel > b.rel ? 1 : 0))

  const same = onlyIn1.length === 0 && onlyIn2.length === 0 && differ.length === 0
  return { same, onlyIn1, onlyIn2, differ, readErrors }
}

function errorMessage(err: unknown): string {
  return err instanceof Error ? err.message : String(err)
}
