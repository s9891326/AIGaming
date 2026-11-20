import { useState } from 'react'
import type { MessageLogEntry } from '../net/useGameSocket'

interface Props {
  log: MessageLogEntry[]
  onClear(): void
}

/**
 * 開發用：把收發的 protobuf 訊息解碼成 JSON 顯示，彌補 binary frame 在 DevTools 裡看不懂的缺點。
 */
export function DebugPanel({ log, onClear }: Props) {
  const [hidePing, setHidePing] = useState(false)
  const entries = hidePing ? log.filter((e) => e.type !== 'ping' && e.type !== 'pong') : log

  return (
    <section className="card debug">
      <header className="debug-header">
        <h2>協議 Debug</h2>
        <label className="muted">
          <input
            type="checkbox"
            checked={hidePing}
            onChange={(e) => setHidePing(e.target.checked)}
          />{' '}
          隱藏 ping / pong
        </label>
        <button type="button" onClick={onClear}>
          清除
        </button>
      </header>
      {entries.length === 0 ? (
        <p className="muted">尚無訊息</p>
      ) : (
        <ol className="debug-log">
          {entries.map((e) => (
            <li key={e.id} className={`debug-${e.direction}`}>
              <span className="debug-meta">
                {e.direction === 'in' ? '←' : '→'} {new Date(e.at).toLocaleTimeString()}{' '}
                <strong>{e.type}</strong> <span className="muted">{e.bytes} B</span>
              </span>
              <code>{JSON.stringify(e.json)}</code>
            </li>
          ))}
        </ol>
      )}
    </section>
  )
}
