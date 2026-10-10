// cmp_hash web 版入口：页面交互、模式判断、worker 池调度与结果渲染。
//
// 两种模式与 CLI 对应：
//   文件模式：先比大小，再比 XXH64（CompareFiles）
//   目录模式：结构集合差 → 大小快速路径 → 哈希（--diff-all 全量语义，CompareDirs）
//
// 目录相对路径来自 webkitRelativePath（去掉所选根目录名），
// 空目录两侧均不参与比较，与 CLI「仅比较普通文件」一致。

import './style.css'
import { compareTrees, defaultConcurrency, type Entry, type TreeDiff } from './compare'
import { WorkerPool, type PairOutcome } from './pool'

type SlotKind = 'file' | 'dir'

interface SlotState {
  kind: SlotKind
  name: string
  file?: File
  entries?: Entry[]
  fileMap?: Map<string, File>
  count: number
  totalSize: number
}

const $id = (id: string): HTMLElement => {
  const el = document.getElementById(id)
  if (!el) throw new Error(`missing element #${id}`)
  return el
}

const MAX_LIST_ITEMS = 1000

const compareBtn = $id('compare-btn') as HTMLButtonElement
const resetBtn = $id('reset-btn') as HTMLButtonElement
const errorMsg = $id('error-msg')
const progress = $id('progress')
const progressFill = $id('progress-fill')
const progressText = $id('progress-text')
const result = $id('result')
const verdict = $id('verdict')
const verdictSub = $id('verdict-sub')
const summary = $id('summary')
const digestRow = $id('digest-row')
const digestA = $id('digest-a')
const digestB = $id('digest-b')

const sections = {
  onlyIn1: $id('sec-only-in-1'),
  onlyIn2: $id('sec-only-in-2'),
  differ: $id('sec-differ'),
  errors: $id('sec-errors'),
}
const counts = {
  onlyIn1: $id('count-only-in-1'),
  onlyIn2: $id('count-only-in-2'),
  differ: $id('count-differ'),
  errors: $id('count-errors'),
}
const lists = {
  onlyIn1: $id('list-only-in-1') as HTMLUListElement,
  onlyIn2: $id('list-only-in-2') as HTMLUListElement,
  differ: $id('list-differ') as HTMLUListElement,
  errors: $id('list-errors') as HTMLUListElement,
}

const slots: { a: SlotState | null; b: SlotState | null } = { a: null, b: null }
let pool: WorkerPool | null = null

function formatBytes(n: number): string {
  if (n < 1024) return `${n} B`
  const units = ['KiB', 'MiB', 'GiB', 'TiB'] as const
  let v = n
  let i = -1
  do {
    v /= 1024
    i++
  } while (v >= 1024 && i < units.length - 1)
  return `${v.toFixed(v >= 100 ? 0 : 1)} ${units[i]}`
}

/** webkitRelativePath 形如 rootName/sub/file.txt，去掉所选根目录名得到相对路径。 */
function stripRoot(webkitRelativePath: string): string {
  const idx = webkitRelativePath.indexOf('/')
  return idx === -1 ? webkitRelativePath : webkitRelativePath.slice(idx + 1)
}

function slotInfoText(s: SlotState): string {
  if (s.kind === 'file') return `文件：${s.name}（${formatBytes(s.totalSize)}）`
  return `目录：${s.name}（${s.count} 个文件，${formatBytes(s.totalSize)}）`
}

function refreshControls(): void {
  compareBtn.disabled = !(slots.a && slots.b)
}

