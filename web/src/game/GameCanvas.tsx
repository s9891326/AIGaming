import { useEffect, useRef, type RefObject } from 'react'
import type { WorldSnapshot } from '../gen/game/v1/game_pb'
import type { JoinInfo } from '../net/useGameSocket'

interface Props {
  join?: JoinInfo
  snapshotRef: RefObject<WorldSnapshot | undefined>
  /** 尚未加入時顯示在畫面中央的文字。 */
  overlay?: string
}

const GRID = 64
const COLORS = {
  bg: '#16141f',
  grid: 'rgba(255,255,255,0.05)',
  border: '#3a3550',
  self: '#e05a6a',
  selfRing: 'rgba(224,90,106,0.35)',
  other: '#6c8cd5',
  text: '#e8e6f0',
  muted: '#9a96ad',
}

/**
 * 遊戲畫面。React 只負責建立 <canvas>；每一幀由 requestAnimationFrame 從 snapshotRef 讀最新快照繪製，
 * 快照更新不會觸發 React 重新渲染。
 *
 * 目前直接畫最新快照（20Hz），所以移動會有點頓；#5 會加上插值。
 */
export function GameCanvas({ join, snapshotRef, overlay }: Props) {
  const canvasRef = useRef<HTMLCanvasElement>(null)
  const width = join?.worldWidth ?? 960
  const height = join?.worldHeight ?? 640

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

      ctx.fillStyle = COLORS.bg
      ctx.fillRect(0, 0, width, height)

      ctx.strokeStyle = COLORS.grid
      ctx.lineWidth = 1
      ctx.beginPath()
      for (let x = GRID; x < width; x += GRID) {
        ctx.moveTo(x + 0.5, 0)
        ctx.lineTo(x + 0.5, height)
      }
      for (let y = GRID; y < height; y += GRID) {
        ctx.moveTo(0, y + 0.5)
        ctx.lineTo(width, y + 0.5)
      }
      ctx.stroke()

      ctx.strokeStyle = COLORS.border
      ctx.lineWidth = 4
      ctx.strokeRect(2, 2, width - 4, height - 4)

      const snap = snapshotRef.current
      const r = join?.playerRadius ?? 16
      if (join && snap) {
        ctx.font = '12px system-ui, sans-serif'
        ctx.textAlign = 'center'
        for (const p of snap.players) {
          const self = p.id === join.playerId
          if (self) {
            ctx.fillStyle = COLORS.selfRing
            ctx.beginPath()
            ctx.arc(p.x, p.y, r + 6, 0, Math.PI * 2)
            ctx.fill()
          }
          ctx.fillStyle = self ? COLORS.self : COLORS.other
          ctx.beginPath()
          ctx.arc(p.x, p.y, r, 0, Math.PI * 2)
          ctx.fill()
          ctx.fillStyle = COLORS.text
          ctx.fillText(self ? '你' : `P${p.id}`, p.x, p.y - r - 8)
        }
      }

      ctx.textAlign = 'left'
      ctx.font = '12px ui-monospace, Consolas, monospace'
      ctx.fillStyle = COLORS.muted
      const hud = [
        `FPS ${fps.toFixed(0)}`,
        `tick ${snap?.tick ?? '-'}`,
        `players ${snap?.players.length ?? 0}`,
      ]
      hud.forEach((line, i) => ctx.fillText(line, 12, 20 + i * 16))

      if (overlay) {
        ctx.fillStyle = 'rgba(0,0,0,0.5)'
        ctx.fillRect(0, 0, width, height)
        ctx.fillStyle = COLORS.text
        ctx.textAlign = 'center'
        ctx.font = '20px system-ui, sans-serif'
        ctx.fillText(overlay, width / 2, height / 2)
      }

      raf = requestAnimationFrame(draw)
    }
    raf = requestAnimationFrame(draw)
    return () => cancelAnimationFrame(raf)
  }, [join, snapshotRef, overlay, width, height])

  return (
    <canvas
      ref={canvasRef}
      className="game-canvas"
      width={width}
      height={height}
      data-testid="game-canvas"
    />
  )
}
