package ws_test

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"testing"
	"time"

	"github.com/gorilla/websocket"

	"aigaming/server/internal/game"
	"aigaming/server/internal/ws"
)

// 這些測試模擬 Postman：不帶 Origin 標頭、用 text frame 收發 JSON。

func dialJSON(t *testing.T, env *testEnv) *websocket.Conn {
	t.Helper()
	c, resp, err := websocket.DefaultDialer.Dial(env.url+"?format=json", nil)
	if err != nil {
		t.Fatalf("dial json: %v (resp=%v)", err, resp)
	}
	t.Cleanup(func() { _ = c.Close() })
	return c
}

// readJSON 讀一則 text frame 並解析成 map；skip 為 true 的訊息（例如快照）會被跳過。
func readJSON(t *testing.T, c *websocket.Conn, skip func(map[string]any) bool) map[string]any {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for {
		_ = c.SetReadDeadline(deadline)
		typ, data, err := c.ReadMessage()
		if err != nil {
			t.Fatalf("read: %v", err)
		}
		if typ != websocket.TextMessage {
			t.Fatalf("frame type = %d, want text", typ)
		}
		var m map[string]any
		if err := json.Unmarshal(data, &m); err != nil {
			t.Fatalf("server sent invalid JSON %q: %v", data, err)
		}
		if skip == nil || !skip(m) {
			return m
		}
	}
}

func sendText(t *testing.T, c *websocket.Conn, s string) {
	t.Helper()
	if err := c.WriteMessage(websocket.TextMessage, []byte(s)); err != nil {
		t.Fatalf("write: %v", err)
	}
}

func isSnapshot(m map[string]any) bool { _, ok := m["snapshot"]; return ok }

func TestJSONPingPong(t *testing.T) {
	env := newEnvWith(t, ws.Options{AllowJSON: true}, nil)
	c := dialJSON(t, env)

	welcome, ok := readJSON(t, c, nil)["welcome"].(map[string]any)
	if !ok || welcome["connectionId"] == "" || welcome["serverVersion"] != "test" {
		t.Fatalf("want JSON welcome, got %v", welcome)
	}

	sendText(t, c, `{"ping":{"clientTimeMs":1000}}`)
	pong, ok := readJSON(t, c, nil)["pong"].(map[string]any)
	if !ok || pong["clientTimeMs"] != float64(1000) || pong["serverTimeMs"].(float64) <= 0 {
		t.Fatalf("want pong echoing 1000, got %v", pong)
	}
}

func TestJSONJoinInputSnapshot(t *testing.T) {
	loop := game.NewLoop(slog.New(slog.NewTextHandler(io.Discard, nil)), game.DefaultLoopConfig)
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	go loop.Run(ctx)

	env := newEnvWith(t, ws.Options{AllowJSON: true}, loop)
	c := dialJSON(t, env)
	readJSON(t, c, nil) // welcome

	sendText(t, c, `{"join":{}}`)
	accepted, ok := readJSON(t, c, isSnapshot)["joinAccepted"].(map[string]any)
	if !ok {
		t.Fatalf("want joinAccepted, got %v", accepted)
	}
	me := accepted["playerId"]

	findMe := func(m map[string]any) map[string]any {
		for _, p := range m["snapshot"].(map[string]any)["players"].([]any) {
			if p := p.(map[string]any); p["id"] == me {
				return p
			}
		}
		return nil
	}
	notSnapshot := func(m map[string]any) bool { return !isSnapshot(m) }

	first := findMe(readJSON(t, c, notSnapshot))
	// 預設值也會輸出：剛加入時 lastInputSeq 是 0，而不是欄位不存在。
	if first == nil || first["lastInputSeq"] != float64(0) {
		t.Fatalf("want own player with lastInputSeq 0, got %v", first)
	}

	sendText(t, c, `{"input":{"seq":1,"moveX":1}}`)
	for {
		p := findMe(readJSON(t, c, notSnapshot))
		if p["lastInputSeq"] == float64(1) {
			if p["x"].(float64) <= first["x"].(float64) {
				t.Fatalf("x did not increase: %v -> %v", first["x"], p["x"])
			}
			return
		}
	}
}

func TestJSONMalformedAndWrongFrameAreIgnored(t *testing.T) {
	env := newEnvWith(t, ws.Options{AllowJSON: true}, nil)
	c := dialJSON(t, env)
	readJSON(t, c, nil) // welcome

	sendText(t, c, `{"ping":`)            // 壞掉的 JSON
	sendText(t, c, `{"unknownField":{}}`) // 不存在的欄位
	if err := c.WriteMessage(websocket.BinaryMessage, []byte{0x12, 0x00}); err != nil {
		t.Fatal(err) // JSON 連線送 binary frame
	}
	sendText(t, c, `{"ping":{"clientTimeMs":7}}`)

	if pong, ok := readJSON(t, c, nil)["pong"].(map[string]any); !ok || pong["clientTimeMs"] != float64(7) {
		t.Fatalf("connection should survive bad messages, got %v", pong)
	}
	env.waitForLog(t, "ignoring malformed message")
	env.waitForLog(t, "ignoring message with wrong frame type")
}

func TestJSONRejectedWhenDisabled(t *testing.T) {
	env := newEnvWith(t, ws.Options{AllowJSON: false}, nil)
	_, resp, err := websocket.DefaultDialer.Dial(env.url+"?format=json", nil)
	if err == nil || resp == nil || resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("want 400 when json disabled, got err=%v resp=%v", err, resp)
	}
	env.waitForLog(t, "format=json is disabled")

	// 預設的 protobuf 連線不受影響。
	c := env.dial(t)
	if readServerMsg(t, c).GetWelcome() == nil {
		t.Fatal("protobuf connection should still work")
	}
}

func TestUnsupportedFormatRejected(t *testing.T) {
	env := newEnvWith(t, ws.Options{AllowJSON: true}, nil)
	_, resp, err := websocket.DefaultDialer.Dial(env.url+"?format=xml", nil)
	if err == nil || resp == nil || resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("want 400 for unsupported format, got err=%v resp=%v", err, resp)
	}
}

func TestParseFormat(t *testing.T) {
	for in, want := range map[string]ws.Format{"": ws.FormatProtobuf, "protobuf": ws.FormatProtobuf, "json": ws.FormatJSON} {
		if got, err := ws.ParseFormat(in); err != nil || got != want {
			t.Errorf("ParseFormat(%q) = %v, %v; want %v", in, got, err, want)
		}
	}
	if _, err := ws.ParseFormat("JSON"); err == nil {
		t.Error("format is case-sensitive; JSON should be rejected")
	}
}
