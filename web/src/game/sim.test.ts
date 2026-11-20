import { describe, expect, it } from 'vitest'
import { moveStep, normalizeInput, type SimConfig } from './sim'

const cfg: SimConfig = {
  worldWidth: 1000,
  worldHeight: 600,
  playerRadius: 10,
  playerSpeed: 100,
  inputDt: 0.05,
}

describe('normalizeInput（與伺服器 NormalizeInput 一致）', () => {
  it.each([
    ['零向量', 0, 0, 0, 0],
    ['正右', 1, 0, 1, 0],
    ['斜向縮成單位長度', 1, 1, Math.SQRT1_2, Math.SQRT1_2],
    ['過長縮成 1', 10, 0, 1, 0],
    ['類比半推保留長度', 0.5, 0, 0.5, 0],
    ['NaN 視為 0', NaN, 1, 0, 0],
    ['Infinity 視為 0', 1, -Infinity, 0, 0],
  ])('%s', (_, x, y, wx, wy) => {
    const v = normalizeInput(x, y)
    expect(v.x).toBeCloseTo(wx)
    expect(v.y).toBeCloseTo(wy)
  })
})

describe('moveStep（與伺服器 MoveStep 一致）', () => {
  it('每筆輸入移動 speed × inputDt', () => {
    expect(moveStep({ x: 500, y: 300 }, { x: 1, y: 0 }, cfg)).toEqual({ x: 505, y: 300 })
  })

  it('夾在世界內（扣掉半徑）', () => {
    expect(moveStep({ x: 12, y: 598 }, { x: -1, y: 1 }, cfg)).toEqual({ x: 10, y: 590 })
  })
})
