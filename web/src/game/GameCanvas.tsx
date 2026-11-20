import { useEffect, useRef } from 'react'
import type { GameClient } from './GameClient'
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
      const cam: Vec = v.self ?? (world ? { x: world.w / 2, y: world.h / 2 } : { x: 0, y: 0 })
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

      ctx.font = '12px system-ui, sans-serif'
      ctx.textAlign = 'center'
      for (const o of v.others) {
        drawPlayer(ctx, o.pos, r, COLORS.other, `P${o.id}`)
      }
      if (v.self) {
        ctx.fillStyle = COLORS.selfRing
        ctx.beginPath()
        ctx.arc(v.self.x, v.self.y, r + 6, 0, Math.PI * 2)
        ctx.fill()
        drawPlayer(ctx, v.self, r, COLORS.self, '你')
      }
      ctx.restore()

      // HUD（固定在畫面左上，不跟相機移動）
      ctx.textAlign = 'left'
      ctx.font = '12px ui-monospace, Consolas, monospace'
      const hud: [string, string?][] = [
        [`FPS ${fps.toFixed(0)}`],
        [`tick ${v.tick ?? '-'}`],
        [`RTT ${v.rttMs === undefined ? '-' : `${v.rttMs.toFixed(0)} ms`}`],
        [`players ${v.others.length + (v.self ? 1 : 0)}`],
        [`pending inputs ${v.pendingInputs}`],
        [
          `correction ${v.lastCorrection.toFixed(2)}`,
          v.lastCorrection > 0.01 ? COLORS.warn : undefined,
        ],
        [`pos ${v.self ? `${v.self.x.toFixed(0)}, ${v.self.y.toFixed(0)}` : '-'}`],
      ]
      if (simulatedLagMs) hud.push([`simulated lag ${simulatedLagMs} ms`, COLORS.warn])
      hud.forEach(([line, color], i) => {
        ctx.fillStyle = color ?? COLORS.muted
        ctx.fillText(line, 12, 20 + i * 16)
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

function drawPlayer(
  ctx: CanvasRenderingContext2D,
  p: Vec,
  r: number,
  color: string,
  label: string,
) {
  ctx.fillStyle = color
  ctx.beginPath()
  ctx.arc(p.x, p.y, r, 0, Math.PI * 2)
  ctx.fill()
  ctx.fillStyle = COLORS.text
  ctx.fillText(label, p.x, p.y - r - 8)
}
