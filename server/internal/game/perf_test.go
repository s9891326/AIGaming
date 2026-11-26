package game

import (
	"fmt"
	"math"
	"math/rand/v2"
	"testing"

	"google.golang.org/protobuf/proto"

	gamev1 "aigaming/server/internal/pb/game/v1"
)

// perfScenario 描述效能量測用的場景。
type perfScenario struct {
	name    string
	enemies int
	spread  float64 // 敵人分布在玩家周圍多大的半徑；0 = 均勻散布整張地圖
}

var perfScenarios = []perfScenario{
	{"300 聚集", 300, 500},
	{"1000 聚集", 1000, 700},
	{"1000 散布", 1000, 0},
	{"2000 散布", 2000, 0},
}

// perfWorld 建立一個固定的高負載世界：一位玩家帶三把強化過的武器、n 隻不會死的敵人（各種類混合）、
// 不自動生成，讓每個 tick 的工作量固定。
func perfWorld(sc perfScenario) (*World, *Player) {
	cfg := DefaultWorldConfig
	cfg.Seed = 1
	cfg.InputDt = 0.05
	cfg.Combat.Waves = nil
	cfg.Combat.SpawnRateBase, cfg.Combat.SpawnRateGrowth = 0, 0
	cfg.Combat.MaxEnemies = 1 << 30
	cfg.Combat.SpawnProtection = 1e18 // 玩家不會死
	w := NewWorld(cfg)
	p := w.Player(w.AddPlayer())
	bolt := NewMagicBolt(DefaultMagicBolt)
	bolt.cfg.Count, bolt.cfg.Pierce = 4, 3
	orbit := NewOrbit(DefaultOrbit)
	orbit.cfg.Count = 6
	p.Weapons = []Weapon{bolt, NewAura(DefaultAura), orbit}

	rng := rand.New(rand.NewPCG(42, 42))
	kinds := []EnemyKind{KindGrunt, KindRunner, KindBrute, KindElite}
	for i := range sc.enemies {
		def := DefaultWaves.Kinds[kinds[i%len(kinds)]]
		def.HP = 1 << 30 // 不會死，數量固定
		var pos Vec2
		if sc.spread > 0 {
			a, r := rng.Float64()*2*math.Pi, math.Sqrt(rng.Float64())*sc.spread
			pos = Vec2{p.Pos.X + math.Cos(a)*r, p.Pos.Y + math.Sin(a)*r}
		} else {
			pos = Vec2{rng.Float64() * cfg.Width, rng.Float64() * cfg.Height}
		}
		e := w.spawnEnemy(def, p.Pos)
		e.Pos = w.clampToWorld(pos, e.Radius)
	}
	for range 40 { // 讓分離、投射物進入穩定狀態
		w.Step(0.05)
	}
	return w, p
}

// BenchmarkStep 量測一個 tick 的模擬耗時（不含網路）。
//
//	go test -bench Step -run ^$ ./internal/game/
func BenchmarkStep(b *testing.B) {
	for _, sc := range perfScenarios {
		b.Run(sc.name, func(b *testing.B) {
			w, _ := perfWorld(sc)
			b.ResetTimer()
			for range b.N {
				w.Step(0.05)
			}
		})
	}
}

// BenchmarkSnapshot 量測組裝並編碼一份快照的耗時（每位玩家每個 tick 一次）。
func BenchmarkSnapshot(b *testing.B) {
	for _, sc := range perfScenarios {
		b.Run(sc.name, func(b *testing.B) {
			w, p := perfWorld(sc)
			b.ResetTimer()
			for range b.N {
				_, _ = proto.Marshal(snapshotMessage(w, p))
			}
		})
	}
}

// snapshotMessage 回傳某位玩家會收到的快照訊息。
func snapshotMessage(w *World, viewer *Player) *gamev1.ServerMessage {
	b := newSnapshotBuilder(w, DefaultAOI, 1.8e12)
	return &gamev1.ServerMessage{Payload: &gamev1.ServerMessage_Snapshot{Snapshot: b.For(viewer.Pos)}}
}

// TestPerfReport 印出各場景的快照大小與實體數（go test -run PerfReport -v）。
func TestPerfReport(t *testing.T) {
	if testing.Short() {
		t.Skip("report")
	}
	for _, sc := range perfScenarios {
		w, p := perfWorld(sc)
		msg := snapshotMessage(w, p)
		b, err := proto.Marshal(msg)
		if err != nil {
			t.Fatal(err)
		}
		s := msg.GetSnapshot()
		t.Logf("%-10s snapshot %6d bytes (%5.1f KB/s at 20Hz)  enemies %4d  projectiles %3d  gems %3d",
			sc.name, len(b), float64(len(b))*20/1024, len(s.GetEnemies()), len(s.GetProjectiles()), len(s.GetGems()))
	}
	fmt.Print("")
}
