package game

import (
	"math"
	"testing"
)

const eps = 1e-9

func near(a, b float64) bool { return math.Abs(a-b) < 1e-6 }

func testConfig() WorldConfig {
	return WorldConfig{Width: 1000, Height: 600, PlayerSpeed: 100, PlayerRadius: 10, InputDt: 0.05, MaxInputBudget: 0.25}
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

// applyN 送 n 筆同方向的輸入，每筆之間推進一個 tick（模擬正常速度的客戶端）。
func applyN(w *World, id PlayerID, n int, x, y float64) {
	seq := w.Player(id).LastInputSeq
	for range n {
		seq++
		w.ApplyInput(id, seq, x, y)
		w.Step(w.cfg.InputDt)
	}
}

func TestEachInputMovesOneInputDt(t *testing.T) {
	w := NewWorld(testConfig())
	id := w.AddPlayer()

	if r := w.ApplyInput(id, 1, 1, 0); r != InputApplied {
		t.Fatalf("result = %v, want applied", r)
	}
	// 100 單位/秒 × 0.05 秒 = 5
	if p := w.Player(id).Pos; !near(p.X, 505) || !near(p.Y, 300) {
		t.Fatalf("pos = %+v, want (505, 300)", p)
	}

	applyN(w, id, 10, 1, 0)
	if p := w.Player(id).Pos; !near(p.X, 555) {
		t.Fatalf("after 11 inputs x = %v, want 555", p.X)
	}
	if w.Player(id).LastInputSeq != 11 {
		t.Fatalf("last seq = %d, want 11", w.Player(id).LastInputSeq)
	}
}

func TestStepAloneDoesNotMove(t *testing.T) {
	w := NewWorld(testConfig())
	id := w.AddPlayer()
	w.ApplyInput(id, 1, 1, 0)
	before := w.Player(id).Pos
	for range 20 {
		w.Step(0.05)
	}
	if w.Player(id).Pos != before {
		t.Fatal("without inputs the player must not move (movement is input-driven)")
	}
	if w.Tick() != 20 {
		t.Fatalf("tick = %d, want 20", w.Tick())
	}
}

func TestDiagonalIsNotFaster(t *testing.T) {
	w := NewWorld(testConfig())
	straight, diagonal := w.AddPlayer(), w.AddPlayer()
	start := w.Player(diagonal).Pos
	applyN(w, straight, 5, 1, 0)
	applyN(w, diagonal, 5, 1, 1)

	moved := func(id PlayerID) float64 {
		p := w.Player(id).Pos
		return math.Hypot(p.X-start.X, p.Y-start.Y)
	}
	if !near(moved(straight), 25) || !near(moved(diagonal), 25) {
		t.Fatalf("straight moved %v, diagonal moved %v; want both 25", moved(straight), moved(diagonal))
	}
}

func TestPlayersStayInsideBounds(t *testing.T) {
	cfg := testConfig()
	dirs := []Vec2{{-1, 0}, {1, 0}, {0, -1}, {0, 1}, {-1, -1}, {1, 1}}
	for _, d := range dirs {
		w := NewWorld(cfg)
		id := w.AddPlayer()
		applyN(w, id, 400, d.X, d.Y) // 遠超過走到牆邊所需的量
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

func TestApplyInputRejectsUnknownAndStale(t *testing.T) {
	w := NewWorld(testConfig())
	id := w.AddPlayer()

	if r := w.ApplyInput(999, 1, 1, 0); r != InputUnknown {
		t.Fatalf("unknown player: %v", r)
	}
	w.ApplyInput(id, 5, 1, 0)
	pos := w.Player(id).Pos
	for _, seq := range []uint32{5, 3} {
		if r := w.ApplyInput(id, seq, 1, 0); r != InputStale {
			t.Fatalf("seq %d: result %v, want stale", seq, r)
		}
	}
	if w.Player(id).Pos != pos || w.Player(id).LastInputSeq != 5 {
		t.Fatal("stale input must not move the player")
	}
}

func TestInputBudgetPreventsSpeedHack(t *testing.T) {
	w := NewWorld(testConfig())
	id := w.AddPlayer()

	// 一口氣送 100 筆（沒有推進 tick）：只有預算內的 0.25 / 0.05 = 5 筆會被套用。
	applied := 0
	for seq := uint32(1); seq <= 100; seq++ {
		if w.ApplyInput(id, seq, 1, 0) == InputApplied {
			applied++
		}
	}
	if applied != 5 {
		t.Fatalf("applied %d inputs in a burst, want 5", applied)
	}

	// 長時間下來，每秒最多只能移動 1 秒的量：連續 2 秒、每 tick 送 3 筆。
	w2 := NewWorld(testConfig())
	id2 := w2.AddPlayer()
	start := w2.Player(id2).Pos.X
	seq := uint32(0)
	for range 40 {
		for range 3 {
			seq++
			w2.ApplyInput(id2, seq, 1, 0)
		}
		w2.Step(0.05)
	}
	// 2 秒 × 100 單位/秒 = 200，加上初始預算 0.25 秒 = 25。
	if moved := w2.Player(id2).Pos.X - start; moved > 225+1e-6 {
		t.Fatalf("moved %v in 2s with 3x inputs, want ≤ 225", moved)
	}
}

func TestBudgetIsCapped(t *testing.T) {
	w := NewWorld(testConfig())
	id := w.AddPlayer()
	for range 100 {
		w.Step(0.05) // 閒置很久
	}
	if b := w.Player(id).InputBudget; !near(b, 0.25) {
		t.Fatalf("budget = %v, want capped at 0.25", b)
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
