import { describe, expect, it } from 'vitest'
import { SnapshotBuffer, ServerClock, sampleSnapshots, type TimedSnapshot } from './interpolation'

const snap = (time: number, entities: Record<number, [number, number]>): TimedSnapshot => ({
  time,
  entities: new Map(Object.entries(entities).map(([id, [x, y]]) => [Number(id), { x, y }])),
})

describe('sampleSnapshots', () => {
  const s = [snap(0, { 1: [0, 0] }), snap(50, { 1: [10, 20] }), snap(100, { 1: [30, 20] })]

  it('沒有快照時回傳空', () => {
    expect(sampleSnapshots([], 10).size).toBe(0)
  })

  it('在兩個快照之間線性插值', () => {
    expect(sampleSnapshots(s, 25).get(1)).toEqual({ x: 5, y: 10 })
    expect(sampleSnapshots(s, 75).get(1)).toEqual({ x: 20, y: 20 })
  })

  it('剛好落在快照時間時等於該快照', () => {
    expect(sampleSnapshots(s, 50).get(1)).toEqual({ x: 10, y: 20 })
  })

  it('早於第一個快照時用第一個', () => {
    expect(sampleSnapshots(s, -100).get(1)).toEqual({ x: 0, y: 0 })
  })

  it('晚於最後一個快照時停在最後一個（不外插）', () => {
    expect(sampleSnapshots(s, 500).get(1)).toEqual({ x: 30, y: 20 })
  })

  it('剛加入的實體直接用新位置；已離開的實體不顯示', () => {
    const joinLeave = [snap(0, { 1: [0, 0], 2: [100, 100] }), snap(50, { 1: [10, 0], 3: [7, 7] })]
    const got = sampleSnapshots(joinLeave, 25)
    expect(got.get(1)).toEqual({ x: 5, y: 0 })
    expect(got.get(3)).toEqual({ x: 7, y: 7 })
    expect(got.has(2)).toBe(false)
  })
})

describe('SnapshotBuffer', () => {
  it('忽略重複或亂序的快照', () => {
    const b = new SnapshotBuffer()
    b.push(snap(50, { 1: [1, 1] }))
    b.push(snap(50, { 1: [9, 9] }))
    b.push(snap(0, { 1: [9, 9] }))
    expect(b.size).toBe(1)
    expect(b.sample(50).get(1)).toEqual({ x: 1, y: 1 })
  })

  it('只保留最近一段時間，但至少兩個', () => {
    const b = new SnapshotBuffer(100)
    for (let t = 0; t <= 1000; t += 50) b.push(snap(t, { 1: [t, 0] }))
    expect(b.size).toBe(3) // 900, 950, 1000
    const small = new SnapshotBuffer(10)
    small.push(snap(0, {}))
    small.push(snap(1000, {}))
    expect(small.size).toBe(2)
  })
})

describe('ServerClock', () => {
  it('取延遲最小（offset 最大）的樣本', () => {
    const c = new ServerClock()
    expect(Number.isNaN(c.toServerTime(0))).toBe(true)
    c.observe(1000, 100) // 延遲較大：offset 900
    c.observe(1050, 140) // 延遲較小：offset 910
    c.observe(1100, 230) // 延遲較大：offset 870
    expect(c.toServerTime(300)).toBe(1210)
  })

  it('只看最近 windowMs 內的樣本（時鐘漂移後會更新）', () => {
    const c = new ServerClock(1000)
    c.observe(1000, 0) // offset 1000，之後過期
    c.observe(1500, 1600) // offset -100
    expect(c.toServerTime(2000)).toBe(1900)
  })
})
