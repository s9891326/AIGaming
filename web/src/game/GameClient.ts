import type { WorldSnapshot } from '../gen/game/v1/game_pb'
import { HealthTracker } from './health'
import { ServerClock, SnapshotBuffer } from './interpolation'
import { Predictor } from './prediction'
import { lerpVec, type SimConfig, type Vec } from './sim'

/** 加入遊戲後伺服器告知的資訊。 */
export interface JoinInfo {
  playerId: number
  worldWidth: number
  worldHeight: number
  tickRate: number
  playerRadius: number
  playerSpeed: number
  enemyRadius: number
  projectileRadius: number
  gemRadius: number
}

/** 自己的成長數據（來自最新快照，不預測）。 */
export interface PlayerProgress {
  level: number
  xp: number
  xpToNext: number
  kills: number
}

/** 一個要畫在畫面上的角色。 */
export interface RenderPlayer {
  id: number
  pos: Vec
  hp: number
  maxHp: number
  /** 剛受傷（畫受傷閃爍）。 */
  hurt: boolean
}

export interface RenderView {
  join?: JoinInfo
  /** 本地玩家（位置是預測 + 平滑；血量來自最新快照）。 */
  self?: RenderPlayer
  /** 其他玩家（位置是插值）。 */
  others: RenderPlayer[]
  /** 敵人（位置是插值；hurt = 剛被打中）。 */
  enemies: { id: number; pos: Vec; hurt: boolean }[]
  /** 投射物與經驗寶石（位置是插值）。 */
  projectiles: { id: number; pos: Vec }[]
  gems: { id: number; pos: Vec }[]
  /** 自己的等級、經驗、擊殺數。 */
  progress?: PlayerProgress
  tick?: number
  pendingInputs: number
  /** 最近一次伺服器校正的距離（遊戲單位），0 代表預測完全正確。 */
  lastCorrection: number
  rttMs?: number
}

type SendInput = (seq: number, x: number, y: number) => void

/** 插值延遲：看其他玩家「100ms 前」的位置，約兩個快照間隔，可容忍一個快照晚到或遺失。 */
const INTERP_DELAY_MS = 100
/** 校正誤差的平滑時間常數：誤差每 ~80ms 縮小到 1/e，而不是瞬間跳過去。 */
const CORRECTION_SMOOTHING_MS = 80
/** 一幀最多補幾個輸入 tick（分頁在背景被暫停後回來時，避免一次送出大量輸入）。 */
const MAX_CATCHUP_STEPS = 5

/**
 * 客戶端遊戲狀態（與 React 無關）：
 * - 本地玩家：每個輸入 tick 產生一筆輸入，立即在本地預測並送給伺服器；快照回來時校正
 * - 其他玩家：快照放進緩衝，畫面時間延後 INTERP_DELAY_MS 插值
 *
 * 由 GameCanvas 每一幀呼叫 update(now) → view(now)。
 */
export class GameClient {
  private join?: JoinInfo
  private sim?: SimConfig
  private tickMs = 50
  private predictor?: Predictor
  private readonly buffer = new SnapshotBuffer()
  private readonly enemyBuffer = new SnapshotBuffer()
  private readonly projectileBuffer = new SnapshotBuffer()
  private readonly gemBuffer = new SnapshotBuffer()
  /** 敵人與玩家的 ID 是不同的編號空間，所以血量分開追蹤。 */
  private readonly enemyHealth = new HealthTracker()
  private progress?: PlayerProgress
  private readonly clock = new ServerClock()
  private readonly health = new HealthTracker()

  private move: Vec = { x: 0, y: 0 }
  private seq = 0
  private lastStepAt = 0
  private stepNow = false
  /** 上一個輸入 tick 時的預測位置，用來在兩個 tick 之間平滑繪製。 */
  private prevPos?: Vec
  /** 校正造成的瞬間位移，逐幀衰減到 0。 */
  private correction: Vec = { x: 0, y: 0 }
  private lastViewAt = 0
  private lastTick?: number
  private lastCorrection = 0

  private rttMs?: number
  /** 送出輸入的方式，由連線層在連線建立時接上；沒有連線時輸入只在本地預測、不送出。 */
  private sendInput?: SendInput

  /** 接上送出輸入的函式，回傳拔除用的函式。 */
  attachSender(send: SendInput): () => void {
    this.sendInput = send
    return () => {
      if (this.sendInput === send) this.sendInput = undefined
    }
  }

  /** 最新的往返延遲（顯示在 HUD）。 */
  setRtt(ms: number | undefined): void {
    this.rttMs = ms
  }

  get joined(): JoinInfo | undefined {
    return this.join
  }

  onJoin(info: JoinInfo): void {
    this.reset()
    this.join = info
    this.tickMs = 1000 / info.tickRate
    this.sim = {
      worldWidth: info.worldWidth,
      worldHeight: info.worldHeight,
      playerRadius: info.playerRadius,
      playerSpeed: info.playerSpeed,
      inputDt: 1 / info.tickRate,
    }
    this.predictor = new Predictor(this.sim)
  }

