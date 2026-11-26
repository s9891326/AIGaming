import { useCallback, useEffect } from 'react'
import { ConnectionCard } from './components/ConnectionCard'
import { DebugPanel } from './components/DebugPanel'
import { GameOverOverlay } from './components/GameOverOverlay'
import { HealthCard } from './components/HealthCard'
import { LevelUpMenu } from './components/LevelUpMenu'
import { GameCanvas } from './game/GameCanvas'
import { useKeyboardMove } from './game/useKeyboardMove'
import { lagFromLocation } from './net/GameSocket'
import { useGameSocket } from './net/useGameSocket'

// debug 面板：開發模式預設顯示；部署版本在網址加上 ?debug 也能打開。
const showDebug = import.meta.env.DEV || new URLSearchParams(location.search).has('debug')
// 開發用延遲模擬：網址加 ?lag=150 模擬 150ms 往返延遲。
const simulatedLagMs = lagFromLocation()

function App() {
  const socket = useGameSocket({ recordLog: showDebug, simulatedLagMs })
  const { game } = socket
  const onMove = useCallback((x: number, y: number) => game.setMove(x, y), [game])
  useKeyboardMove(onMove)

  // 開發模式：在 DevTools console 用 __aigaming.game.view(performance.now()) 查看預測 / 插值狀態。
  useEffect(() => {
    if (!import.meta.env.DEV) return
    ;(window as unknown as { __aigaming?: unknown }).__aigaming = { game }
  }, [game])

  // 死亡時由 GameOverOverlay 顯示結算，canvas 不另外蓋文字。
  const overlay =
    socket.status !== 'open'
      ? '與伺服器連線中…'
      : !socket.join && !socket.gameOver
        ? '加入遊戲中…'
        : undefined

  return (
    <main className="app">
      <header className="app-header">
        <div>
          <h1>AIGaming</h1>
          <p className="subtitle">survivors-like 網頁遊戲 · 開發中</p>
        </div>
        <p className="hint">
          <kbd>W</kbd>
          <kbd>A</kbd>
          <kbd>S</kbd>
          <kbd>D</kbd> / 方向鍵移動
        </p>
      </header>

      <div className="game-stage">
        <GameCanvas game={game} overlay={overlay} simulatedLagMs={simulatedLagMs} />
        {socket.gameOver && socket.status === 'open' && (
          <GameOverOverlay result={socket.gameOver} onRestart={socket.rejoin} />
        )}
        {socket.levelUp && socket.status === 'open' && (
          <LevelUpMenu offer={socket.levelUp} onChoose={socket.chooseUpgrade} />
        )}
      </div>

      <div className="grid">
        <ConnectionCard socket={socket} />
        <HealthCard />
      </div>

      {showDebug && <DebugPanel log={socket.log} onClear={socket.clearLog} />}
    </main>
  )
}

export default App
