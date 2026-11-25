package ws

import (
	"errors"
	"log/slog"
	"sync"
	"time"

	"github.com/gorilla/websocket"

	gamev1 "aigaming/server/internal/pb/game/v1"
)

const (
	// writeWait 是單次寫入允許的最長時間。
	writeWait = 10 * time.Second
	// pongWait 是等待客戶端回應心跳的最長時間；超過就視為斷線。
	pongWait = 60 * time.Second
	// pingPeriod 是伺服器送出 WebSocket ping 的週期，必須小於 pongWait。
	pingPeriod = pongWait * 9 / 10
	// maxMessageSize 是單一客戶端訊息的上限（位元組），客戶端只送小訊息。
	maxMessageSize = 4 << 10
	// sendBuffer 是每條連線待送訊息的緩衝數量；塞滿代表客戶端太慢，直接斷線。
	sendBuffer = 64
)

// ErrSendBufferFull 表示連線的待送佇列已滿。
var ErrSendBufferFull = errors.New("ws: send buffer full")

// Conn 是一條 WebSocket 連線。
//
// gorilla/websocket 同時只允許一個 goroutine 讀、一個 goroutine 寫，所以：
//   - readPump：唯一的讀取者，解碼 ClientMessage 後交給 Handler。
//   - writePump：唯一的寫入者，從 send channel 取出訊息寫出，並定期送心跳 ping。
//
// 其他 goroutine 一律透過 Send 把訊息放進 send channel，不直接碰底層連線。
type Conn struct {
	id     string
	ws     *websocket.Conn
	log    *slog.Logger
	format Format // 訊息編碼（protobuf / json），連線建立時決定
	send   chan []byte

	closeOnce sync.Once
	done      chan struct{}
}

// ID 回傳連線識別碼。
func (c *Conn) ID() string { return c.id }

// Format 回傳這條連線使用的訊息編碼。
func (c *Conn) Format() Format { return c.format }

// Send 把訊息編碼後排入待送佇列，不會阻塞。
// 佇列已滿時關閉連線並回傳 ErrSendBufferFull（寧可踢掉慢的客戶端，也不拖慢遊戲迴圈）。
func (c *Conn) Send(msg *gamev1.ServerMessage) error {
	b, err := c.format.encode(msg)
	if err != nil {
		return err
	}
	select {
	case <-c.done:
		return websocket.ErrCloseSent
	default:
	}
	select {
	case c.send <- b:
		return nil
	default:
		c.log.Warn("send buffer full, closing connection")
		c.Close()
		return ErrSendBufferFull
	}
}

// Close 要求關閉連線，可重複呼叫、任何 goroutine 都能呼叫。
// 實際關閉由 writePump 負責：先送出 close 訊框再關閉底層連線，readPump 隨後會讀到錯誤而結束。
func (c *Conn) Close() {
	c.closeOnce.Do(func() { close(c.done) })
}

func (c *Conn) readPump(h Handler) {
	defer c.Close()

	c.ws.SetReadLimit(maxMessageSize)
	_ = c.ws.SetReadDeadline(time.Now().Add(pongWait))
	c.ws.SetPongHandler(func(string) error {
		return c.ws.SetReadDeadline(time.Now().Add(pongWait))
	})

	for {
		typ, data, err := c.ws.ReadMessage()
		if err != nil {
			// 1006（abnormal closure）在瀏覽器很常見：關分頁、網路中斷、React StrictMode 重複掛載，
			// 斷線本身已有 "client disconnected" 日誌，所以只有其他非預期的錯誤才記 Warn。
			if websocket.IsUnexpectedCloseError(err, websocket.CloseNormalClosure, websocket.CloseGoingAway,
				websocket.CloseNoStatusReceived, websocket.CloseAbnormalClosure) {
				c.log.Warn("read error", "err", err)
			} else {
				c.log.Debug("connection closed", "reason", err)
			}
			return
		}
		// 收到任何訊息都代表連線還活著。
		_ = c.ws.SetReadDeadline(time.Now().Add(pongWait))

		if typ != c.format.frameType() {
			c.log.Warn("ignoring message with wrong frame type", "type", typ, "format", c.format)
			continue
		}
		var msg gamev1.ClientMessage
		if err := c.format.decode(data, &msg); err != nil {
			c.log.Warn("ignoring malformed message", "err", err, "bytes", len(data))
			continue
		}
		h.OnMessage(c, &msg)
	}
}

func (c *Conn) writePump() {
	ticker := time.NewTicker(pingPeriod)
	defer func() {
		ticker.Stop()
		c.Close()
		_ = c.ws.Close() // 唯一關閉底層連線的地方
	}()

	for {
		select {
		case b := <-c.send:
			_ = c.ws.SetWriteDeadline(time.Now().Add(writeWait))
			if err := c.ws.WriteMessage(c.format.frameType(), b); err != nil {
				return
			}
		case <-ticker.C:
			_ = c.ws.SetWriteDeadline(time.Now().Add(writeWait))
			if err := c.ws.WriteMessage(websocket.PingMessage, nil); err != nil {
				return
			}
		case <-c.done:
			// 盡力送出關閉訊框，讓客戶端知道是正常關閉。
			_ = c.ws.WriteControl(websocket.CloseMessage,
				websocket.FormatCloseMessage(websocket.CloseGoingAway, "server closing"),
				time.Now().Add(time.Second))
			return
		}
	}
}
