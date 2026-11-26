package game

import "testing"

func TestQuantize(t *testing.T) {
	for _, tc := range []struct {
		in   float64
		want uint32
	}{{0, 0}, {1500, 15000}, {123.456, 1235}, {-3, 0}, {2999.99, 30000}} {
		if got := quantize(tc.in); got != tc.want {
			t.Errorf("quantize(%v) = %d, want %d", tc.in, got, tc.want)
		}
	}
}

func TestDamagePct(t *testing.T) {
	for _, tc := range []struct {
		hp, max int
		want    uint32
	}{{100, 100, 0}, {99, 100, 1}, {399, 400, 1}, {200, 400, 50}, {0, 400, 100}, {-5, 20, 100}, {5, 0, 0}} {
		if got := damagePct(tc.hp, tc.max); got != tc.want {
			t.Errorf("damagePct(%d, %d) = %d, want %d", tc.hp, tc.max, got, tc.want)
		}
	}
}

func TestSnapshotAOI(t *testing.T) {
	w, p := weaponWorld()
	p.Weapons = nil
	inside := w.addEnemy(Vec2{p.Pos.X + 600, p.Pos.Y + 400}) // 畫面外但在邊距內
	outsideX := w.addEnemy(Vec2{p.Pos.X + 700, p.Pos.Y})     // 超出半寬 640
	outsideY := w.addEnemy(Vec2{p.Pos.X, p.Pos.Y - 500})     // 超出半高 480
	dead := w.addEnemy(Vec2{p.Pos.X + 10, p.Pos.Y})
	dead.HP = 0
	w.dropGem(Vec2{p.Pos.X + 100, p.Pos.Y}, 1)
	w.dropGem(Vec2{p.Pos.X + 2000, p.Pos.Y}, 1)
	w.spawnProjectile(Projectile{Pos: Vec2{p.Pos.X - 900, p.Pos.Y}, TTL: 9})
	w.grid.rebuild(w)

	other := w.Player(w.AddPlayer())
	other.Pos = Vec2{100, 100} // 另一位玩家在很遠的地方

	snap := newSnapshotBuilder(w, DefaultAOI, 0).For(p.Pos)

	if len(snap.GetEnemies()) != 1 || snap.GetEnemies()[0].GetId() != uint32(inside.ID) {
		t.Fatalf("enemies = %v, want only %d (outside: %d, %d; dead: %d)", snap.GetEnemies(), inside.ID, outsideX.ID, outsideY.ID, dead.ID)
	}
	if e := snap.GetEnemies()[0]; e.GetQx() != quantize(inside.Pos.X) || e.GetQy() != quantize(inside.Pos.Y) {
		t.Fatalf("quantized position %d,%d", e.GetQx(), e.GetQy())
	}
	if len(snap.GetGems()) != 1 || len(snap.GetProjectiles()) != 0 {
		t.Fatalf("gems %d projectiles %d, want 1 / 0", len(snap.GetGems()), len(snap.GetProjectiles()))
	}
	if len(snap.GetPlayers()) != 2 {
		t.Fatalf("players %d, want all players regardless of distance", len(snap.GetPlayers()))
	}
}
