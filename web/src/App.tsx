import { ConnectionCard } from './components/ConnectionCard'
import { DebugPanel } from './components/DebugPanel'
import { HealthCard } from './components/HealthCard'
import { useGameSocket } from './net/useGameSocket'

// debug 面板：開發模式預設顯示；部署版本在網址加上 ?debug 也能打開。
const showDebug = import.meta.env.DEV || new URLSearchParams(location.search).has('debug')

function App() {
  const socket = useGameSocket()

  return (
    <main className="app">
      <h1>AIGaming</h1>
      <p className="subtitle">survivors-like 網頁遊戲 · 開發中</p>

      <div className="grid">
        <ConnectionCard socket={socket} />
        <HealthCard />
      </div>

      {showDebug && <DebugPanel log={socket.log} onClear={socket.clearLog} />}
    </main>
  )
}

export default App
