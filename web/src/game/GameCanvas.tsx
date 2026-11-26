import { useEffect, useRef } from 'react'
import { VIEW_H, VIEW_W, drawFrame } from './draw'
import type { GameClient } from './GameClient'

interface Props {
  game: GameClient
  /** 尚未加入時顯示在畫面中央的文字。 */
  overlay?: string
  /** 開發用延遲模擬（毫秒），顯示在 HUD 提醒自己正在模擬。 */
  simulatedLagMs?: number
}

/**
 * 遊戲畫面。React 只負責建立 <canvas>；每一幀由 requestAnimationFrame：
 *   game.update(now) 產生輸入 → game.view(now) 取得預測 / 插值後的位置 → drawFrame 繪製
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
      drawFrame(ctx, game.view(now), { fps, overlay, simulatedLagMs })
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
