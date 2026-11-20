import { create, toJson, type JsonValue } from '@bufbuild/protobuf'
import { useCallback, useEffect, useRef, useState, type RefObject } from 'react'
import {
  ClientMessageSchema,
  JoinRequestSchema,
  PingSchema,
  PlayerInputSchema,
  ServerMessageSchema,
  type ClientMessage,
  type ServerMessage,
  type WorldSnapshot,
} from '../gen/game/v1/game_pb'
import { GameSocket, defaultSocketUrl, type ConnectionStatus } from './GameSocket'

const PING_INTERVAL_MS = 1000
const RTT_SAMPLES = 10
const LOG_LIMIT = 100
/** 高頻訊息（每秒數次以上），另外存放，避免把 join / input 等重要訊息擠出紀錄。 */
const HIGH_FREQUENCY = new Set(['ping', 'pong', 'snapshot'])
const HIGH_FREQUENCY_LIMIT = 40

export interface MessageLogEntry {
  id: number
  direction: 'in' | 'out'
  at: number // Date.now()
  type: string
  bytes: number
  json: JsonValue
  highFrequency: boolean
}

/** 加入遊戲後伺服器告知的資訊。 */
export interface JoinInfo {
  playerId: number
  worldWidth: number
  worldHeight: number
  tickRate: number
  playerRadius: number
}

export interface GameSocketOptions {
  url?: string
  /** 是否保留收發紀錄給 debug 面板（每則訊息都要轉 JSON，關閉可省效能）。 */
  recordLog?: boolean
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
  /** 加入遊戲後才有值；斷線時清空。 */
  join?: JoinInfo
  /**
   * 最新的世界快照。存在 ref 而不是 state：每秒 20 次的更新不需要觸發 React 重新渲染，
   * Canvas 在 requestAnimationFrame 裡自己讀取。
   */
  snapshotRef: RefObject<WorldSnapshot | undefined>
  /** 設定移動方向；與上一次相同時不會送出。 */
  setMoveInput(x: number, y: number): void
  log: MessageLogEntry[]
  clearLog(): void
}

/**
 * 連線管理 hook：
 * - 建立 GameSocket（自動重連）、每秒送 Ping 量測 RTT
 * - 收到 Welcome 就送 JoinRequest 加入遊戲（重新整理或重連都會重新加入）
 * - 保存最新快照、送出移動輸入
 * - 保留收發紀錄給 debug 面板
 */
export function useGameSocket({
  url = defaultSocketUrl(),
  recordLog = false,
}: GameSocketOptions = {}): GameSocketState {
  const [status, setStatus] = useState<ConnectionStatus>('connecting')
  const [retryInMs, setRetryInMs] = useState<number>()
  const [attempt, setAttempt] = useState(0)
  const [welcome, setWelcome] = useState<{ id: string; version: string }>()
  const [rtt, setRtt] = useState<number>()
  const [rttAvg, setRttAvg] = useState<number>()
  const [join, setJoin] = useState<JoinInfo>()
  const [log, setLog] = useState<MessageLogEntry[]>([])
  const samples = useRef<number[]>([])
  const nextLogId = useRef(0)
  const snapshotRef = useRef<WorldSnapshot | undefined>(undefined)
  const socketRef = useRef<GameSocket | null>(null)
  const joinedRef = useRef(false)
  /** 目前的移動方向與序號；重新加入時要把目前方向補送給伺服器。 */
  const inputRef = useRef({ x: 0, y: 0, seq: 0 })

  const sendInput = useCallback(() => {
    const socket = socketRef.current
    if (!socket || !joinedRef.current) return
    const input = inputRef.current
    input.seq += 1
    socket.send(
      create(ClientMessageSchema, {
        payload: {
          case: 'input',
          value: create(PlayerInputSchema, { seq: input.seq, moveX: input.x, moveY: input.y }),
        },
      }),
    )
  }, [])

  const setMoveInput = useCallback(
    (x: number, y: number) => {
      const input = inputRef.current
      if (input.x === x && input.y === y) return
      input.x = x
      input.y = y
      sendInput()
    },
    [sendInput],
  )

  useEffect(() => {
    const record = (
      direction: MessageLogEntry['direction'],
      msg: ClientMessage | ServerMessage,
      bytes: number,
    ) => {
      if (!recordLog) return
      const json =
        direction === 'in'
          ? toJson(ServerMessageSchema, msg as ServerMessage)
          : toJson(ClientMessageSchema, msg as ClientMessage)
      const type = msg.payload.case ?? 'unknown'
      const entry: MessageLogEntry = {
        id: nextLogId.current++,
        direction,
        at: Date.now(),
        type,
        bytes,
        json,
        highFrequency: HIGH_FREQUENCY.has(type),
      }
      setLog((prev) => {
        // 兩類分開限制數量，再依時間（id）合併。
        const next = [entry, ...prev]
        let hf = 0
        let normal = 0
        return next.filter((e) =>
          e.highFrequency ? ++hf <= HIGH_FREQUENCY_LIMIT : ++normal <= LOG_LIMIT,
        )
      })
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
          setJoin(undefined)
          joinedRef.current = false
          snapshotRef.current = undefined
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
            socket.send(
              create(ClientMessageSchema, {
                payload: { case: 'join', value: create(JoinRequestSchema) },
              }),
            )
            break
          case 'joinAccepted': {
            const v = msg.payload.value
            setJoin({
              playerId: v.playerId,
              worldWidth: v.worldWidth,
              worldHeight: v.worldHeight,
              tickRate: v.tickRate,
              playerRadius: v.playerRadius,
            })
            joinedRef.current = true
            // 加入前就按著方向鍵的話，補送目前方向（伺服器上的新角色是靜止的）。
            if (inputRef.current.x !== 0 || inputRef.current.y !== 0) sendInput()
            break
          }
          case 'snapshot':
            snapshotRef.current = msg.payload.value
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
    socketRef.current = socket
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
      socketRef.current = null
    }
  }, [url, recordLog, sendInput])

  return {
    status,
    retryInMs,
    attempt,
    connectionId: welcome?.id,
    serverVersion: welcome?.version,
    rtt,
    rttAvg,
    join,
    snapshotRef,
    setMoveInput,
    log,
    clearLog: () => setLog([]),
  }
}
