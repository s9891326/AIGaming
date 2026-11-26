import { create, toJson, type JsonValue } from '@bufbuild/protobuf'
import { useCallback, useEffect, useRef, useState } from 'react'
import {
  ClientMessageSchema,
  JoinRequestSchema,
  PingSchema,
  PlayerInputSchema,
  ServerMessageSchema,
  type ClientMessage,
  type ServerMessage,
} from '../gen/game/v1/game_pb'
import { GameClient, type JoinInfo } from '../game/GameClient'
import { GameSocket, defaultSocketUrl, type ConnectionStatus } from './GameSocket'

const PING_INTERVAL_MS = 1000
const RTT_SAMPLES = 10
const LOG_LIMIT = 100
/** 高頻訊息（每秒數次以上），另外存放，避免把 join 等重要訊息擠出紀錄。 */
const HIGH_FREQUENCY = new Set(['ping', 'pong', 'snapshot', 'input'])
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

export interface GameSocketOptions {
  url?: string
  /** 是否保留收發紀錄給 debug 面板（每則訊息都要轉 JSON，關閉可省效能）。 */
  recordLog?: boolean
  /** 開發用：模擬往返延遲（毫秒）。 */
  simulatedLagMs?: number
}

export interface GameOverInfo {
  survivedMs: number
  kills: number
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
  /** 加入遊戲後才有值；斷線或死亡時清空。 */
  join?: JoinInfo
  /** 自己的角色死亡時的結果；按「再來一局」或斷線時清空。 */
  gameOver?: GameOverInfo
  /** 再來一局：重新送 JoinRequest，以新角色加入。 */
  rejoin(): void
  /**
   * 客戶端遊戲狀態（預測、插值）。不放進 React state：
   * 每秒數十次的更新由 GameCanvas 在 requestAnimationFrame 裡直接讀取。
   */
  game: GameClient
  log: MessageLogEntry[]
  clearLog(): void
}

/**
 * 連線管理 hook：
 * - 建立 GameSocket（自動重連）、每秒送 Ping 量測 RTT
 * - 收到 Welcome 就送 JoinRequest（重新整理或重連都會重新加入）
 * - 把 JoinAccepted / WorldSnapshot 交給 GameClient，並替它送出輸入
 * - 保留收發紀錄給 debug 面板
 */
export function useGameSocket({
  url = defaultSocketUrl(),
  recordLog = false,
  simulatedLagMs = 0,
}: GameSocketOptions = {}): GameSocketState {
  const [status, setStatus] = useState<ConnectionStatus>('connecting')
  const [retryInMs, setRetryInMs] = useState<number>()
  const [attempt, setAttempt] = useState(0)
  const [welcome, setWelcome] = useState<{ id: string; version: string }>()
  const [rtt, setRtt] = useState<number>()
  const [rttAvg, setRttAvg] = useState<number>()
  const [join, setJoin] = useState<JoinInfo>()
  const [gameOver, setGameOver] = useState<GameOverInfo>()
  const [log, setLog] = useState<MessageLogEntry[]>([])
  const samples = useRef<number[]>([])
  const nextLogId = useRef(0)
  /** 目前連線的「送出 JoinRequest」函式，給 rejoin 使用（只在事件處理中讀取）。 */
  const sendJoinRef = useRef<(() => void) | null>(null)

  // GameClient 只建立一次，跨重連保留；送出輸入的函式在連線建立時接上。
  const [game] = useState(() => new GameClient())

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
        let hf = 0
        let normal = 0
        return [entry, ...prev].filter((e) =>
          e.highFrequency ? ++hf <= HIGH_FREQUENCY_LIMIT : ++normal <= LOG_LIMIT,
        )
      })
    }

    const socket = new GameSocket(
      url,
      {
        onStatus(s, info) {
          setStatus(s)
          setAttempt(info.attempt)
          setRetryInMs(info.retryInMs)
          if (s !== 'open') {
            setWelcome(undefined)
            setRtt(undefined)
            setRttAvg(undefined)
            setJoin(undefined)
            setGameOver(undefined)
            game.reset()
            game.setRtt(undefined)
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
              sendJoin()
              break
            case 'joinAccepted': {
              const v = msg.payload.value
              const info: JoinInfo = {
                playerId: v.playerId,
                worldWidth: v.worldWidth,
                worldHeight: v.worldHeight,
                tickRate: v.tickRate,
                playerRadius: v.playerRadius,
                playerSpeed: v.playerSpeed,
                enemyRadius: v.enemyRadius,
                projectileRadius: v.projectileRadius,
                gemRadius: v.gemRadius,
              }
              game.onJoin(info)
              setJoin(info)
              setGameOver(undefined)
              break
            }
            case 'gameOver':
              // 角色已被伺服器移除，不會再收到快照；清掉預測 / 插值狀態，等玩家按「再來一局」。
              game.reset()
              setJoin(undefined)
              setGameOver({
                survivedMs: msg.payload.value.survivedMs,
                kills: msg.payload.value.kills,
              })
              break
            case 'snapshot':
              game.onSnapshot(msg.payload.value, performance.now())
              break
            case 'pong': {
              // 用本地的單調時鐘計算，不受兩邊時鐘不同步影響。
              const sample = performance.now() - msg.payload.value.clientTimeMs
              const s = [...samples.current, sample].slice(-RTT_SAMPLES)
              samples.current = s
              setRtt(sample)
              setRttAvg(s.reduce((a, b) => a + b, 0) / s.length)
              game.setRtt(sample)
              break
            }
          }
        },
      },
      { simulatedLagMs },
    )
    const sendJoin = () => {
      socket.send(
        create(ClientMessageSchema, {
          payload: { case: 'join', value: create(JoinRequestSchema) },
        }),
      )
    }
    sendJoinRef.current = sendJoin
    const detachSender = game.attachSender((seq, x, y) => {
      socket.send(
        create(ClientMessageSchema, {
          payload: {
            case: 'input',
            value: create(PlayerInputSchema, { seq, moveX: x, moveY: y }),
          },
        }),
      )
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
      detachSender()
      if (sendJoinRef.current === sendJoin) sendJoinRef.current = null
      socket.close()
    }
  }, [url, recordLog, simulatedLagMs, game])

  const rejoin = useCallback(() => sendJoinRef.current?.(), [])

  return {
    status,
    retryInMs,
    attempt,
    connectionId: welcome?.id,
    serverVersion: welcome?.version,
    rtt,
    rttAvg,
    join,
    gameOver,
    rejoin,
    game,
    log,
    clearLog: () => setLog([]),
  }
}
