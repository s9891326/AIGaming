import { moveStep, normalizeInput, type SimConfig, type Vec } from './sim'

/**
 * 客戶端預測（client-side prediction）與伺服器校正（server reconciliation）：
 *
 * 1. 按下方向鍵時不等伺服器，立刻在本地套用這筆輸入（apply），並記在 pending 清單。
 * 2. 快照回來時，伺服器告訴我們「處理到第幾筆輸入（lastSeq）時的位置」。
 * 3. 從伺服器位置開始，重播所有序號更大、伺服器還沒處理的輸入，得到新的預測位置（reconcile）。
 *
 * 因為移動規則與伺服器完全相同，正常情況下重播結果與原本預測一致（校正量 0）；
 * 只有伺服器丟棄輸入（例如送太快）或被其他東西擋住時才會產生校正。
 */

export interface PendingInput {
  seq: number
  x: number
  y: number
}

export class Predictor {
  /** 預測的本地玩家位置；收到第一個含自己的快照前為 null。 */
  pos: Vec | null = null
  private pending: PendingInput[] = []
  private readonly cfg: SimConfig

  constructor(cfg: SimConfig) {
    this.cfg = cfg
  }

  get pendingCount(): number {
    return this.pending.length
  }

  /** 在本地立即套用一筆輸入。尚未知道起始位置時忽略（不會送出也不會記錄）。 */
  apply(input: PendingInput): boolean {
    if (!this.pos) return false
    this.pending.push(input)
    this.pos = moveStep(this.pos, normalizeInput(input.x, input.y), this.cfg)
    return true
  }

  /**
   * 以伺服器權威狀態校正，回傳校正前後的距離（0 = 預測完全正確）。
   * @param serverPos 伺服器處理完 lastSeq 之後的位置
   * @param lastSeq 伺服器最後處理的輸入序號
   */
  reconcile(serverPos: Vec, lastSeq: number): number {
    this.pending = this.pending.filter((p) => p.seq > lastSeq)
    let p: Vec = { x: serverPos.x, y: serverPos.y }
    for (const input of this.pending) {
      p = moveStep(p, normalizeInput(input.x, input.y), this.cfg)
    }
    const error = this.pos ? Math.hypot(this.pos.x - p.x, this.pos.y - p.y) : 0
    this.pos = p
    return error
  }

  reset(): void {
    this.pos = null
    this.pending = []
  }
}
