import { describe, expect, it } from 'vitest'
import { Predictor } from './prediction'
import { moveStep, type SimConfig, type Vec } from './sim'

const cfg: SimConfig = {
  worldWidth: 1000,
  worldHeight: 600,
  playerRadius: 10,
  playerSpeed: 100,
  inputDt: 0.05,
}

/** 模擬伺服器：依序套用收到的輸入（與 Predictor 同一套規則）。 */
function serverApply(start: Vec, inputs: { x: number; y: number }[]): Vec {
  return inputs.reduce((p, i) => moveStep(p, i, cfg), start)
}

describe('Predictor', () => {
  it('知道起始位置前不預測', () => {
    const p = new Predictor(cfg)
    expect(p.apply({ seq: 1, x: 1, y: 0 })).toBe(false)
    expect(p.pos).toBeNull()
  })

  it('輸入立即反映在本地位置（不等伺服器）', () => {
    const p = new Predictor(cfg)
    p.reconcile({ x: 500, y: 300 }, 0)
    p.apply({ seq: 1, x: 1, y: 0 })
    p.apply({ seq: 2, x: 1, y: 0 })
    expect(p.pos).toEqual({ x: 510, y: 300 })
    expect(p.pendingCount).toBe(2)
  })

  it('伺服器只處理了部分輸入時，從伺服器位置重播剩下的，結果與預測一致（校正量 0）', () => {
    const p = new Predictor(cfg)
    const start = { x: 500, y: 300 }
    p.reconcile(start, 0)
    const inputs = [
      { seq: 1, x: 1, y: 0 },
      { seq: 2, x: 0, y: 1 },
      { seq: 3, x: -1, y: 1 },
      { seq: 4, x: 1, y: 1 },
    ]
    for (const i of inputs) p.apply(i)
    const predicted = p.pos

    // 伺服器延遲：只處理到 seq 2
    const server = serverApply(start, inputs.slice(0, 2))
    const error = p.reconcile(server, 2)

    expect(error).toBeCloseTo(0)
    expect(p.pos!.x).toBeCloseTo(predicted!.x)
    expect(p.pos!.y).toBeCloseTo(predicted!.y)
    expect(p.pendingCount).toBe(2) // seq 3、4 仍待確認
  })

  it('全部確認後 pending 清空，位置等於伺服器', () => {
    const p = new Predictor(cfg)
    p.reconcile({ x: 500, y: 300 }, 0)
    p.apply({ seq: 1, x: 1, y: 0 })
    expect(p.reconcile({ x: 505, y: 300 }, 1)).toBeCloseTo(0)
    expect(p.pendingCount).toBe(0)
  })

  it('伺服器丟掉輸入（例如送太快）時，校正到伺服器結果並回報校正量', () => {
    const p = new Predictor(cfg)
    p.reconcile({ x: 500, y: 300 }, 0)
    p.apply({ seq: 1, x: 1, y: 0 })
    p.apply({ seq: 2, x: 1, y: 0 })
    // 伺服器丟掉 seq 1、只套用 seq 2：位置 505 而不是 510
    const error = p.reconcile({ x: 505, y: 300 }, 2)
    expect(error).toBeCloseTo(5)
    expect(p.pos).toEqual({ x: 505, y: 300 })
  })

  it('reset 清除狀態', () => {
    const p = new Predictor(cfg)
    p.reconcile({ x: 1, y: 1 }, 0)
    p.apply({ seq: 1, x: 1, y: 0 })
    p.reset()
    expect(p.pos).toBeNull()
    expect(p.pendingCount).toBe(0)
  })
})
