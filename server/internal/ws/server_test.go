package ws_test

import (
	"bytes"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"google.golang.org/protobuf/proto"

	gamev1 "aigaming/server/internal/pb/game/v1"
	"aigaming/server/internal/session"
	"aigaming/server/internal/ws"
	"aigaming/server/pkg/logger"
)

type testEnv struct {
	srv  *ws.Server
	http *httptest.Server
	url  string
	logs *bytes.Buffer
}

func newEnv(t *testing.T) *testEnv {
	t.Helper()
	var logs bytes.Buffer
	log := logger.New(logger.Options{Writer: &logs, Level: slog.LevelDebug, Color: logger.ColorNever})
	srv := ws.NewServer(log, session.NewHandler(log, "test"))
	hs := httptest.NewServer(srv)
	t.Cleanup(func() {
		srv.Close()
		hs.Close()
	})
	return &testEnv{srv: srv, http: hs, url: "ws" + strings.TrimPrefix(hs.URL, "http"), logs: &logs}
}

func (e *testEnv) dial(t *testing.T) *websocket.Conn {
	t.Helper()
	c, resp, err := websocket.DefaultDialer.Dial(e.url, nil)
	if err != nil {
		t.Fatalf("dial: %v (resp=%v)", err, resp)
	}
	t.Cleanup(func() { _ = c.Close() })
	return c
}

func readServerMsg(t *testing.T, c *websocket.Conn) *gamev1.ServerMessage {
	t.Helper()
	_ = c.SetReadDeadline(time.Now().Add(2 * time.Second))
	typ, data, err := c.ReadMessage()
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if typ != websocket.BinaryMessage {
		t.Fatalf("message type = %d, want binary", typ)
	}
	var msg gamev1.ServerMessage
	if err := proto.Unmarshal(data, &msg); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	return &msg
}

func sendPing(t *testing.T, c *websocket.Conn, clientTime float64) {
	t.Helper()
	b, err := proto.Marshal(&gamev1.ClientMessage{
		Payload: &gamev1.ClientMessage_Ping{Ping: &gamev1.Ping{ClientTimeMs: clientTime}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := c.WriteMessage(websocket.BinaryMessage, b); err != nil {
		t.Fatalf("write: %v", err)
	}
}

// waitFor 輪詢條件直到成立或逾時（連線計數在另一個 goroutine 更新）。
func waitFor(t *testing.T, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for !cond() {
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for %s", what)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func TestWelcomeThenPingPong(t *testing.T) {
	env := newEnv(t)
	c := env.dial(t)

	welcome := readServerMsg(t, c).GetWelcome()
	if welcome == nil || welcome.GetConnectionId() == "" || welcome.GetServerVersion() != "test" {
		t.Fatalf("want Welcome with id and version, got %v", welcome)
	}

	before := float64(time.Now().UnixMilli())
	sendPing(t, c, 12345.5)

	pong := readServerMsg(t, c).GetPong()
	if pong == nil {
		t.Fatal("want Pong")
	}
	if pong.GetClientTimeMs() != 12345.5 {
		t.Errorf("client_time_ms = %v, want echo 12345.5", pong.GetClientTimeMs())
	}
	if pong.GetServerTimeMs() < before {
		t.Errorf("server_time_ms = %v, want >= %v", pong.GetServerTimeMs(), before)
	}
}

func TestMalformedMessagesAreIgnored(t *testing.T) {
	env := newEnv(t)
	c := env.dial(t)
	readServerMsg(t, c) // Welcome

	// 壞掉的 protobuf 與文字訊框都應該被忽略，連線保持可用。
	if err := c.WriteMessage(websocket.BinaryMessage, []byte{0xff, 0xff, 0xff}); err != nil {
		t.Fatal(err)
	}
	if err := c.WriteMessage(websocket.TextMessage, []byte("hello")); err != nil {
		t.Fatal(err)
	}
	sendPing(t, c, 1)

	if readServerMsg(t, c).GetPong() == nil {
		t.Fatal("connection should still answer Ping after bad messages")
	}
	logs := env.logs.String()
	if !strings.Contains(logs, "ignoring malformed message") || !strings.Contains(logs, "ignoring non-binary message") {
		t.Errorf("want warnings for bad messages, got logs:\n%s", logs)
	}
}

func TestCrossOriginRejected(t *testing.T) {
	env := newEnv(t)

	header := http.Header{"Origin": []string{"http://evil.example"}}
	_, resp, err := websocket.DefaultDialer.Dial(env.url, header)
	if err == nil {
		t.Fatal("cross-origin dial should fail")
	}
	if resp == nil || resp.StatusCode != http.StatusForbidden {
		t.Fatalf("want 403, got %v", resp)
	}
}

func TestConnectionCountAndLogs(t *testing.T) {
	env := newEnv(t)

	c1 := env.dial(t)
	c2 := env.dial(t)
	readServerMsg(t, c1)
	readServerMsg(t, c2)
	waitFor(t, "2 connections", func() bool { return env.srv.Count() == 2 })

	_ = c1.WriteMessage(websocket.CloseMessage, websocket.FormatCloseMessage(websocket.CloseNormalClosure, ""))
	_ = c1.Close()
	waitFor(t, "1 connection", func() bool { return env.srv.Count() == 1 })

	logs := env.logs.String()
	if !strings.Contains(logs, "client connected") || !strings.Contains(logs, "client disconnected") {
		t.Errorf("want connect/disconnect logs, got:\n%s", logs)
	}
}

func TestServerCloseDisconnectsClients(t *testing.T) {
	env := newEnv(t)
	c := env.dial(t)
	readServerMsg(t, c)

	env.srv.Close()

	_ = c.SetReadDeadline(time.Now().Add(2 * time.Second))
	_, _, err := c.ReadMessage()
	if !websocket.IsCloseError(err, websocket.CloseGoingAway) {
		t.Fatalf("want close 1001 (going away), got %v", err)
	}
	waitFor(t, "0 connections", func() bool { return env.srv.Count() == 0 })

	// 關閉後的新連線會立刻收到 close 1001，而不是等到逾時。
	c2, _, err := websocket.DefaultDialer.Dial(env.url, nil)
	if err == nil {
		defer c2.Close()
		_ = c2.SetReadDeadline(time.Now().Add(500 * time.Millisecond))
		if _, _, err := c2.ReadMessage(); !websocket.IsCloseError(err, websocket.CloseGoingAway) {
			t.Fatalf("new connection after Close: want close 1001, got %v", err)
		}
	}
}
