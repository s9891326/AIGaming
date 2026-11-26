import { describe, expect, it } from 'vitest'
import { HURT_FLASH_MS, HealthTracker } from './health'

describe('HealthTracker', () => {
  it('第一次看到的角色不算受傷', () => {
    const h = new HealthTracker()
    h.observe(1, 100, 100, 1000)
    expect(h.get(1)).toMatchObject({ hp: 100, maxHp: 100 })
    expect(h.isHurt(1, 1000)).toBe(false)
  })

  it('血量下降後閃爍一段時間', () => {
    const h = new HealthTracker()
    h.observe(1, 100, 100, 1000)
    h.observe(1, 90, 100, 2000)
    expect(h.isHurt(1, 2000)).toBe(true)
    expect(h.isHurt(1, 2000 + HURT_FLASH_MS - 1)).toBe(true)
    expect(h.isHurt(1, 2000 + HURT_FLASH_MS)).toBe(false)
  })

  it('血量不變或上升不會重設受傷時間', () => {
    const h = new HealthTracker()
    h.observe(1, 100, 100, 0)
    h.observe(1, 90, 100, 1000)
    h.observe(1, 90, 100, 1050) // 下一個快照，血量沒變
    expect(h.get(1)!.hurtAt).toBe(1000)
    h.observe(1, 100, 100, 1100) // 回血
    expect(h.get(1)!.hurtAt).toBe(1000)
  })

  it('retain 移除已離開的角色', () => {
    const h = new HealthTracker()
    h.observe(1, 100, 100, 0)
    h.observe(2, 100, 100, 0)
    h.retain(new Set([2]))
    expect(h.get(1)).toBeUndefined()
    expect(h.get(2)).toBeDefined()
  })
})
