package game

import (
	"context"
	"io"
	"log/slog"
	"sync"
	"testing"
	"time"

	gamev1 "aigaming/server/internal/pb/game/v1"
)

// fakeSink 記錄收到的訊息；Loop 在自己的 goroutine 呼叫 Send，測試在另一個 goroutine 讀取，所以要加鎖。
type fakeSink struct {
	mu   sync.Mutex
	msgs []*gamev1.ServerMessage
}

func (s *fakeSink) Send(m *gamev1.ServerMessage) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.msgs = append(s.msgs, m)
	return nil
}

func (s *fakeSink) all() []*gamev1.ServerMessage {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]*gamev1.ServerMessage(nil), s.msgs...)
}

func (s *fakeSink) lastSnapshot() *gamev1.WorldSnapshot {
	msgs := s.all()
	for i := len(msgs) - 1; i >= 0; i-- {
		if snap := msgs[i].GetSnapshot(); snap != nil {
			return snap
		}
	}
	return nil
}

func startLoop(t *testing.T) *Loop {
	t.Helper()
	cfg := LoopConfig{TickRate: 100, World: WorldConfig{Width: 1000, Height: 600, PlayerSpeed: 100, PlayerRadius: 10}}
	l := NewLoop(slog.New(slog.NewTextHandler(io.Discard, nil)), cfg)
	ctx, cancel := context.WithCancel(context.Background())
	go l.Run(ctx)
	t.Cleanup(func() {
		cancel()
		<-l.done
	})
	return l
}

func eventually(t *testing.T, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for !cond() {
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for %s", what)
		}
		time.Sleep(5 * time.Millisecond)
	}
}

func findPlayer(snap *gamev1.WorldSnapshot, id uint32) *gamev1.PlayerState {
	if snap == nil {
		return nil
	}
	for _, p := range snap.GetPlayers() {
		if p.GetId() == id {
			return p
		}
	}
	return nil
}

func TestJoinSendsAcceptedBeforeSnapshots(t *testing.T) {
	l := startLoop(t)
	sink := &fakeSink{}
	l.Join("conn-a", sink)
	l.Join("conn-a", sink) // 重複加入：忽略

	eventually(t, "a few snapshots", func() bool { return len(sink.all()) >= 3 })

	msgs := sink.all()
	accepted := msgs[0].GetJoinAccepted()
	if accepted == nil {
		t.Fatalf("first message = %v, want JoinAccepted", msgs[0])
	}
	if accepted.GetPlayerId() == 0 || accepted.GetWorldWidth() != 1000 || accepted.GetTickRate() != 100 {
		t.Fatalf("unexpected JoinAccepted: %v", accepted)
	}
	for _, m := range msgs[1:] {
		if m.GetJoinAccepted() != nil {
			t.Fatal("duplicate Join must not send a second JoinAccepted")
		}
		snap := m.GetSnapshot()
		if snap == nil || findPlayer(snap, accepted.GetPlayerId()) == nil {
			t.Fatalf("snapshot should contain own player: %v", m)
		}
	}
}

func TestInputMovesPlayer(t *testing.T) {
	l := startLoop(t)
	sink := &fakeSink{}
	l.Join("conn-a", sink)
	eventually(t, "JoinAccepted", func() bool { return len(sink.all()) > 0 })
	id := sink.all()[0].GetJoinAccepted().GetPlayerId()

	l.Input("conn-a", 1, 1, 0)

	eventually(t, "player moved right with seq acked", func() bool {
		p := findPlayer(sink.lastSnapshot(), id)
		return p != nil && p.GetX() > 520 && p.GetY() == 300 && p.GetLastInputSeq() == 1
	})
}

func TestLeaveRemovesPlayerFromOthersSnapshots(t *testing.T) {
	l := startLoop(t)
	a, b := &fakeSink{}, &fakeSink{}
	l.Join("conn-a", a)
	l.Join("conn-b", b)
	eventually(t, "both see two players", func() bool {
		return len(b.lastSnapshot().GetPlayers()) == 2
	})

	l.Leave("conn-a")
	l.Leave("conn-a")          // 重複離開：忽略
	l.Input("conn-a", 1, 1, 0) // 離開後的輸入：忽略

	eventually(t, "b sees one player", func() bool {
		return len(b.lastSnapshot().GetPlayers()) == 1
	})
}

func TestCommandsAfterStopDoNotBlock(t *testing.T) {
	l := NewLoop(slog.New(slog.NewTextHandler(io.Discard, nil)), DefaultLoopConfig)
	ctx, cancel := context.WithCancel(context.Background())
	go l.Run(ctx)
	cancel()
	<-l.done

	done := make(chan struct{})
	go func() {
		// 指令佇列容量是 256，送超過容量也不能卡住。
		for range 1000 {
			l.Input("x", 1, 1, 0)
		}
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("enqueue blocked after loop stopped")
	}
}
