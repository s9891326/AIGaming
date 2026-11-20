import { ConnectionCard } from './components/ConnectionCard'
import { DebugPanel } from './components/DebugPanel'
import { HealthCard } from './components/HealthCard'
import { GameCanvas } from './game/GameCanvas'
import { useKeyboardMove } from './game/useKeyboardMove'
import { useGameSocket } from './net/useGameSocket'

// debug 面板：開發模式預設顯示；部署版本在網址加上 ?debug 也能打開。
const showDebug = import.meta.env.DEV || new URLSearchParams(location.search).has('debug')

function App() {
  const socket = useGameSocket({ recordLog: showDebug })
  useKeyboardMove(socket.setMoveInput)

  const overlay =
    socket.status !== 'open' ? '與伺服器連線中…' : !socket.join ? '加入遊戲中…' : undefined

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

      <GameCanvas join={socket.join} snapshotRef={socket.snapshotRef} overlay={overlay} />

      <div className="grid">
        <ConnectionCard socket={socket} />
        <HealthCard />
      </div>

      {showDebug && <DebugPanel log={socket.log} onClear={socket.clearLog} />}
    </main>
  )
}

export default App
