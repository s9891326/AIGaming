import { create, toJson, type JsonValue } from '@bufbuild/protobuf'
import { useEffect, useRef, useState } from 'react'
import {
  ClientMessageSchema,
  PingSchema,
  ServerMessageSchema,
  type ClientMessage,
  type ServerMessage,
} from '../gen/game/v1/game_pb'
import { GameSocket, defaultSocketUrl, type ConnectionStatus } from './GameSocket'

const PING_INTERVAL_MS = 1000
const RTT_SAMPLES = 10
const LOG_LIMIT = 100

export interface MessageLogEntry {
  id: number
  direction: 'in' | 'out'
  at: number // Date.now()
  type: string
  bytes: number
  json: JsonValue
}

export interface GameSocketState {
  status: ConnectionStatus
  /** 重連前的等待時間（毫秒），只有 status === 'closed' 且會重連時有值。 */
  retryInMs?: number
  attempt: number
  connectionId?: string
  serverVersion?: string
  /** 最近一次的往返延遲（毫秒）。 */
  rtt?: number
  /** 最近 RTT_SAMPLES 次的平均往返延遲（毫秒）。 */
  rttAvg?: number
  log: MessageLogEntry[]
  clearLog(): void
}

/**
 * 連線管理 hook：建立 GameSocket、每秒送 Ping 量測 RTT，並保留收發紀錄給 debug 面板。
 */
export function useGameSocket(url: string = defaultSocketUrl()): GameSocketState {
  const [status, setStatus] = useState<ConnectionStatus>('connecting')
  const [retryInMs, setRetryInMs] = useState<number>()
  const [attempt, setAttempt] = useState(0)
  const [welcome, setWelcome] = useState<{ id: string; version: string }>()
  const [rtt, setRtt] = useState<number>()
  const [rttAvg, setRttAvg] = useState<number>()
  const [log, setLog] = useState<MessageLogEntry[]>([])
  const samples = useRef<number[]>([])
  const nextLogId = useRef(0)

  useEffect(() => {
    const record = (
      direction: MessageLogEntry['direction'],
      msg: ClientMessage | ServerMessage,
      bytes: number,
    ) => {
      const json =
        direction === 'in'
          ? toJson(ServerMessageSchema, msg as ServerMessage)
          : toJson(ClientMessageSchema, msg as ClientMessage)
      const entry: MessageLogEntry = {
        id: nextLogId.current++,
        direction,
        at: Date.now(),
        type: msg.payload.case ?? 'unknown',
        bytes,
        json,
      }
      setLog((prev) => [entry, ...prev].slice(0, LOG_LIMIT))
    }

    const socket = new GameSocket(url, {
      onStatus(s, info) {
        setStatus(s)
        setAttempt(info.attempt)
        setRetryInMs(info.retryInMs)
        if (s !== 'open') {
          setWelcome(undefined)
          setRtt(undefined)
          setRttAvg(undefined)
          samples.current = []
        }
      },
      onSent(msg, bytes) {
        record('out', msg, bytes)
      },
      onMessage(msg, bytes) {
        record('in', msg, bytes)
        switch (msg.payload.case) {
          case 'welcome':
            setWelcome({
              id: msg.payload.value.connectionId,
              version: msg.payload.value.serverVersion,
            })
            break
          case 'pong': {
            // 用本地的單調時鐘計算，不受兩邊時鐘不同步影響。
            const sample = performance.now() - msg.payload.value.clientTimeMs
            const s = [...samples.current, sample].slice(-RTT_SAMPLES)
            samples.current = s
            setRtt(sample)
            setRttAvg(s.reduce((a, b) => a + b, 0) / s.length)
            break
          }
        }
      },
    })
    socket.connect()

    const ping = setInterval(() => {
      socket.send(
        create(ClientMessageSchema, {
          payload: { case: 'ping', value: create(PingSchema, { clientTimeMs: performance.now() }) },
        }),
      )
    }, PING_INTERVAL_MS)

    return () => {
      clearInterval(ping)
      socket.close()
    }
  }, [url])

  return {
    status,
    retryInMs,
    attempt,
    connectionId: welcome?.id,
    serverVersion: welcome?.version,
    rtt,
    rttAvg,
    log,
    clearLog: () => setLog([]),
  }
}
