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

	// 每筆輸入移動一個 tick：速度 100 × 0.01 秒 = 1 單位；10 筆在預算（5 個 tick）內分兩批送。
	for seq := uint32(1); seq <= 10; seq++ {
		l.Input("conn-a", seq, 1, 0)
		if seq == 5 {
			time.Sleep(80 * time.Millisecond) // 讓預算回補
		}
	}

	eventually(t, "player moved 10 units right with seq acked", func() bool {
		p := findPlayer(sink.lastSnapshot(), id)
		return p != nil && p.GetX() == 510 && p.GetY() == 300 && p.GetLastInputSeq() == 10
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

func TestDeathSendsGameOverAndAllowsRejoin(t *testing.T) {
	// 很快就會死的設定：敵人生成在旁邊、一碰就死、沒有保護時間。
	cfg := LoopConfig{TickRate: 100, World: WorldConfig{
		Width: 1000, Height: 600, PlayerSpeed: 100, PlayerRadius: 10,
		Combat: CombatConfig{
			PlayerMaxHP: 10, ContactDamage: 10, HurtInvuln: 0.5,
			EnemySpeed: 500, EnemyRadius: 10, EnemyHP: 20, MaxEnemies: 10,
			SpawnRateBase: 50, SpawnRateMax: 50, SpawnDistance: 50,
		},
	}}
	l := NewLoop(slog.New(slog.NewTextHandler(io.Discard, nil)), cfg)
	ctx, cancel := context.WithCancel(context.Background())
	go l.Run(ctx)
	t.Cleanup(func() { cancel(); <-l.done })

	sink := &fakeSink{}
	l.Join("conn-a", sink)

	var over *gamev1.GameOver
	eventually(t, "GameOver", func() bool {
		for _, m := range sink.all() {
			if over = m.GetGameOver(); over != nil {
				return true
			}
		}
		return false
	})
	if over.GetSurvivedMs() == 0 || over.GetSurvivedMs() > 2000 || over.GetKills() != 0 {
		t.Fatalf("unexpected GameOver: %v", over)
	}

	// 死亡後不再收到快照。
	n := len(sink.all())
	time.Sleep(50 * time.Millisecond)
	if got := len(sink.all()); got != n {
		t.Fatalf("received %d more messages after GameOver, want 0", got-n)
	}

	// 「再來一局」：同一條連線重新 Join，得到新角色。
	firstID := sink.all()[0].GetJoinAccepted().GetPlayerId()
	l.Join("conn-a", sink)
	eventually(t, "second JoinAccepted", func() bool {
		msgs := sink.all()
		for _, m := range msgs[n:] {
			if a := m.GetJoinAccepted(); a != nil {
				if a.GetPlayerId() == firstID {
					t.Fatalf("rejoin reused player id %d", firstID)
				}
				return true
			}
		}
		return false
	})
}

func TestSnapshotIncludesEnemiesAndHP(t *testing.T) {
	cfg := LoopConfig{TickRate: 100, World: WorldConfig{
		Width: 3000, Height: 2000, PlayerSpeed: 100, PlayerRadius: 10,
		Combat: CombatConfig{
			PlayerMaxHP: 100, SpawnProtection: 100, EnemyRadius: 10, EnemyHP: 20, MaxEnemies: 10,
			SpawnRateBase: 50, SpawnRateMax: 50, SpawnDistance: 600,
		},
		Loot: DefaultLootConfig,
	}}
	l := NewLoop(slog.New(slog.NewTextHandler(io.Discard, nil)), cfg)
	ctx, cancel := context.WithCancel(context.Background())
	go l.Run(ctx)
	t.Cleanup(func() { cancel(); <-l.done })

	sink := &fakeSink{}
	l.Join("conn-a", sink)
	eventually(t, "snapshot with enemies", func() bool {
		s := sink.lastSnapshot()
		return s != nil && len(s.GetEnemies()) > 0
	})
	s := sink.lastSnapshot()
	if p := s.GetPlayers()[0]; p.GetHp() != 100 || p.GetMaxHp() != 100 || p.GetLevel() != 1 || p.GetXpToNext() != 5 {
		t.Fatalf("player = %v, want hp 100/100, level 1, xp_to_next 5", p)
	}
	if e := s.GetEnemies()[0]; e.GetHp() != 20 || e.GetMaxHp() != 20 {
		t.Fatalf("enemy hp = %d/%d, want 20/20", e.GetHp(), e.GetMaxHp())
	}
	if a := sink.all()[0].GetJoinAccepted(); a.GetEnemyRadius() != 10 {
		t.Fatalf("JoinAccepted.enemy_radius = %v, want 10", a.GetEnemyRadius())
	}
}

func TestSnapshotIncludesProjectilesGemsAndKills(t *testing.T) {
	cfg := LoopConfig{TickRate: 100, World: WorldConfig{
		Width: 3000, Height: 2000, PlayerSpeed: 100, PlayerRadius: 10,
		Combat: CombatConfig{
			PlayerMaxHP: 100, SpawnProtection: 100, EnemySpeed: 0, EnemyRadius: 10, EnemyHP: 20,
			MaxEnemies: 5, SpawnRateBase: 50, SpawnRateMax: 50, SpawnDistance: 300,
		},
		Loot:            LootConfig{GemValue: 1, GemRadius: 7, MagnetRadius: 0, MaxGems: 100, XPPerLevel: 5},
		StartingWeapons: DefaultWorldConfig.StartingWeapons,
	}}
	l := NewLoop(slog.New(slog.NewTextHandler(io.Discard, nil)), cfg)
	ctx, cancel := context.WithCancel(context.Background())
	go l.Run(ctx)
	t.Cleanup(func() { cancel(); <-l.done })

	sink := &fakeSink{}
	l.Join("conn-a", sink)

	sawProjectile := false
	eventually(t, "a kill with a gem on the ground", func() bool {
		s := sink.lastSnapshot()
		if s == nil {
			return false
		}
		sawProjectile = sawProjectile || len(s.GetProjectiles()) > 0
		return s.GetPlayers()[0].GetKills() > 0 && len(s.GetGems()) > 0
	})
	if !sawProjectile {
		t.Error("never saw a projectile in snapshots")
	}
	if g := sink.lastSnapshot().GetGems()[0]; g.GetValue() != 1 {
		t.Fatalf("gem value = %d, want 1", g.GetValue())
	}
}
