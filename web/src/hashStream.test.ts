import { describe, expect, it } from 'vitest'
import { xxhash64 } from 'hash-wasm'
import { xxhash64Hex } from './hashStream'

/** 生成确定性 pattern：第 i 字节 = i mod 256（与 Go 探针 byte(i) 一致）。 */
function patternBytes(n: number): Uint8Array<ArrayBuffer> {
  const out = new Uint8Array(n)
  for (let i = 0; i < n; i++) out[i] = i & 0xff
  return out
}

// 由仓库内 Go 探针（cespare/xxhash/v2 v2.3.0）计算的参考向量，
// 保证 web 版与 CLI 版算法逐字节一致。
const GO_VECTOR_EMPTY = 'ef46db3751d8e999'
const GO_VECTOR_HELLO = '26c7827d889f6da3'
const GO_VECTOR_PATTERN_1M = '44ec7540579dd3f0'
const GO_VECTOR_PATTERN_8M_123 = '9abb172070a8e990'

describe('xxhash64Hex（流式分块哈希）', () => {
  it('空输入得到 XXH64 空串标准值（与 Go 参考一致）', async () => {
    expect(await xxhash64Hex(new Blob([]))).toBe(GO_VECTOR_EMPTY)
  })

  it('短文本摘要与 Go cespare/xxhash 参考实现一致', async () => {
    const hello = new TextEncoder().encode('hello')
    expect(await xxhash64Hex(new Blob([hello]))).toBe(GO_VECTOR_HELLO)
  })

  it('跨多个分块（8 MiB + 123 字节）的流式结果与一次性 xxhash64 一致', async () => {
    const data = patternBytes(8 * 1024 * 1024 + 123)
    const blob = new Blob([data])
    expect(await xxhash64Hex(blob)).toBe(await xxhash64(data))
    expect(await xxhash64Hex(blob)).toBe(GO_VECTOR_PATTERN_8M_123)
  })

  it('1 MiB pattern 与 Go cespare/xxhash 参考实现一致', async () => {
    const data = patternBytes(1024 * 1024)
    expect(await xxhash64Hex(new Blob([data]))).toBe(GO_VECTOR_PATTERN_1M)
  })

  it('哈希器复用时状态正确重置：相同输入得到相同摘要', async () => {
    const hello = new TextEncoder().encode('hello')
    const first = await xxhash64Hex(new Blob([hello]))
    const second = await xxhash64Hex(new Blob([hello]))
    expect(first).toBe(second)
    expect(first).toBe(GO_VECTOR_HELLO)
  })

  it('不同输入产生不同摘要', async () => {
    const a = await xxhash64Hex(new Blob([new TextEncoder().encode('hello')]))
    const b = await xxhash64Hex(new Blob([new TextEncoder().encode('hellp')]))
    expect(a).not.toBe(b)
  })

  it('progress 回调累计字节数最终等于输入大小', async () => {
    const data = patternBytes(8 * 1024 * 1024 + 5)
    let last = 0
    const digest = await xxhash64Hex(new Blob([data]), (bytes) => {
      last = bytes
    })
    expect(digest).toBeDefined()
    expect(last).toBe(data.length)
  })
})