function wireSlot(side: 'a' | 'b'): void {
  const fileInput = $id(`input-file-${side}`) as HTMLInputElement
  const dirInput = $id(`input-dir-${side}`) as HTMLInputElement
  const info = $id(`info-${side}`)

  $id(`pick-file-${side}`).addEventListener('click', () => fileInput.click())
  $id(`pick-dir-${side}`).addEventListener('click', () => dirInput.click())

  fileInput.addEventListener('change', () => {
    const f = fileInput.files?.[0]
    if (!f) return
    const state: SlotState = { kind: 'file', name: f.name, file: f, count: 1, totalSize: f.size }
    slots[side] = state
    info.textContent = slotInfoText(state)
    info.classList.add('picked')
    fileInput.value = ''
    refreshControls()
  })

  dirInput.addEventListener('change', () => {
    const files = Array.from(dirInput.files ?? [])
    if (files.length === 0) return
    const first = files[0].webkitRelativePath || files[0].name
    const name = first.includes('/') ? first.slice(0, first.indexOf('/')) : first
    const entries: Entry[] = []
    const fileMap = new Map<string, File>()
    let totalSize = 0
    for (const f of files) {
      const rel = stripRoot(f.webkitRelativePath || f.name)
      entries.push({ rel, size: f.size })
      fileMap.set(rel, f)
      totalSize += f.size
    }
    const state: SlotState = { kind: 'dir', name, entries, fileMap, count: files.length, totalSize }
    slots[side] = state
    info.textContent = slotInfoText(state)
    info.classList.add('picked')
    dirInput.value = ''
    refreshControls()
  })
}

function showError(message: string): void {
  errorMsg.textContent = message
  errorMsg.hidden = false
}

function hideError(): void {
  errorMsg.hidden = true
  errorMsg.textContent = ''
}

function setProgress(frac: number, text: string): void {
  progress.hidden = false
  const pct = Math.min(100, Math.max(0, frac * 100))
  progressFill.style.width = `${pct.toFixed(1)}%`
  progressText.textContent = text
}

function setVerdict(kind: 'ok' | 'bad' | 'warn', text: string, sub: string): void {
  verdict.textContent = text
  verdict.className = `verdict ${kind}`
  verdictSub.textContent = sub
}

function fillSection(sec: HTMLElement, count: HTMLElement, ul: HTMLUListElement, items: readonly string[]): void {
  if (items.length === 0) {
    sec.hidden = true
    ul.innerHTML = ''
    return
  }
  sec.hidden = false
  count.textContent = `${items.length} 项`
  ul.innerHTML = ''
  const shown = items.length > MAX_LIST_ITEMS ? items.slice(0, MAX_LIST_ITEMS) : items
  for (const item of shown) {
    const li = document.createElement('li')
    li.textContent = item
    ul.appendChild(li)
  }
  if (items.length > MAX_LIST_ITEMS) {
    const li = document.createElement('li')
    li.className = 'muted'
    li.textContent = `…还有 ${items.length - MAX_LIST_ITEMS} 项未显示`
    ul.appendChild(li)
  }
}

function resetResult(): void {
  result.hidden = true
  digestRow.hidden = true
  for (const sec of Object.values(sections)) sec.hidden = true
  for (const ul of Object.values(lists)) ul.innerHTML = ''
}

function renderFileVerdict(same: boolean, a: SlotState, b: SlotState, h1?: string, h2?: string): void {
  result.hidden = false
  const sizeDiffer = h1 === undefined
  if (same) setVerdict('ok', '✅ 相同', '两个文件内容一致')
  else setVerdict('bad', '❌ 不同', sizeDiffer ? '文件大小不同' : '文件内容不同')
  const fa = a.file as File
  const fb = b.file as File
  summary.textContent = `${a.name}（${formatBytes(fa.size)}） ↔ ${b.name}（${formatBytes(fb.size)}）`
  if (sizeDiffer) {
    digestRow.hidden = true
  } else {
    digestRow.hidden = false
    digestA.textContent = h1 ?? ''
    digestB.textContent = h2 ?? ''
  }
  for (const sec of Object.values(sections)) sec.hidden = true
}

function renderDirVerdict(diff: TreeDiff, a: SlotState, b: SlotState): void {
  result.hidden = false
  const hasErrors = diff.readErrors.length > 0
  if (hasErrors) setVerdict('warn', '⚠️ 无法完全确定', '部分文件读取失败，以下结果可能不完整')
  else if (diff.same) setVerdict('ok', '✅ 相同', '两个目录内容一致')
  else setVerdict('bad', '❌ 不同', '两个目录存在差异')

  summary.textContent =
    `${a.name}（${a.count} 个文件） ↔ ${b.name}（${b.count} 个文件）` +
    ` · 仅在 A：${diff.onlyIn1.length}` +
    ` · 仅在 B：${diff.onlyIn2.length}` +
    ` · 内容不同：${diff.differ.length}` +
    (hasErrors ? ` · 读取失败：${diff.readErrors.length}` : '')

  fillSection(sections.onlyIn1, counts.onlyIn1, lists.onlyIn1, diff.onlyIn1)
  fillSection(sections.onlyIn2, counts.onlyIn2, lists.onlyIn2, diff.onlyIn2)
  fillSection(sections.differ, counts.differ, lists.differ, diff.differ)
  fillSection(
    sections.errors,
    counts.errors,
    lists.errors,
    diff.readErrors.map((e) => `${e.rel} — ${e.message}`),
  )
  digestRow.hidden = true
}

