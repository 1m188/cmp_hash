// 流式 xxHash64：对 Blob/File 这类可 slice 的数据源分块读取并增量更新，
// 避免将整个文件载入内存。与 CLI 版 hash.go 的 io.CopyBuffer 流式哈希对应。
//
// 哈希器实例在 worker 内复用（init() 会重置内部状态），避免每个文件
// 重复实例化 WASM。

import { createXXHash64, type IHasher } from 'hash-wasm'

/** 分块大小：8 MiB。与 CLI 的 1 MiB 缓冲同思路，取更大块以减少浏览器侧调用次数。 */
export const CHUNK_SIZE = 8 * 1024 * 1024

/** File / Blob 共有的最小接口：size + slice。 */
export type SliceableSource = {
  readonly size: number
  slice(start?: number, end?: number): Blob
}

let sharedHasher: Promise<IHasher> | undefined

function getSharedHasher(): Promise<IHasher> {
  sharedHasher ??= createXXHash64()
  return sharedHasher
}

/**
 * 用给定哈希器对流式数据源计算摘要（hex）。
 * onProgress 在每个块读取完毕后回调，参数为已读取的累计字节数。
 */
export async function hashWith(
  hasher: IHasher,
  source: SliceableSource,
  onProgress?: (bytesRead: number) => void,
): Promise<string> {
  hasher.init()
  let offset = 0
  while (offset < source.size) {
    const end = Math.min(offset + CHUNK_SIZE, source.size)
    const buf = await source.slice(offset, end).arrayBuffer()
    hasher.update(new Uint8Array(buf))
    offset = end
    onProgress?.(offset)
  }
  return hasher.digest()
}

/** 便捷入口：使用共享哈希器实例计算数据源的 XXH64 hex 摘要。 */
export async function xxhash64Hex(
  source: SliceableSource,
  onProgress?: (bytesRead: number) => void,
): Promise<string> {
  return hashWith(await getSharedHasher(), source, onProgress)
}
