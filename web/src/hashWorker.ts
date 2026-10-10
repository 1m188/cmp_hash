// 哈希 worker：接收 {kind:'hash'|'pair'} 任务，流式读取 File 内容并计算 XXH64。
// File 对象经 structured clone 传入，文件内容始终留在用户设备上，不上传任何数据。

import { xxhash64Hex } from './hashStream'

type WorkerCtx = {
  addEventListener(type: 'message', listener: (ev: MessageEvent) => void): void
  postMessage(message: unknown): void
}

const ctx = self as unknown as WorkerCtx

type HashTask = { kind: 'hash'; id: number; file: File }
type PairTask = { kind: 'pair'; id: number; a: File; b: File }
type Task = HashTask | PairTask

ctx.addEventListener('message', (ev: MessageEvent) => {
  void handle(ev.data as Task)
})

let lastReport = 0

/** 进度节流：至少间隔 80ms 上报一次，避免海量 postMessage。 */
function report(id: number, bytes: number): void {
  const now = performance.now()
  if (now - lastReport < 80) return
  lastReport = now
  ctx.postMessage({ id, type: 'progress', bytes })
}

async function handle(task: Task): Promise<void> {
  try {
    if (task.kind === 'hash') {
      const digest = await xxhash64Hex(task.file, (bytes) => report(task.id, bytes))
      ctx.postMessage({ id: task.id, type: 'done', digest })
    } else {
      const sizeA = task.a.size
      const h1 = await xxhash64Hex(task.a, (bytes) => report(task.id, bytes))
      const h2 = await xxhash64Hex(task.b, (bytes) => report(task.id, sizeA + bytes))
      ctx.postMessage({ id: task.id, type: 'done', outcome: h1 === h2 ? 'same' : 'differ' })
    }
  } catch (err) {
    ctx.postMessage({
      id: task.id,
      type: 'error',
      message: err instanceof Error ? err.message : String(err),
    })
  }
}
