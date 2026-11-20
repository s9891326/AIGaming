import { clamp, lerpVec, type Vec } from './sim'

/**
 * 實體插值（entity interpolation）：
 * 其他玩家的位置不畫「最新快照」，而是畫「稍早一點的時間點」，並在前後兩個快照之間線性插值。
 * 這樣 20Hz 的快照在 60fps 下也是連續移動；代價是看到的其他人比實際晚約 100ms。
 */

export interface TimedSnapshot {
  /** 快照代表的伺服器時間（毫秒）。用 tick × tick 間隔，比伺服器實際送出時間更穩定。 */
  time: number
  entities: ReadonlyMap<number, Vec>
}

/**
 * 在 renderTime 取樣：
 * - 落在兩個快照之間 → 線性插值；只存在於較新快照的實體（剛加入）直接用新位置；
 *   只存在於較舊快照的實體（已離開）不顯示
 * - 早於第一個快照 → 用第一個；晚於最後一個 → 停在最後一個（不外插，寧可停頓也不要猜錯）
 */
export function sampleSnapshots(
  snapshots: readonly TimedSnapshot[],
  renderTime: number,
): Map<number, Vec> {
  const n = snapshots.length
  if (n === 0) return new Map()
  if (renderTime <= snapshots[0].time) return new Map(snapshots[0].entities)
  if (renderTime >= snapshots[n - 1].time) return new Map(snapshots[n - 1].entities)

  // 快照數量很少（約 20 個），線性搜尋即可；從尾端找，因為 renderTime 通常接近最新。
  let i = n - 2
  while (i > 0 && snapshots[i].time > renderTime) i--
  const a = snapshots[i]
  const b = snapshots[i + 1]
  const t = clamp((renderTime - a.time) / (b.time - a.time), 0, 1)

  const out = new Map<number, Vec>()
  for (const [id, to] of b.entities) {
    const from = a.entities.get(id)
    out.set(id, from ? lerpVec(from, to, t) : to)
  }
  return out
}

/** 依時間排序、只保留最近一段時間的快照緩衝。 */
export class SnapshotBuffer {
  private items: TimedSnapshot[] = []
  private readonly keepMs: number

  constructor(keepMs = 1000) {
    this.keepMs = keepMs
  }

  /** 加入快照；時間沒有比最新的更新（重複或亂序）時忽略。 */
  push(s: TimedSnapshot): void {
    const last = this.items[this.items.length - 1]
    if (last && s.time <= last.time) return
    this.items.push(s)
    // 丟掉太舊的，但至少留兩個才能插值。
    const cutoff = s.time - this.keepMs
    while (this.items.length > 2 && this.items[0].time < cutoff) this.items.shift()
  }

  sample(renderTime: number): Map<number, Vec> {
    return sampleSnapshots(this.items, renderTime)
  }

  get size(): number {
    return this.items.length
  }

  clear(): void {
    this.items = []
  }
}

/**
 * 估計「伺服器時間 − 本地時間」的差值。
 * 每個快照提供一個樣本：快照時間 − 收到時的本地時間；網路延遲只會讓樣本變小，
 * 所以取最近一段時間內的最大值，就是延遲最小（最準）的那一次。
 */
export class ServerClock {
  private samples: { offset: number; at: number }[] = []
  private readonly windowMs: number

  constructor(windowMs = 2000) {
    this.windowMs = windowMs
  }

  observe(serverTime: number, localNow: number): void {
    this.samples.push({ offset: serverTime - localNow, at: localNow })
    const cutoff = localNow - this.windowMs
    while (this.samples.length > 1 && this.samples[0].at < cutoff) this.samples.shift()
  }

  get ready(): boolean {
    return this.samples.length > 0
  }

  /** 本地時間換算成伺服器時間；尚無樣本時回傳 NaN。 */
  toServerTime(localNow: number): number {
    if (this.samples.length === 0) return NaN
    let best = -Infinity
    for (const s of this.samples) best = Math.max(best, s.offset)
    return localNow + best
  }

  reset(): void {
    this.samples = []
  }
}
