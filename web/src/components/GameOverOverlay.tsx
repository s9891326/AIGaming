import { useEffect } from 'react'
import type { GameOverInfo } from '../net/useGameSocket'

interface Props {
  result: GameOverInfo
  onRestart(): void
}

function formatDuration(ms: number): string {
  const total = Math.floor(ms / 1000)
  const m = Math.floor(total / 60)
  const s = total % 60
  return `${m}:${String(s).padStart(2, '0')}`
}

/** 角色死亡時蓋在遊戲畫面上的結算畫面。Enter / 空白鍵也可以再來一局。 */
export function GameOverOverlay({ result, onRestart }: Props) {
  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      if (e.code === 'Enter' || e.code === 'Space') {
        e.preventDefault()
        onRestart()
      }
    }
    window.addEventListener('keydown', onKey)
    return () => window.removeEventListener('keydown', onKey)
  }, [onRestart])

  return (
    <div className="game-over" role="dialog" aria-labelledby="game-over-title">
      <h2 id="game-over-title">Game Over</h2>
      <dl>
        <dt>存活時間</dt>
        <dd data-testid="survived">{formatDuration(result.survivedMs)}</dd>
        <dt>擊殺數</dt>
        <dd>{result.kills}</dd>
      </dl>
      <button type="button" className="primary" onClick={onRestart} autoFocus>
        再來一局
      </button>
      <p className="muted">或按 Enter</p>
    </div>
  )
}