async function compareFiles(a: SlotState, b: SlotState): Promise<void> {
  const fa = a.file as File
  const fb = b.file as File
  // 大小快速路径：大小不同直接判差，不做哈希。
  if (fa.size !== fb.size) {
    renderFileVerdict(false, a, b)
    return
  }
  const p = new WorkerPool(defaultConcurrency())
  pool = p
  const total = fa.size + fb.size
  const read = [0, 0]
  const update = (): void => setProgress((read[0] + read[1]) / total, '哈希中…')
  const [h1, h2] = await Promise.all([
    p.hashFile(fa, (n) => {
      read[0] = n
      update()
    }),
    p.hashFile(fb, (n) => {
      read[1] = n
      update()
    }),
  ])
  setProgress(1, '完成')
  renderFileVerdict(h1 === h2, a, b, h1, h2)
}

async function compareDirs(a: SlotState, b: SlotState): Promise<void> {
  const entries1 = a.entries as Entry[]
  const entries2 = b.entries as Entry[]
  const map2 = new Map(entries2.map((e) => [e.rel, e]))

  // 进度总量：仅统计需要哈希（同路径且同大小）的文件对字节数。
  let totalBytes = 0
  let pairCount = 0
  for (const e1 of entries1) {
    const e2 = map2.get(e1.rel)
    if (e2 && e1.size === e2.size) {
      totalBytes += e1.size + e2.size
      pairCount++
    }
  }

  const p = new WorkerPool(defaultConcurrency())
  pool = p

  const read = new Map<string, number>()
  let donePairs = 0
  const update = (): void => {
    let bytes = 0
    for (const v of read.values()) bytes += v
    const frac = totalBytes > 0 ? bytes / totalBytes : 1
    setProgress(frac, `哈希中… ${donePairs}/${pairCount} 个文件对`)
  }

  const comparePair = (rel: string): Promise<PairOutcome> => {
    const fa = (a.fileMap as Map<string, File>).get(rel) as File
    const fb = (b.fileMap as Map<string, File>).get(rel) as File
    return p
      .comparePair(fa, fb, (n) => {
        read.set(rel, n)
        update()
      })
      .finally(() => {
        donePairs++
        update()
      })
  }

  const diff = await compareTrees(entries1, entries2, comparePair, defaultConcurrency())
  setProgress(1, '完成')
  renderDirVerdict(diff, a, b)
}

async function runCompare(): Promise<void> {
  const a = slots.a
  const b = slots.b
  if (!a || !b) return
  hideError()
  resetResult()
  compareBtn.disabled = true
  resetBtn.disabled = true
  setProgress(0, '准备中…')
  try {
    if (a.kind !== b.kind) {
      showError('两侧类型不一致：一个是文件、一个是目录。请选择同为文件或同为目录。')
      return
    }
    if (a.kind === 'file') await compareFiles(a, b)
    else await compareDirs(a, b)
  } catch (err) {
    showError(err instanceof Error ? err.message : String(err))
  } finally {
    pool?.terminate()
    pool = null
    compareBtn.disabled = false
    resetBtn.disabled = false
  }
}

function reset(): void {
  slots.a = null
  slots.b = null
  for (const side of ['a', 'b'] as const) {
    const info = $id(`info-${side}`)
    info.textContent = '未选择'
    info.classList.remove('picked')
  }
  hideError()
  resetResult()
  progress.hidden = true
  refreshControls()
}

wireSlot('a')
wireSlot('b')
compareBtn.addEventListener('click', () => void runCompare())
resetBtn.addEventListener('click', reset)
refreshControls()
