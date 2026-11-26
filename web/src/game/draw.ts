import type { RenderPlayer, RenderView } from './GameClient'
import type { Vec } from './sim'

export interface DrawOptions {
  fps: number
  /** 尚未加入時顯示在畫面中央的文字。 */
  overlay?: string
  /** 開發用延遲模擬（毫秒），顯示在 HUD。 */
  simulatedLagMs?: number
}

/** 視窗（相機看到的範圍）大小；世界比這個大，相機跟著自己移動。 */
export const VIEW_W = 960
export const VIEW_H = 640
const GRID = 64
const COLORS = {
  void: '#0c0b12',
  bg: '#16141f',
  grid: 'rgba(255,255,255,0.06)',
  border: '#3a3550',
  self: '#e05a6a',
  selfRing: 'rgba(224,90,106,0.35)',
  other: '#6c8cd5',
  text: '#e8e6f0',
  muted: '#9a96ad',
  warn: '#d9a83f',
  enemy: '#5f9e6e',
  enemyEdge: '#3c6b48',
  hp: '#4caf7d',
  hpLow: '#e5534b',
  hurt: '#ffffff',
  projectile: '#ffd36b',
  gem: '#5ad1e6',
  gemEdge: '#2b8fa3',
  xp: '#5ad1e6',
  gemBig: '#ffd36b',
  gemBigEdge: '#b8902f',
  blade: '#cfd8ff',
  bladeEdge: '#7f8bc2',
  aura: 'rgba(255, 211, 107, 0.12)',
  auraEdge: 'rgba(255, 211, 107, 0.45)',
}

/**
 * 畫一幀遊戲畫面。純函式：只依 view 與 opts 繪製，不讀其他狀態，
 * 所以可以在離屏 canvas 上用合成的場景量測繪製耗時（drawBench.ts）。
 */
