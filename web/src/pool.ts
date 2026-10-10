// 主线程侧的 Web Worker 池：把单文件哈希 / 文件对比较任务分发给 N 个 worker，
// 并转发分块读取进度。对应 CLI 版 compareDirsAll 的 worker pool
// （worker 数 = 并发度，任务内先哈希 A 再哈希 B）。

export type PairOutcome = 'same' | 'differ'

type HashTask = { kind: 'hash'; id: number; file: File }
type PairTask = { kind: 'pair'; id: number; a: File; b: File }
type Task = HashTask | PairTask
/** 提交任务时的形状（id 由池内部分配）。 */
type TaskInput = Omit<HashTask, 'id'> | Omit<PairTask, 'id'>

export type WorkerReply =
  | { id: number; type: 'progress'; bytes: number }
  | { id: number; type: 'done'; outcome?: PairOutcome; digest?: string }
  | { id: number; type: 'error'; message: string }

type DoneReply = Extract<WorkerReply, { type: 'done' }>

interface PendingEntry {
  resolve: (value: DoneReply) => void
  reject: (err: Error) => void
  onProgress?: (bytes: number) => void
}

export class WorkerPool {
  private readonly all: Worker[] = []
  private idle: Worker[] = []
  private readonly queue: Task[] = []
  private readonly pending = new Map<number, PendingEntry>()
  private nextId = 1
  private dead = false

  constructor(size: number) {
    for (let i = 0; i < Math.max(1, size); i++) {
      const w = new Worker(new URL('./hashWorker.ts', import.meta.url), { type: 'module' })
      w.addEventListener('message', (ev: MessageEvent<WorkerReply>) => this.onMessage(w, ev.data))
      w.addEventListener('error', (ev) => {
        ev.preventDefault()
        this.failPending(new Error(ev.message || 'worker 崩溃'))
      })
      this.all.push(w)
      this.idle.push(w)
    }
  }

  /** 计算单个文件的 XXH64 hex 摘要。 */
  hashFile(file: File, onProgress?: (bytes: number) => void): Promise<string> {
    return this.submit({ kind: 'hash', file }, onProgress).then((r) => r.digest as string)
  }

  /** 比较一对文件内容：'same' / 'differ'。 */
  comparePair(a: File, b: File, onProgress?: (bytes: number) => void): Promise<PairOutcome> {
    return this.submit({ kind: 'pair', a, b }, onProgress).then((r) => r.outcome as PairOutcome)
  }

  terminate(): void {
    this.dead = true
    this.queue.length = 0
    for (const w of this.all) w.terminate()
    this.idle = []
    this.failPending(new Error('worker pool 已终止'))
  }

  private submit(
    task: TaskInput,
    onProgress?: (bytes: number) => void,
  ): Promise<DoneReply> {
    if (this.dead) return Promise.reject(new Error('worker pool 已终止'))
    return new Promise<DoneReply>((resolve, reject) => {
      const id = this.nextId++
      this.pending.set(id, { resolve, reject, onProgress })
      this.queue.push({ ...task, id } as Task)
      this.dispatch()
    })
  }

  private dispatch(): void {
    while (!this.dead && this.queue.length > 0 && this.idle.length > 0) {
      const task = this.queue.shift() as Task
      const w = this.idle.pop() as Worker
      w.postMessage(task)
    }
  }

  private onMessage(w: Worker, msg: WorkerReply): void {
    const entry = this.pending.get(msg.id)
    if (!entry) {
      // 未知回复：不终止 worker，保持池可用。
      this.idle.push(w)
      return
    }
    if (msg.type === 'progress') {
      entry.onProgress?.(msg.bytes)
      return
    }
    this.pending.delete(msg.id)
    this.idle.push(w)
    if (msg.type === 'error') entry.reject(new Error(msg.message))
    else entry.resolve(msg)
    this.dispatch()
  }

  private failPending(err: Error): void {
    for (const entry of this.pending.values()) entry.reject(err)
    this.pending.clear()
  }
}
