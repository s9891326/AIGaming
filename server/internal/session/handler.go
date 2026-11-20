// Package session 決定每種客戶端訊息要怎麼回應。目前只有連線握手與 Ping/Pong，
// 之後的遊戲邏輯（加入遊戲、輸入）也會從這裡分派出去。
package session

import (
	"fmt"
	"log/slog"
	"time"

	gamev1 "aigaming/server/internal/pb/game/v1"
	"aigaming/server/internal/ws"
)

// Handler 實作 ws.Handler。
type Handler struct {
	log     *slog.Logger
	version string
	now     func() time.Time // 測試可替換
}

// NewHandler 建立 Handler。
func NewHandler(log *slog.Logger, version string) *Handler {
	return &Handler{log: log, version: version, now: time.Now}
}

// OnConnect 送出 Welcome。
func (h *Handler) OnConnect(c *ws.Conn) {
	_ = c.Send(&gamev1.ServerMessage{
		Payload: &gamev1.ServerMessage_Welcome{Welcome: &gamev1.Welcome{
			ConnectionId:  c.ID(),
			ServerVersion: h.version,
		}},
	})
}

// OnMessage 依訊息種類分派。
func (h *Handler) OnMessage(c *ws.Conn, msg *gamev1.ClientMessage) {
	switch p := msg.Payload.(type) {
	case *gamev1.ClientMessage_Ping:
		_ = c.Send(&gamev1.ServerMessage{
			Payload: &gamev1.ServerMessage_Pong{Pong: &gamev1.Pong{
				ClientTimeMs: p.Ping.GetClientTimeMs(),
				ServerTimeMs: float64(h.now().UnixMilli()),
			}},
		})
	default:
		h.log.Debug("unhandled client message", "conn", c.ID(), "type", fmt.Sprintf("%T", msg.Payload))
	}
}

// OnDisconnect 目前沒有需要清理的狀態。
func (h *Handler) OnDisconnect(*ws.Conn) {}
