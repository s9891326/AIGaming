import { VIEW_H, VIEW_W, drawFrame } from './draw'
import type { JoinInfo, RenderView } from './GameClient'

/**
 * 繪製效能量測（開發用）：用合成的場景在離屏 canvas 上連續畫 frames 幀，回傳每幀耗時。
 * 不依賴 requestAnimationFrame，瀏覽器分頁在背景也能量。
 *
 * 在開發模式的 DevTools console：
 *   const { runDrawBench } = await import('/src/game/drawBench.ts')
 *   runDrawBench({ enemies: 1000 })
 */
export interface DrawBenchOptions {
  /** 要量的繪製函式（預設 drawFrame），比較不同實作用。 */
  draw?: typeof drawFrame
  enemies?: number
  gems?: number
  projectiles?: number
  frames?: number
}

export interface DrawBenchResult {
  enemies: number
  frames: number
  /** 每幀平均耗時（毫秒）。60fps 的預算是 16.7ms。 */
  avgMs: number
}

export function runDrawBench(opts: DrawBenchOptions = {}): DrawBenchResult {
  const { draw = drawFrame, enemies = 1000, gems = 150, projectiles = 80, frames = 120 } = opts
  const canvas = document.createElement('canvas')
  canvas.width = VIEW_W
  canvas.height = VIEW_H
  const ctx = canvas.getContext('2d')!
  const view = syntheticView(enemies, gems, projectiles)

  // 連續畫完所有幀，最後才讀回一個像素，強迫繪圖管線把排隊的工作做完，再取平均。
  // 不要每幀都讀回：Chrome 偵測到頻繁讀回會把 canvas 改成 CPU 繪製，
  // 從 GPU 上的 sprite drawImage 過來時就得每次搬資料，量到的不是真實遊戲的成本。
  const t0 = performance.now()
  for (let i = 0; i < frames; i++) {
    // 每幀讓實體動一點，避免瀏覽器快取同一張畫面
    for (const e of view.enemies) e.pos.x += (i % 2 ? 1 : -1) * 0.5
    draw(ctx, view, { fps: 60 })
  }
  ctx.getImageData(0, 0, 1, 1)
  return { enemies, frames, avgMs: round((performance.now() - t0) / frames) }
}

function round(v: number) {
  return Math.round(v * 100) / 100
}

/** 以玩家為中心、分布在 AOI（約 1280 × 960）內的合成場景；種類混合，部分受傷。 */
function syntheticView(enemyCount: number, gemCount: number, projectileCount: number): RenderView {
  let seed = 12345
  const rand = () => (seed = (seed * 1103515245 + 12345) & 0x7fffffff) / 0x7fffffff
  const center = { x: 1500, y: 1000 }
  const around = (hw: number, hh: number) => ({
    x: center.x + (rand() * 2 - 1) * hw,
    y: center.y + (rand() * 2 - 1) * hh,
  })
  const join: JoinInfo = {
    playerId: 1,
    worldWidth: 3000,
    worldHeight: 2000,
    tickRate: 20,
    playerRadius: 16,
    playerSpeed: 220,
    enemyRadius: 14,
    projectileRadius: 6,
    gemRadius: 7,
    enemyKindRadius: { 1: 14, 2: 11, 3: 22, 4: 26 },
    positionScale: 10,
  }
  const kinds = [1, 1, 2, 2, 3, 4]
  return {
    join,
    self: {
      id: 1,
      pos: { ...center },
      hp: 80,
      maxHp: 100,
      hurt: false,
      choosing: false,
      auraRadius: 100,
      blades: Array.from({ length: 5 }, (_, i) => ({
        x: center.x + Math.cos(i) * 95,
        y: center.y + Math.sin(i) * 95,
      })),
      bladeRadius: 18,
    },
    others: [],
    enemies: Array.from({ length: enemyCount }, (_, i) => {
      const kind = kinds[i % kinds.length]
      return {
        id: i + 1,
        pos: around(640, 480),
        hurt: i % 13 === 0,
        kind,
        radius: join.enemyKindRadius[kind],
        hp: i % 5 === 0 ? 40 : 100,
        maxHp: 100,
      }
    }),
    projectiles: Array.from({ length: projectileCount }, (_, i) => ({
      id: i + 1,
      pos: around(450, 450),
    })),
    gems: Array.from({ length: gemCount }, (_, i) => ({
      id: i + 1,
      pos: around(640, 480),
      value: i % 10 === 0 ? 15 : 1,
    })),
    tick: 1000,
    pendingInputs: 0,
    lastCorrection: 0,
    rttMs: 5,
    progress: { level: 12, xp: 30, xpToNext: 60, kills: 800, survivedMs: 300000 },
  }
}