export function drawFrame(ctx: CanvasRenderingContext2D, v: RenderView, opts: DrawOptions) {
  const { fps, overlay, simulatedLagMs } = opts
  const world = v.join ? { w: v.join.worldWidth, h: v.join.worldHeight } : undefined
  const r = v.join?.playerRadius ?? 16

  // 相機：以自己為中心；還沒有位置時看世界中央。
  const cam: Vec = v.self?.pos ?? (world ? { x: world.w / 2, y: world.h / 2 } : { x: 0, y: 0 })
  const ox = Math.round(VIEW_W / 2 - cam.x)
  const oy = Math.round(VIEW_H / 2 - cam.y)

  ctx.fillStyle = COLORS.void
  ctx.fillRect(0, 0, VIEW_W, VIEW_H)

  ctx.save()
  ctx.translate(ox, oy)
  if (world) {
    ctx.fillStyle = COLORS.bg
    ctx.fillRect(0, 0, world.w, world.h)
    drawGrid(ctx, world, cam)
    ctx.strokeStyle = COLORS.border
    ctx.lineWidth = 4
    ctx.strokeRect(-2, -2, world.w + 4, world.h + 4)
  }

  // 由下而上：寶石 → 敵人 → 投射物 → 玩家。
  // 效能（#10 用 drawBench 量測後的結論）：逐個畫圓本身就很快（2000 隻約 3ms / 幀）；
  // 「合併成一條大 path」與「每種外觀先畫成小圖再 drawImage」實測反而更慢，所以不採用。
  // 有效的是：略過畫面外的實體（AOI 會多送邊距外的實體）、投射物光暈不用 shadowBlur。
  const view = visibleRect(cam, 40)
  const gr0 = v.join?.gemRadius ?? 7
  ctx.lineWidth = 1.5
  for (const g of v.gems) {
    if (!view.contains(g.pos)) continue
    // 菱形；高價值（巨漢、精英掉的）比較大、金色
    const big = g.value > 1
    const gr = big ? gr0 * 1.6 : gr0
    ctx.fillStyle = big ? COLORS.gemBig : COLORS.gem
    ctx.strokeStyle = big ? COLORS.gemBigEdge : COLORS.gemEdge
    ctx.beginPath()
    ctx.moveTo(g.pos.x, g.pos.y - gr)
    ctx.lineTo(g.pos.x + gr * 0.75, g.pos.y)
    ctx.lineTo(g.pos.x, g.pos.y + gr)
    ctx.lineTo(g.pos.x - gr * 0.75, g.pos.y)
    ctx.closePath()
    ctx.fill()
    ctx.stroke()
  }

  const visibleEnemies = v.enemies.filter((e) => view.contains(e.pos, e.radius))
  for (const e of visibleEnemies) {
    const style = ENEMY_STYLE[e.kind] ?? ENEMY_STYLE[1]
    ctx.fillStyle = e.hurt ? COLORS.hurt : style.fill
    ctx.strokeStyle = style.edge
    ctx.lineWidth = e.kind === 4 ? 4 : 2
    ctx.beginPath()
    ctx.arc(e.pos.x, e.pos.y, e.radius, 0, Math.PI * 2)
    ctx.fill()
    ctx.stroke()
  }
  // 耐打的敵人（巨漢、精英）受傷後頭上顯示血條：底色一批、血量一批
  const bars = visibleEnemies.filter(
    (e) => (e.kind === 3 || e.kind === 4) && e.maxHp > 0 && e.hp < e.maxHp,
  )
  if (bars.length > 0) {
    ctx.fillStyle = 'rgba(0,0,0,0.6)'
    ctx.beginPath()
    for (const e of bars) ctx.rect(e.pos.x - e.radius, e.pos.y - e.radius - 8, e.radius * 2, 4)
    ctx.fill()
    ctx.fillStyle = COLORS.hpLow
    ctx.beginPath()
    for (const e of bars) {
      const ratio = Math.max(0, e.hp / e.maxHp)
      ctx.rect(e.pos.x - e.radius, e.pos.y - e.radius - 8, e.radius * 2 * ratio, 4)
    }
    ctx.fill()
  }

  const pr = v.join?.projectileRadius ?? 6
  const glow = projectileSprite(pr)
  const half = glow.width / 2
  for (const p of v.projectiles) {
    if (!view.contains(p.pos)) continue
    ctx.drawImage(glow, Math.round(p.pos.x - half), Math.round(p.pos.y - half))
  }

  // 聖光環：畫在角色底下的半透明圓
  for (const p of [...v.others, ...(v.self ? [v.self] : [])]) {
    if (p.auraRadius <= 0) continue
    ctx.fillStyle = COLORS.aura
    ctx.strokeStyle = COLORS.auraEdge
    ctx.lineWidth = 2
    ctx.beginPath()
    ctx.arc(p.pos.x, p.pos.y, p.auraRadius, 0, Math.PI * 2)
    ctx.fill()
    ctx.stroke()
  }

  // 旋刃
  ctx.beginPath()
  for (const p of [...v.others, ...(v.self ? [v.self] : [])]) {
    for (const b of p.blades) {
      ctx.moveTo(b.x + p.bladeRadius, b.y)
      ctx.arc(b.x, b.y, p.bladeRadius, 0, Math.PI * 2)
    }
  }
  ctx.fillStyle = COLORS.blade
  ctx.strokeStyle = COLORS.bladeEdge
  ctx.lineWidth = 2
  ctx.fill()
  ctx.stroke()

  ctx.font = '12px system-ui, sans-serif'
  ctx.textAlign = 'center'
  for (const o of v.others) {
    drawPlayer(ctx, o, r, COLORS.other, `P${o.id}`)
  }
  if (v.self) {
    ctx.fillStyle = COLORS.selfRing
    ctx.beginPath()
    ctx.arc(v.self.pos.x, v.self.pos.y, r + 6, 0, Math.PI * 2)
    ctx.fill()
    drawPlayer(ctx, v.self, r, COLORS.self, '你')
  }
  ctx.restore()

  // 自己受傷：畫面邊緣閃紅。
  if (v.self?.hurt) {
    const g = ctx.createRadialGradient(
      VIEW_W / 2,
      VIEW_H / 2,
      VIEW_H * 0.35,
      VIEW_W / 2,
      VIEW_H / 2,
      VIEW_W * 0.6,
    )
    g.addColorStop(0, 'rgba(229,83,75,0)')
    g.addColorStop(1, 'rgba(229,83,75,0.45)')
    ctx.fillStyle = g
    ctx.fillRect(0, 0, VIEW_W, VIEW_H)
  }

  // 自己的血條（左下，固定在畫面上）
  if (v.self && v.self.maxHp > 0) {
    const w = 240
    const h = 14
    const x = 16
    const y = VIEW_H - 16 - h
    const ratio = Math.max(0, v.self.hp / v.self.maxHp)
    ctx.fillStyle = 'rgba(0,0,0,0.5)'
    ctx.fillRect(x - 2, y - 2, w + 4, h + 4)
    ctx.fillStyle = ratio > 0.3 ? COLORS.hp : COLORS.hpLow
    ctx.fillRect(x, y, w * ratio, h)
    ctx.fillStyle = COLORS.text
    ctx.textAlign = 'left'
    ctx.font = '12px ui-monospace, Consolas, monospace'
    ctx.fillText(`HP ${v.self.hp} / ${v.self.maxHp}`, x + w + 12, y + h - 2)
  }

  // 經驗條（畫面最上方，全寬）
  const prog = v.progress
  if (prog) {
    const h = 8
    const ratio = prog.xpToNext > 0 ? Math.min(1, prog.xp / prog.xpToNext) : 0
    ctx.fillStyle = 'rgba(0,0,0,0.55)'
    ctx.fillRect(0, 0, VIEW_W, h)
    ctx.fillStyle = COLORS.xp
    ctx.fillRect(0, 0, VIEW_W * ratio, h)
    ctx.font = 'bold 14px system-ui, sans-serif'
    ctx.textAlign = 'right'
    ctx.fillStyle = COLORS.text
    ctx.fillText(`Lv ${prog.level}`, VIEW_W - 12, h + 18)
    ctx.font = '12px ui-monospace, Consolas, monospace'
    ctx.fillStyle = COLORS.muted
    ctx.fillText(`XP ${prog.xp} / ${prog.xpToNext}  ·  擊殺 ${prog.kills}`, VIEW_W - 12, h + 36)
    ctx.font = 'bold 16px ui-monospace, Consolas, monospace'
    ctx.textAlign = 'center'
    ctx.fillStyle = COLORS.text
    ctx.fillText(formatClock(prog.survivedMs), VIEW_W / 2, h + 22)
  }

  // HUD（固定在畫面左上，不跟相機移動）
  ctx.textAlign = 'left'
  ctx.font = '12px ui-monospace, Consolas, monospace'
  const hud: [string, string?][] = [
    [`FPS ${fps.toFixed(0)}`],
    [`tick ${v.tick ?? '-'}`],
    [`RTT ${v.rttMs === undefined ? '-' : `${v.rttMs.toFixed(0)} ms`}`],
    [`players ${v.others.length + (v.self ? 1 : 0)}`],
    [`enemies ${v.enemies.length}`],
    [`projectiles ${v.projectiles.length}  gems ${v.gems.length}`],
    [`pending inputs ${v.pendingInputs}`],
    [
      `correction ${v.lastCorrection.toFixed(2)}`,
      v.lastCorrection > 0.01 ? COLORS.warn : undefined,
    ],
    [`pos ${v.self ? `${v.self.pos.x.toFixed(0)}, ${v.self.pos.y.toFixed(0)}` : '-'}`],
  ]
  if (simulatedLagMs) hud.push([`simulated lag ${simulatedLagMs} ms`, COLORS.warn])
  hud.forEach(([line, color], i) => {
    ctx.fillStyle = color ?? COLORS.muted
    ctx.fillText(line, 12, 28 + i * 16)
  })

  if (overlay) {
    ctx.fillStyle = 'rgba(0,0,0,0.5)'
    ctx.fillRect(0, 0, VIEW_W, VIEW_H)
    ctx.fillStyle = COLORS.text
    ctx.textAlign = 'center'
    ctx.font = '20px system-ui, sans-serif'
    ctx.fillText(overlay, VIEW_W / 2, VIEW_H / 2)
  }
}

