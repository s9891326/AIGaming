package game

import (
	"math"
	"math/rand/v2"
	"testing"
)

// TestGridMatchesBruteForce 用隨機的敵人分布比對：空間格子的查詢結果必須與逐一比對完全相同。
func TestGridMatchesBruteForce(t *testing.T) {
	w, _ := weaponWorld()
	rng := rand.New(rand.NewPCG(3, 4))
	kinds := []EnemyKind{KindGrunt, KindRunner, KindBrute, KindElite}
	for i := range 800 {
		e := w.spawnEnemy(DefaultWaves.Kinds[kinds[i%4]], Vec2{})
		e.Pos = Vec2{rng.Float64() * w.cfg.Width, rng.Float64() * w.cfg.Height}
		if i%7 == 0 {
			e.HP = 0 // 死掉但還沒移除的也要被略過
		}
	}
	w.grid.rebuild(w)

	for range 300 {
		pos := Vec2{rng.Float64() * w.cfg.Width, rng.Float64() * w.cfg.Height}
		r := rng.Float64() * 200

		// enemiesNear vs 逐一比對
		got := map[EnemyID]bool{}
		w.enemiesNear(pos, r, func(e *Enemy) bool { got[e.ID] = true; return true })
		for _, e := range w.enemies {
			want := e.HP > 0 && dist(e.Pos, pos) < r+e.Radius
			if got[e.ID] != want {
				t.Fatalf("enemiesNear(%v, %.1f): enemy %d at %v (r %.0f, hp %d) got %v want %v",
					pos, r, e.ID, e.Pos, e.Radius, e.HP, got[e.ID], want)
			}
		}

		// nearestEnemies vs 排序後取前幾個
		maxD := rng.Float64() * 500
		near := w.nearestEnemies(pos, maxD, 4)
		var brute []*Enemy
		for _, e := range w.enemies {
			if e.HP > 0 && dist(e.Pos, pos) <= maxD {
				brute = append(brute, e)
			}
		}
		for k := range min(4, len(brute)) { // 選擇排序取前 k 個
			best := k
			for i := k + 1; i < len(brute); i++ {
				di, db := dist(brute[i].Pos, pos), dist(brute[best].Pos, pos)
				if di < db || (di == db && brute[i].ID < brute[best].ID) {
					best = i
				}
			}
			brute[k], brute[best] = brute[best], brute[k]
		}
		brute = brute[:min(4, len(brute))]
		if len(near) != len(brute) {
			t.Fatalf("nearestEnemies count %d, brute force %d", len(near), len(brute))
		}
		for i := range near {
			if near[i].ID != brute[i].ID {
				t.Fatalf("nearestEnemies[%d] = %d, brute force %d", i, near[i].ID, brute[i].ID)
			}
		}
	}
}

func TestGridHandlesWorldEdges(t *testing.T) {
	w, _ := weaponWorld()
	e := w.spawnEnemy(DefaultWaves.Kinds[KindGrunt], Vec2{})
	e.Pos = Vec2{w.cfg.Width, w.cfg.Height} // 剛好在最右下角
	w.grid.rebuild(w)
	found := false
	w.enemiesNear(Vec2{w.cfg.Width - 5, w.cfg.Height - 5}, 10, func(*Enemy) bool { found = true; return true })
	if !found {
		t.Fatal("enemy at the world corner not found")
	}
	// 查詢範圍超出世界也不會越界。
	w.enemiesNear(Vec2{-500, -500}, 2000, func(*Enemy) bool { return true })
	_ = math.Pi
}
