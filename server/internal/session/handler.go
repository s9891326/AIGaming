// Package session 決定每種客戶端訊息要怎麼回應：連線握手、Ping/Pong，
// 以及把加入遊戲、移動輸入轉交給遊戲迴圈。
package session

import (
	"fmt"
	"log/slog"
	"time"

	"aigaming/server/internal/game"
	gamev1 "aigaming/server/internal/pb/game/v1"
	"aigaming/server/internal/ws"
)

// Game 是 session 需要的遊戲操作（由 *game.Loop 實作），以連線 ID 作為玩家的 key。
type Game interface {
	Join(key string, sink game.Sink)
	Leave(key string)
	Input(key string, seq uint32, x, y float64)
}

// Handler 實作 ws.Handler。
type Handler struct {
	log     *slog.Logger
	version string
	game    Game             // 為 nil 時忽略所有遊戲訊息（只測連線層時使用）
	now     func() time.Time // 測試可替換
}

// NewHandler 建立 Handler。
func NewHandler(log *slog.Logger, version string, g Game) *Handler {
	return &Handler{log: log, version: version, game: g, now: time.Now}
}

// OnConnect 送出 Welcome。客戶端收到後再送 JoinRequest 加入遊戲。
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
	case *gamev1.ClientMessage_Join:
		if h.game != nil {
			h.game.Join(c.ID(), c)
		}
	case *gamev1.ClientMessage_Input:
		if h.game != nil {
			in := p.Input
			h.game.Input(c.ID(), in.GetSeq(), float64(in.GetMoveX()), float64(in.GetMoveY()))
		}
	default:
		h.log.Debug("unhandled client message", "conn", c.ID(), "type", fmt.Sprintf("%T", msg.Payload))
	}
}

// OnDisconnect 讓斷線的玩家離開遊戲（重新整理頁面時，舊角色就在這裡被移除）。
func (h *Handler) OnDisconnect(c *ws.Conn) {
	if h.game != nil {
		h.game.Leave(c.ID())
	}
}
