import { useCallback, useEffect, useState } from 'react'
import { fetchHealth, type HealthResponse } from '../api/health'

type Status =
  { kind: 'loading' } | { kind: 'ok'; data: HealthResponse } | { kind: 'error'; message: string }

/** 呼叫 GET /api/healthz 顯示後端 HTTP 狀態。 */
export function HealthCard() {
  const [status, setStatus] = useState<Status>({ kind: 'loading' })

  // 只在非同步回呼裡更新狀態；初始狀態已經是 loading，不需要在 effect 裡同步 setState。
  const load = useCallback((signal?: AbortSignal) => {
    fetchHealth(signal)
      .then((data) => setStatus({ kind: 'ok', data }))
      .catch((err: unknown) => {
        if (signal?.aborted) return
        setStatus({ kind: 'error', message: err instanceof Error ? err.message : String(err) })
      })
  }, [])

  useEffect(() => {
    const ctrl = new AbortController()
    load(ctrl.signal)
    return () => ctrl.abort()
  }, [load])

  const retry = () => {
    setStatus({ kind: 'loading' })
    load()
  }

  return (
    <section className="card">
      <h2>伺服器狀態（HTTP）</h2>
      {status.kind === 'loading' && <p>檢查中…</p>}
      {status.kind === 'ok' && (
        <dl>
          <dt>狀態</dt>
          <dd className="ok">● {status.data.status}</dd>
          <dt>版本</dt>
          <dd>{status.data.version}</dd>
          <dt>伺服器時間</dt>
          <dd>{new Date(status.data.serverTime).toLocaleString()}</dd>
        </dl>
      )}
      {status.kind === 'error' && (
        <p className="error">● 無法連線到後端（{status.message}）。請確認 server 已啟動。</p>
      )}
      <button type="button" onClick={retry}>
        重新檢查
      </button>
    </section>
  )
}
