package game

import (
	"math"
	"testing"
)

const eps = 1e-9

func near(a, b float64) bool { return math.Abs(a-b) < 1e-6 }

func testConfig() WorldConfig {
	return WorldConfig{Width: 1000, Height: 600, PlayerSpeed: 100, PlayerRadius: 10}
}

func TestNormalizeInput(t *testing.T) {
	tests := []struct {
		name         string
		x, y         float64
		wantX, wantY float64
	}{
		{"zero", 0, 0, 0, 0},
		{"right", 1, 0, 1, 0},
		{"diagonal is unit length", 1, 1, 1 / math.Sqrt2, 1 / math.Sqrt2},
		{"too long is clamped", 10, 0, 1, 0},
		{"analog half keeps length", 0.5, 0, 0.5, 0},
		{"NaN becomes zero", math.NaN(), 1, 0, 0},
		{"Inf becomes zero", 1, math.Inf(-1), 0, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := NormalizeInput(tt.x, tt.y)
			if !near(got.X, tt.wantX) || !near(got.Y, tt.wantY) {
				t.Fatalf("NormalizeInput(%v, %v) = %+v, want (%v, %v)", tt.x, tt.y, got, tt.wantX, tt.wantY)
			}
			if got.Len() > 1+eps {
				t.Fatalf("length %v > 1", got.Len())
			}
		})
	}
}

func TestAddPlayerSpawnsAtCenterWithUniqueIDs(t *testing.T) {
	w := NewWorld(testConfig())
	a, b := w.AddPlayer(), w.AddPlayer()
	if a == 0 || b == 0 || a == b {
		t.Fatalf("ids = %d, %d; want unique non-zero", a, b)
	}
	if p := w.Player(a).Pos; p != (Vec2{500, 300}) {
		t.Fatalf("spawn = %+v, want center", p)
	}
}

func TestStepMovesBySpeedTimesDt(t *testing.T) {
	w := NewWorld(testConfig())
	id := w.AddPlayer()
	w.SetInput(id, 1, 1, 0)

	w.Step(0.5) // 100 單位/秒 × 0.5 秒 = 50

	if p := w.Player(id).Pos; !near(p.X, 550) || !near(p.Y, 300) {
		t.Fatalf("pos = %+v, want (550, 300)", p)
	}
	if w.Tick() != 1 {
		t.Fatalf("tick = %d, want 1", w.Tick())
	}
}

func TestDiagonalIsNotFaster(t *testing.T) {
	w := NewWorld(testConfig())
	straight, diagonal := w.AddPlayer(), w.AddPlayer()
	w.SetInput(straight, 1, 1, 0)
	w.SetInput(diagonal, 1, 1, 1)
	start := w.Player(diagonal).Pos

	w.Step(0.5)

	moved := func(id PlayerID) float64 {
		p := w.Player(id).Pos
		return math.Hypot(p.X-start.X, p.Y-start.Y)
	}
	if !near(moved(straight), moved(diagonal)) {
		t.Fatalf("straight moved %v, diagonal moved %v; want equal", moved(straight), moved(diagonal))
	}
}

func TestPlayersStayInsideBounds(t *testing.T) {
	cfg := testConfig()
	dirs := []Vec2{{-1, 0}, {1, 0}, {0, -1}, {0, 1}, {-1, -1}, {1, 1}}
	for _, d := range dirs {
		w := NewWorld(cfg)
		id := w.AddPlayer()
		w.SetInput(id, 1, d.X, d.Y)
		for range 1000 { // 遠超過走到牆邊所需的時間
			w.Step(0.05)
		}
		p := w.Player(id).Pos
		r := cfg.PlayerRadius
		if p.X < r || p.X > cfg.Width-r || p.Y < r || p.Y > cfg.Height-r {
			t.Fatalf("dir %+v: pos %+v left the world (radius %v)", d, p, r)
		}
		// 確實有走到牆邊，而不是卡在中間。
		if d.X < 0 && !near(p.X, r) || d.X > 0 && !near(p.X, cfg.Width-r) {
			t.Fatalf("dir %+v: x = %v, want at wall", d, p.X)
		}
	}
}

func TestSetInputIgnoresUnknownPlayerAndStaleSeq(t *testing.T) {
	w := NewWorld(testConfig())
	id := w.AddPlayer()

	w.SetInput(999, 1, 1, 0) // 不存在的角色：不應 panic

	w.SetInput(id, 5, 1, 0)
	w.SetInput(id, 3, -1, 0) // 舊的序號：忽略
	p := w.Player(id)
	if p.Move != (Vec2{1, 0}) || p.LastInputSeq != 5 {
		t.Fatalf("move = %+v seq = %d; stale input should be ignored", p.Move, p.LastInputSeq)
	}
}

func TestRemovePlayerAndSortedSnapshot(t *testing.T) {
	w := NewWorld(testConfig())
	a, b, c := w.AddPlayer(), w.AddPlayer(), w.AddPlayer()
	w.RemovePlayer(b)
	w.RemovePlayer(12345) // 不存在：不應 panic

	ps := w.Players()
	if len(ps) != 2 || ps[0].ID != a || ps[1].ID != c {
		t.Fatalf("players = %+v, want [%d %d]", ps, a, c)
	}
	// 回傳的是副本，修改不影響世界。
	ps[0].Pos = Vec2{-1, -1}
	if w.Player(a).Pos == (Vec2{-1, -1}) {
		t.Fatal("Players() must return copies")
	}
}
