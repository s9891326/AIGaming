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
  // 快照每秒 20 則，預設隱藏高頻訊息，才看得到 join / input 等事件。
  const [hideHighFrequency, setHideHighFrequency] = useState(true)
  const entries = hideHighFrequency ? log.filter((e) => !e.highFrequency) : log

  return (
    <section className="card debug">
      <header className="debug-header">
        <h2>協議 Debug</h2>
        <label className="muted">
          <input
            type="checkbox"
            checked={hideHighFrequency}
            onChange={(e) => setHideHighFrequency(e.target.checked)}
          />{' '}
          隱藏高頻訊息（ping / pong / snapshot）
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
