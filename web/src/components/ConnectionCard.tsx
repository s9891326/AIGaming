import type { GameSocketState } from '../net/useGameSocket'

const STATUS_TEXT = {
  connecting: '連線中…',
  open: '已連線',
  closed: '已斷線',
} as const

export function ConnectionCard({ socket }: { socket: GameSocketState }) {
  const { status, retryInMs, attempt, connectionId, serverVersion, rtt, rttAvg } = socket

  return (
    <section className="card">
      <h2>WebSocket 連線</h2>
      <dl>
        <dt>狀態</dt>
        <dd className={`status status-${status}`} data-testid="ws-status">
          ● {STATUS_TEXT[status]}
          {status === 'closed' && retryInMs !== undefined && (
            <span className="muted">
              {' '}
              （{(retryInMs / 1000).toFixed(1)} 秒後重連，第 {attempt} 次）
            </span>
          )}
        </dd>
        <dt>延遲（RTT）</dt>
        <dd data-testid="ws-rtt">
          {rtt === undefined ? (
            '—'
          ) : (
            <>
              {rtt.toFixed(1)} ms <span className="muted">（平均 {rttAvg?.toFixed(1)} ms）</span>
            </>
          )}
        </dd>
        <dt>連線 ID</dt>
        <dd>
          <code>{connectionId ?? '—'}</code>
        </dd>
        <dt>伺服器版本</dt>
        <dd>{serverVersion ?? '—'}</dd>
      </dl>
    </section>
  )
}
