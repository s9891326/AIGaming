import { create } from '@bufbuild/protobuf'
import { describe, expect, it } from 'vitest'
import { PlayerStateSchema, WorldSnapshotSchema } from '../gen/game/v1/game_pb'
import { GameClient, type JoinInfo } from './GameClient'

const join: JoinInfo = {
  playerId: 1,
  worldWidth: 3000,
  worldHeight: 2000,
  tickRate: 20,
  playerRadius: 16,
  playerSpeed: 200,
  enemyRadius: 14,
  projectileRadius: 6,
  gemRadius: 7,
}

function snapshot(
  tick: number,
  self: { x: number; choosing?: boolean; speed?: number; seq?: number },
) {
  return create(WorldSnapshotSchema, {
    tick,
    players: [
      create(PlayerStateSchema, {
        id: 1,
        x: self.x,
        y: 1000,
        hp: 100,
        maxHp: 100,
        speed: self.speed ?? 200,
        choosing: self.choosing ?? false,
        lastInputSeq: self.seq ?? 0,
      }),
    ],
  })
}

function setup() {
  const sent: { seq: number; x: number }[] = []
  const g = new GameClient()
  g.attachSender((seq, x) => sent.push({ seq, x }))
  g.onJoin(join)
  g.onSnapshot(snapshot(1, { x: 1500 }), 0)
  g.setMove(1, 0)
  return { g, sent }
}

describe('GameClient 升級暫停', () => {
  it('平常每個 tick 送一筆輸入', () => {
    const { g, sent } = setup()
    g.update(0) // 從靜止起步立刻送一筆
    g.update(50)
    g.update(100)
    expect(sent.map((s) => s.seq)).toEqual([1, 2, 3])
  })

  it('選單開著時不送輸入，關掉後還要等伺服器確認選完', () => {
    const { g, sent } = setup()
    g.update(0)
    g.setOfferOpen(true)
    g.update(50)
    g.update(100)
    expect(sent).toHaveLength(1)

    // 玩家選完，但伺服器的快照仍說 choosing=true → 繼續暫停
    g.setOfferOpen(false)
    g.onSnapshot(snapshot(2, { x: 1510, choosing: true, seq: 1 }), 120)
    g.update(150)
    expect(sent).toHaveLength(1)

    // 伺服器確認選完 → 恢復
    g.onSnapshot(snapshot(3, { x: 1510, choosing: false, seq: 1 }), 170)
    g.update(200)
    g.update(250)
    expect(sent.length).toBeGreaterThan(1)
  })

  it('快照中的速度更新預測（選了疾行）', () => {
    const { g } = setup()
    g.setOfferOpen(true)
    g.onSnapshot(snapshot(2, { x: 1500, speed: 300 }), 50)
    g.setOfferOpen(false)
    g.update(100) // 起步
    const v = g.view(100 + 50) // 一個 tick 之後，平滑到底
    // 300 × 0.05 = 15
    expect(v.self!.pos.x).toBeCloseTo(1515)
  })
})