/** 各種敵人的顏色（1 普通、2 疾行者、3 巨漢、4 精英）。 */
const ENEMY_STYLE: Record<number, { fill: string; edge: string }> = {
  1: { fill: '#5f9e6e', edge: '#3c6b48' },
  2: { fill: '#c9b458', edge: '#8a7a2e' },
  3: { fill: '#9e4a4a', edge: '#5e2626' },
  4: { fill: '#8e5fc9', edge: '#e0c2ff' },
}

/** 相機看得到的世界範圍（再加 margin），用來略過畫面外的實體。 */
function visibleRect(cam: Vec, margin: number) {
  const x0 = cam.x - VIEW_W / 2 - margin
  const x1 = cam.x + VIEW_W / 2 + margin
  const y0 = cam.y - VIEW_H / 2 - margin
  const y1 = cam.y + VIEW_H / 2 + margin
  return {
    contains: (p: Vec, r = 0) => p.x + r >= x0 && p.x - r <= x1 && p.y + r >= y0 && p.y - r <= y1,
  }
}

let glowCache: { radius: number; canvas: HTMLCanvasElement } | undefined

/** 帶光暈的投射物圖（只畫一次，之後每幀 drawImage）。 */
function projectileSprite(radius: number): HTMLCanvasElement {
  if (glowCache?.radius === radius) return glowCache.canvas
  const size = Math.ceil((radius + 10) * 2)
  const c = document.createElement('canvas')
  c.width = c.height = size
  const g = c.getContext('2d')!
  g.shadowColor = COLORS.projectile
  g.shadowBlur = 10
  g.fillStyle = COLORS.projectile
  g.beginPath()
  g.arc(size / 2, size / 2, radius, 0, Math.PI * 2)
  g.fill()
  glowCache = { radius, canvas: c }
  return c
}

