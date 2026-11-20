/**
 * 移動規則，與伺服器 server/internal/game/world.go 的 MoveStep / NormalizeInput 一致。
 * 客戶端預測用同一套算法，所以正常情況下預測位置會與伺服器完全相同，不需要校正。
 */

export interface Vec {
  x: number
  y: number
}

export interface SimConfig {
  worldWidth: number
  worldHeight: number
  playerRadius: number
  /** 單位 / 秒 */
  playerSpeed: number
  /** 一筆輸入代表的秒數（= 1 / tickRate） */
  inputDt: number
}

/** 長度超過 1 才縮放成 1（斜向不會比較快）；NaN / Infinity 視為 0。 */
export function normalizeInput(x: number, y: number): Vec {
  if (!Number.isFinite(x) || !Number.isFinite(y)) return { x: 0, y: 0 }
  const len = Math.hypot(x, y)
  return len > 1 ? { x: x / len, y: y / len } : { x, y }
}

/** 一筆輸入的移動：沿方向走 speed × inputDt，再把整個身體夾回世界內。 */
export function moveStep(pos: Vec, dir: Vec, cfg: SimConfig): Vec {
  const step = cfg.playerSpeed * cfg.inputDt
  const r = cfg.playerRadius
  return {
    x: clamp(pos.x + dir.x * step, r, cfg.worldWidth - r),
    y: clamp(pos.y + dir.y * step, r, cfg.worldHeight - r),
  }
}

export function clamp(v: number, min: number, max: number): number {
  return Math.min(Math.max(v, min), max)
}

export function lerp(a: number, b: number, t: number): number {
  return a + (b - a) * t
}

export function lerpVec(a: Vec, b: Vec, t: number): Vec {
  return { x: lerp(a.x, b.x, t), y: lerp(a.y, b.y, t) }
}
