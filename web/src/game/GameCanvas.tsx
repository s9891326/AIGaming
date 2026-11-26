import { useEffect, useRef } from 'react'
import type { GameClient, RenderPlayer } from './GameClient'
import type { Vec } from './sim'

interface Props {
  game: GameClient
  /** 尚未加入時顯示在畫面中央的文字。 */
  overlay?: string
  /** 開發用延遲模擬（毫秒），顯示在 HUD 提醒自己正在模擬。 */
  simulatedLagMs?: number
}

/** 視窗（相機看到的範圍）大小；世界比這個大，相機跟著自己移動。 */
const VIEW_W = 960
const VIEW_H = 640
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
}

/**
 * 遊戲畫面。React 只負責建立 <canvas>；每一幀由 requestAnimationFrame：
 *   game.update(now) 產生輸入 → game.view(now) 取得預測 / 插值後的位置 → 以自己為中心繪製
 * 快照與輸入都不會觸發 React 重新渲染。
 */
export function GameCanvas({ game, overlay, simulatedLagMs }: Props) {
  const canvasRef = useRef<HTMLCanvasElement>(null)

  useEffect(() => {
    const canvas = canvasRef.current
    const ctx = canvas?.getContext('2d')
    if (!canvas || !ctx) return

    let raf = 0
    let frames = 0
    let fps = 0
    let fpsSince = performance.now()

    const draw = (now: number) => {
      frames++
      if (now - fpsSince >= 500) {
        fps = (frames * 1000) / (now - fpsSince)
        frames = 0
        fpsSince = now
      }

      game.update(now)
      const v = game.view(now)
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
      const gr = v.join?.gemRadius ?? 7
      ctx.fillStyle = COLORS.gem
      ctx.strokeStyle = COLORS.gemEdge
      ctx.lineWidth = 1.5
      for (const g of v.gems) {
        // 菱形
        ctx.beginPath()
        ctx.moveTo(g.pos.x, g.pos.y - gr)
        ctx.lineTo(g.pos.x + gr * 0.75, g.pos.y)
        ctx.lineTo(g.pos.x, g.pos.y + gr)
        ctx.lineTo(g.pos.x - gr * 0.75, g.pos.y)
        ctx.closePath()
        ctx.fill()
        ctx.stroke()
      }

      const er = v.join?.enemyRadius ?? 14
      ctx.strokeStyle = COLORS.enemyEdge
      ctx.lineWidth = 2
      for (const e of v.enemies) {
        ctx.fillStyle = e.hurt ? COLORS.hurt : COLORS.enemy
        ctx.beginPath()
        ctx.arc(e.pos.x, e.pos.y, er, 0, Math.PI * 2)
        ctx.fill()
        ctx.stroke()
      }

      const pr = v.join?.projectileRadius ?? 6
      ctx.fillStyle = COLORS.projectile
      ctx.shadowColor = COLORS.projectile
      ctx.shadowBlur = 10
      for (const p of v.projectiles) {
        ctx.beginPath()
        ctx.arc(p.pos.x, p.pos.y, pr, 0, Math.PI * 2)
        ctx.fill()
      }
      ctx.shadowBlur = 0

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

      raf = requestAnimationFrame(draw)
    }
    raf = requestAnimationFrame(draw)
    return () => cancelAnimationFrame(raf)
  }, [game, overlay, simulatedLagMs])

  return (
    <canvas
      ref={canvasRef}
      className="game-canvas"
      width={VIEW_W}
      height={VIEW_H}
      data-testid="game-canvas"
    />
  )
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
  ctx.fillText(label, p.pos.x, p.pos.y - r - 16)
}
