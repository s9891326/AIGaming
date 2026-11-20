import { fromBinary, toBinary } from '@bufbuild/protobuf'
import {
  ClientMessageSchema,
  ServerMessageSchema,
  type ClientMessage,
  type ServerMessage,
} from '../gen/game/v1/game_pb'

export type ConnectionStatus = 'connecting' | 'open' | 'closed'

export interface GameSocketListener {
  onStatus?(status: ConnectionStatus, info: { attempt: number; retryInMs?: number }): void
  onMessage?(msg: ServerMessage, bytes: number): void
  onSent?(msg: ClientMessage, bytes: number): void
}

export interface GameSocketOptions {
  /** 第一次重連等待時間（毫秒），之後每次加倍。 */
  minReconnectDelayMs?: number
  /** 重連等待時間上限（毫秒）。 */
  maxReconnectDelayMs?: number
}

/** 依目前頁面推算 WebSocket 位址；開發時由 Vite、部署時由 nginx 代理到後端。 */
export function defaultSocketUrl(): string {
  const scheme = location.protocol === 'https:' ? 'wss' : 'ws'
  return `${scheme}://${location.host}/ws`
}

/**
 * GameSocket 管理一條到遊戲伺服器的 WebSocket：
 * - 訊息一律是 binary frame，內容為 protobuf 編碼的 ClientMessage / ServerMessage
 * - 斷線後以指數退避自動重連（0.5s → 1s → 2s … 最多 5s），連上後重設
 *
 * 這個類別與 React 無關，方便之後在遊戲迴圈中直接使用。
 */
export class GameSocket {
  private ws: WebSocket | null = null
  private status: ConnectionStatus = 'closed'
  private attempt = 0
  private retryTimer: ReturnType<typeof setTimeout> | null = null
  private stopped = true
  private readonly minDelay: number
  private readonly maxDelay: number
  private readonly url: string
  private readonly listener: GameSocketListener

  constructor(url: string, listener: GameSocketListener, opts: GameSocketOptions = {}) {
    this.url = url
    this.listener = listener
    this.minDelay = opts.minReconnectDelayMs ?? 500
    this.maxDelay = opts.maxReconnectDelayMs ?? 5000
  }

  get currentStatus(): ConnectionStatus {
    return this.status
  }

  /** 開始連線（斷線會自動重連，直到呼叫 close）。 */
  connect(): void {
    this.stopped = false
    this.open()
  }

  /** 關閉連線並停止重連。 */
  close(): void {
    this.stopped = true
    if (this.retryTimer) clearTimeout(this.retryTimer)
    this.retryTimer = null
    const ws = this.ws
    this.ws = null
    ws?.close(1000, 'client closed')
    this.setStatus('closed')
  }

  /** 送出訊息；尚未連線時丟棄並回傳 false（不排隊，避免重連後送出過期的輸入）。 */
  send(msg: ClientMessage): boolean {
    if (!this.ws || this.ws.readyState !== WebSocket.OPEN) return false
    const bytes = toBinary(ClientMessageSchema, msg)
    this.ws.send(bytes)
    this.listener.onSent?.(msg, bytes.byteLength)
    return true
  }

  private open(): void {
    this.attempt += 1
    this.setStatus('connecting')

    const ws = new WebSocket(this.url)
    ws.binaryType = 'arraybuffer'
    this.ws = ws

    ws.onopen = () => {
      if (this.ws !== ws) return
      this.attempt = 0
      this.setStatus('open')
    }

    ws.onmessage = (ev: MessageEvent) => {
      if (this.ws !== ws) return
      if (!(ev.data instanceof ArrayBuffer)) {
        console.warn('[ws] ignoring non-binary message')
        return
      }
      try {
        const msg = fromBinary(ServerMessageSchema, new Uint8Array(ev.data))
        this.listener.onMessage?.(msg, ev.data.byteLength)
      } catch (err) {
        console.warn('[ws] failed to decode message', err)
      }
    }

    // 連線失敗時瀏覽器會先觸發 error 再觸發 close，重連統一在 close 處理。
    ws.onclose = () => {
      if (this.ws !== ws) return
      this.ws = null
      if (this.stopped) {
        this.setStatus('closed')
        return
      }
      this.scheduleReconnect()
    }
  }

  private scheduleReconnect(): void {
    const delay = Math.min(this.maxDelay, this.minDelay * 2 ** Math.max(0, this.attempt - 1))
    this.setStatus('closed', delay)
    this.retryTimer = setTimeout(() => {
      this.retryTimer = null
      if (!this.stopped) this.open()
    }, delay)
  }

  private setStatus(status: ConnectionStatus, retryInMs?: number): void {
    this.status = status
    this.listener.onStatus?.(status, { attempt: this.attempt, retryInMs })
  }
}