/** 存活時間 mm:ss */
function formatClock(ms: number): string {
  const s = Math.floor(ms / 1000)
  return `${String(Math.floor(s / 60)).padStart(2, '0')}:${String(s % 60).padStart(2, '0')}`
}

/** 只畫相機看得到的格線，世界再大也不會變慢。 */
function drawGrid(ctx: CanvasRenderingContext2D, world: { w: number; h: number }, cam: Vec) {
  const x0 = Math.max(0, Math.floor((cam.x - VIEW_W / 2) / GRID) * GRID)
  const x1 = Math.min(world.w, cam.x + VIEW_W / 2)
  const y0 = Math.max(0, Math.floor((cam.y - VIEW_H / 2) / GRID) * GRID)
  const y1 = Math.min(world.h, cam.y + VIEW_H / 2)
  ctx.strokeStyle = COLORS.grid
  ctx.lineWidth = 1
  ctx.beginPath()
  for (let x = x0; x <= x1; x += GRID) {
    ctx.moveTo(x + 0.5, y0)
    ctx.lineTo(x + 0.5, y1)
  }
  for (let y = y0; y <= y1; y += GRID) {
    ctx.moveTo(x0, y + 0.5)
    ctx.lineTo(x1, y + 0.5)
  }
  ctx.stroke()
}

/** 畫一個角色：圓形、名稱、頭上的小血條；剛受傷時閃白。 */
function drawPlayer(
  ctx: CanvasRenderingContext2D,
  p: RenderPlayer,
  r: number,
  color: string,
  label: string,
) {
  ctx.fillStyle = p.hurt ? COLORS.hurt : color
  ctx.beginPath()
  ctx.arc(p.pos.x, p.pos.y, r, 0, Math.PI * 2)
  ctx.fill()

  if (p.maxHp > 0) {
    const w = r * 2.4
    const h = 4
    const x = p.pos.x - w / 2
    const y = p.pos.y - r - 10
    const ratio = Math.max(0, p.hp / p.maxHp)
    ctx.fillStyle = 'rgba(0,0,0,0.6)'
    ctx.fillRect(x, y, w, h)
    ctx.fillStyle = ratio > 0.3 ? COLORS.hp : COLORS.hpLow
    ctx.fillRect(x, y, w * ratio, h)
  }

  ctx.fillStyle = COLORS.text
  ctx.fillText(p.choosing ? `${label}（選升級中）` : label, p.pos.x, p.pos.y - r - 16)
}
