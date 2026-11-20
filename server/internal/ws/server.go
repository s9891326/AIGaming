// Package ws 處理 WebSocket 連線：升級 HTTP 請求、每條連線的讀寫 goroutine、心跳與連線管理。
// 訊息內容（protobuf）的意義交給 Handler 處理，ws 套件本身不含遊戲邏輯。
package ws

import (
	"crypto/rand"
	"encoding/hex"
	"log/slog"
	"net/http"
	"sync"
	"time"

	"github.com/gorilla/websocket"

	gamev1 "aigaming/server/internal/pb/game/v1"
)

// Handler 接收連線事件。三個方法都在該連線的讀取 goroutine 中被呼叫。
type Handler interface {
	OnConnect(c *Conn)
	OnMessage(c *Conn, msg *gamev1.ClientMessage)
	OnDisconnect(c *Conn)
}

// Server 是 WebSocket 端點（http.Handler），並追蹤目前所有連線。
type Server struct {
	log      *slog.Logger
	handler  Handler
	upgrader websocket.Upgrader

	mu     sync.Mutex
	conns  map[string]*Conn
	closed bool
	wg     sync.WaitGroup
}

// NewServer 建立 WebSocket 伺服器。
//
// Origin 檢查使用 gorilla 預設行為：Origin 的 host 必須等於請求的 Host。
// 開發（Vite）與部署（nginx）都以同源方式代理 /ws，所以其他網站的頁面無法連進來。
func NewServer(log *slog.Logger, h Handler) *Server {
	return &Server{
		log:     log,
		handler: h,
		upgrader: websocket.Upgrader{
			ReadBufferSize:  1024,
			WriteBufferSize: 4096,
		},
		conns: make(map[string]*Conn),
	}
}

// ServeHTTP 把 HTTP 請求升級為 WebSocket，並啟動該連線的讀寫 goroutine。
func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	wsConn, err := s.upgrader.Upgrade(w, r, nil)
	if err != nil {
		// Upgrade 失敗時已經回應 HTTP 錯誤。
		s.log.Warn("websocket upgrade failed", "err", err, "origin", r.Header.Get("Origin"))
		return
	}

	id := newID()
	c := &Conn{
		id:   id,
		ws:   wsConn,
		log:  s.log.With("conn", id),
		send: make(chan []byte, sendBuffer),
		done: make(chan struct{}),
	}

	n, ok := s.add(c)
	if !ok {
		// 伺服器正在關閉：writePump 還沒啟動，直接在這裡送出 close 訊框並關閉。
		_ = wsConn.WriteControl(websocket.CloseMessage,
			websocket.FormatCloseMessage(websocket.CloseGoingAway, "server closing"),
			time.Now().Add(time.Second))
		_ = wsConn.Close()
		return
	}
	c.log.Info("client connected", "ip", r.RemoteAddr, "connections", n)

	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
		c.writePump()
	}()

	s.handler.OnConnect(c)
	c.readPump(s.handler) // 在這個 goroutine 裡阻塞直到斷線
	s.handler.OnDisconnect(c)

	n = s.remove(c)
	c.log.Info("client disconnected", "connections", n)
}

// Count 回傳目前連線數。
func (s *Server) Count() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.conns)
}

// Close 關閉所有連線並等待寫入 goroutine 結束；之後的新連線會被拒絕。
// http.Server.Shutdown 不會關閉已升級（hijack）的連線，所以關機時要另外呼叫。
func (s *Server) Close() {
	s.mu.Lock()
	s.closed = true
	conns := make([]*Conn, 0, len(s.conns))
	for _, c := range s.conns {
		conns = append(conns, c)
	}
	s.mu.Unlock()

	for _, c := range conns {
		c.Close()
	}
	s.wg.Wait()
}

func (s *Server) add(c *Conn) (int, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return 0, false
	}
	s.conns[c.id] = c
	return len(s.conns), true
}

func (s *Server) remove(c *Conn) int {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.conns, c.id)
	return len(s.conns)
}

func newID() string {
	var b [4]byte
	_, _ = rand.Read(b[:])
	return hex.EncodeToString(b[:])
}
