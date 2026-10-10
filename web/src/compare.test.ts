import { describe, expect, it } from 'vitest'
import {
  compareTrees,
  defaultConcurrency,
  type ComparePairFn,
  type Entry,
  type PairOutcome,
} from './compare'

const e = (rel: string, size: number): Entry => ({ rel, size })

/** 可编程的 comparePair 替身：记录调用、按 outcomes 返回，默认 'same'。 */
function fakeComparePair(outcomes: Record<string, PairOutcome> = {}) {
  const calls: string[] = []
  const fn: ComparePairFn = (rel) => {
    calls.push(rel)
    return Promise.resolve(outcomes[rel] ?? 'same')
  }
  return { fn, calls }
}

describe('compareTrees', () => {
  it('两棵树完全相同时报告 same，并对所有共同文件调用 comparePair', async () => {
    const { fn, calls } = fakeComparePair()
    const diff = await compareTrees(
      [e('a.txt', 5), e('sub/b.txt', 3)],
      [e('a.txt', 5), e('sub/b.txt', 3)],
      fn,
    )
    expect(diff.same).toBe(true)
    expect(diff.onlyIn1).toEqual([])
    expect(diff.onlyIn2).toEqual([])
    expect(diff.differ).toEqual([])
    expect(diff.readErrors).toEqual([])
    // 需要哈希的公共文件按相对路径排序后逐个比对
    expect(calls).toEqual(['a.txt', 'sub/b.txt'])
  })

  it('内容差异经 comparePair 收集，输出按路径排序', async () => {
    const { fn } = fakeComparePair({ 'b.txt': 'differ', 'a.txt': 'differ' })
    const diff = await compareTrees(
      [e('b.txt', 1), e('a.txt', 1)],
      [e('b.txt', 1), e('a.txt', 1)],
      fn,
    )
    expect(diff.same).toBe(false)
    expect(diff.differ).toEqual(['a.txt', 'b.txt'])
  })

  it('大小不同的共同文件走快速路径判差，不调用 comparePair', async () => {
    const { fn, calls } = fakeComparePair()
    const diff = await compareTrees([e('big.bin', 100)], [e('big.bin', 200)], fn)
    expect(diff.same).toBe(false)
    expect(diff.differ).toEqual(['big.bin'])
    expect(calls).toEqual([])
  })

  it('两侧结构差异全量收集且各自排序（--diff-all 语义）', async () => {
    const { fn, calls } = fakeComparePair()
    const diff = await compareTrees(
      [e('z_only.txt', 1), e('a_only.txt', 2), e('common.txt', 1)],
      [e('b_only.txt', 2), e('common.txt', 1)],
      fn,
    )
    expect(diff.same).toBe(false)
    expect(diff.onlyIn1).toEqual(['a_only.txt', 'z_only.txt'])
    expect(diff.onlyIn2).toEqual(['b_only.txt'])
    expect(diff.differ).toEqual([])
    expect(calls).toEqual(['common.txt'])
  })

  it('两棵空树报告 same', async () => {
    const { fn } = fakeComparePair()
    const diff = await compareTrees([], [], fn)
    expect(diff.same).toBe(true)
  })

  it('单个文件的读取错误被记录，不中断其余文件比对', async () => {
    const fn: ComparePairFn = (rel) => {
      if (rel === 'bad.txt') return Promise.reject(new Error('disk full'))
      return Promise.resolve(rel === 'diff.txt' ? 'differ' : 'same')
    }
    const diff = await compareTrees(
      [e('bad.txt', 1), e('diff.txt', 1), e('ok.txt', 1)],
      [e('bad.txt', 1), e('diff.txt', 1), e('ok.txt', 1)],
      fn,
      2,
    )
    expect(diff.same).toBe(false)
    expect(diff.differ).toEqual(['diff.txt'])
    expect(diff.readErrors).toEqual([{ rel: 'bad.txt', message: 'disk full' }])
  })

  it('只有读取错误、无真实差异时 same 仍为 true（错误单独报告）', async () => {
    const fn: ComparePairFn = () => Promise.reject(new Error('read failed'))
    const diff = await compareTrees([e('x.txt', 1)], [e('x.txt', 1)], fn)
    expect(diff.same).toBe(true)
    expect(diff.readErrors).toHaveLength(1)
    expect(diff.readErrors[0].rel).toBe('x.txt')
  })

  it('comparePair 的并发被限制在 concurrency 以内', async () => {
    let inFlight = 0
    let peak = 0
    const fn: ComparePairFn = async (rel) => {
      inFlight++
      peak = Math.max(peak, inFlight)
      await new Promise((r) => setTimeout(r, 1))
      inFlight--
      return rel.endsWith('_d.txt') ? 'differ' : 'same'
    }
    const entries = Array.from({ length: 10 }, (_, i) => e(`f${i}${i % 3 === 0 ? '_d' : ''}.txt`, 1))
    const diff = await compareTrees(entries, entries.map((x) => ({ ...x })), fn, 3)
    expect(peak).toBe(3)
    expect(diff.differ).toHaveLength(4)
  })
})

describe('defaultConcurrency', () => {
  it('返回 [1, 8] 范围内的正整数', () => {
    const c = defaultConcurrency()
    expect(Number.isInteger(c)).toBe(true)
    expect(c).toBeGreaterThanOrEqual(1)
    expect(c).toBeLessThanOrEqual(8)
  })
})