  onSnapshot(snap: WorldSnapshot, now: number): void {
    if (!this.join || !this.predictor) return
    const time = snap.tick * this.tickMs
    this.clock.observe(time, now)
    this.lastTick = snap.tick

    const others = new Map<number, Vec>()
    let self: { x: number; y: number; lastInputSeq: number } | undefined
    for (const p of snap.players) {
      this.health.observe(p.id, p.hp, p.maxHp, now)
      if (p.id === this.join.playerId) {
        self = p
        this.progress = { level: p.level, xp: p.xp, xpToNext: p.xpToNext, kills: p.kills }
      } else {
        others.set(p.id, { x: p.x, y: p.y })
      }
    }
    for (const e of snap.enemies) this.enemyHealth.observe(e.id, e.hp, e.maxHp, now)
    this.enemyHealth.retain(new Set(snap.enemies.map((e) => e.id)))
    const positions = (list: readonly { id: number; x: number; y: number }[]) =>
      new Map(list.map((o) => [o.id, { x: o.x, y: o.y }]))
    this.projectileBuffer.push({ time, entities: positions(snap.projectiles) })
    this.gemBuffer.push({ time, entities: positions(snap.gems) })
    this.health.retain(new Set(snap.players.map((p) => p.id)))
    this.buffer.push({ time, entities: others })
    this.enemyBuffer.push({
      time,
      entities: new Map(snap.enemies.map((e) => [e.id, { x: e.x, y: e.y }])),
    })

    if (!self) return
    const before = this.predictor.pos
    this.lastCorrection = this.predictor.reconcile(self, self.lastInputSeq)
    const after = this.predictor.pos!
    if (!before) {
      this.prevPos = after
    } else if (this.lastCorrection > 0.01) {
      // 位置被校正：把跳動量記下來慢慢消掉，畫面上看起來是滑過去而不是瞬移。
      const dx = before.x - after.x
      const dy = before.y - after.y
      this.correction = { x: this.correction.x + dx, y: this.correction.y + dy }
      if (this.prevPos) this.prevPos = { x: this.prevPos.x - dx, y: this.prevPos.y - dy }
    }
  }

  /** 設定目前想移動的方向（鍵盤）。從靜止開始移動時，下一幀立刻產生輸入，不等下一個 tick。 */
  setMove(x: number, y: number): void {
    const wasIdle = this.move.x === 0 && this.move.y === 0
    this.move = { x, y }
    if (wasIdle && (x !== 0 || y !== 0)) this.stepNow = true
  }

  /** 推進輸入 tick（每幀呼叫）：累積經過的時間，每滿一個 tick 產生一筆輸入。 */
  update(now: number): void {
    if (!this.predictor?.pos) {
      this.lastStepAt = now
      return
    }
    if (this.stepNow) {
      this.stepNow = false
      this.lastStepAt = now - this.tickMs
    }
    let steps = 0
    while (now - this.lastStepAt >= this.tickMs && steps < MAX_CATCHUP_STEPS) {
      this.lastStepAt += this.tickMs
      this.step()
      steps++
    }
    if (now - this.lastStepAt >= this.tickMs) this.lastStepAt = now // 落後太多就放棄追趕
  }

  private step(): void {
    const predictor = this.predictor!
    this.prevPos = predictor.pos ?? undefined
    if (this.move.x === 0 && this.move.y === 0) return // 不動時不送輸入
    this.seq += 1
    predictor.apply({ seq: this.seq, x: this.move.x, y: this.move.y })
    this.sendInput?.(this.seq, this.move.x, this.move.y)
  }

  view(now: number): RenderView {
    const frameDt = this.lastViewAt ? now - this.lastViewAt : 0
    this.lastViewAt = now
    const decay = Math.exp(-frameDt / CORRECTION_SMOOTHING_MS)
    this.correction = { x: this.correction.x * decay, y: this.correction.y * decay }

    const player = (id: number, pos: Vec): RenderPlayer => {
      const h = this.health.get(id)
      return { id, pos, hp: h?.hp ?? 0, maxHp: h?.maxHp ?? 0, hurt: this.health.isHurt(id, now) }
    }

    let self: RenderPlayer | undefined
    const pos = this.predictor?.pos
    if (pos && this.join) {
      // 預測位置每個 tick 才變一次；在上一個與目前位置之間依時間比例平滑。
      const alpha = Math.min(1, Math.max(0, (now - this.lastStepAt) / this.tickMs))
      const smooth = this.prevPos ? lerpVec(this.prevPos, pos, alpha) : pos
      self = player(this.join.playerId, {
        x: smooth.x + this.correction.x,
        y: smooth.y + this.correction.y,
      })
    }

    const others: RenderPlayer[] = []
    const enemies: RenderView['enemies'] = []
    const projectiles: RenderView['projectiles'] = []
    const gems: RenderView['gems'] = []
    if (this.clock.ready) {
      const renderTime = this.clock.toServerTime(now) - INTERP_DELAY_MS
      for (const [id, p] of this.buffer.sample(renderTime)) others.push(player(id, p))
      for (const [id, p] of this.enemyBuffer.sample(renderTime)) {
        enemies.push({ id, pos: p, hurt: this.enemyHealth.isHurt(id, now) })
      }
      for (const [id, p] of this.projectileBuffer.sample(renderTime))
        projectiles.push({ id, pos: p })
      for (const [id, p] of this.gemBuffer.sample(renderTime)) gems.push({ id, pos: p })
    }

    return {
      join: this.join,
      self,
      others,
      enemies,
      projectiles,
      gems,
      progress: this.join ? this.progress : undefined,
      tick: this.lastTick,
      pendingInputs: this.predictor?.pendingCount ?? 0,
      lastCorrection: this.lastCorrection,
      rttMs: this.rttMs,
    }
  }

  /** 斷線或重新加入時清除所有狀態（新的角色序號從 1 開始）。 */
  reset(): void {
    this.join = undefined
    this.sim = undefined
    this.predictor = undefined
    this.buffer.clear()
    this.enemyBuffer.clear()
    this.projectileBuffer.clear()
    this.gemBuffer.clear()
    this.enemyHealth.clear()
    this.progress = undefined
    this.clock.reset()
    this.health.clear()
    this.seq = 0
    this.prevPos = undefined
    this.correction = { x: 0, y: 0 }
    this.lastTick = undefined
    this.lastCorrection = 0
  }
}
