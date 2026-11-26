/** 受傷閃爍持續時間（毫秒）。 */
export const HURT_FLASH_MS = 250

export interface Health {
  hp: number
  maxHp: number
  /** 最近一次血量下降的時間（performance.now()），沒受過傷為 -Infinity。 */
  hurtAt: number
}

/**
 * 從快照追蹤每個角色的血量：伺服器只送「目前血量」，前端比較前後兩次，
 * 血量下降就記下受傷時間，用來畫受傷閃爍。
 */
export class HealthTracker {
  private readonly byId = new Map<number, Health>()

  /** 記錄一筆快照中的血量。 */
  observe(id: number, hp: number, maxHp: number, now: number): void {
    const prev = this.byId.get(id)
    const hurtAt = prev && hp < prev.hp ? now : (prev?.hurtAt ?? -Infinity)
    this.byId.set(id, { hp, maxHp, hurtAt })
  }

  get(id: number): Health | undefined {
    return this.byId.get(id)
  }

  /** 受傷後 HURT_FLASH_MS 內回傳 true。 */
  isHurt(id: number, now: number): boolean {
    const h = this.byId.get(id)
    return !!h && now - h.hurtAt < HURT_FLASH_MS
  }

  /** 移除不在 keep 裡的角色（離開或死亡），避免 Map 無限成長。 */
  retain(keep: ReadonlySet<number>): void {
    for (const id of this.byId.keys()) if (!keep.has(id)) this.byId.delete(id)
  }

  clear(): void {
    this.byId.clear()
  }
}
