package game

import "testing"

// lootWorld：沒有敵人、沒有武器，只測寶石與經驗。
func lootWorld() (*World, *Player) {
	cfg := combatConfig()
	cfg.Combat.SpawnRateBase, cfg.Combat.SpawnRateGrowth = 0, 0
	cfg.Loot = DefaultLootConfig // 吸取半徑 100、速度 400、寶石半徑 7
	w := newCombatWorld(cfg)
	return w, w.Player(w.AddPlayer())
}

func TestGemInsideMagnetRadiusFliesToPlayer(t *testing.T) {
	w, p := lootWorld()
	w.dropGem(Vec2{p.Pos.X + 90, p.Pos.Y}, 1) // 吸取範圍內

	w.Step(0.05) // 400 × 0.05 = 20

	g := w.Gems()[0]
	if !near(g.Pos.X, p.Pos.X+70) || !near(g.Pos.Y, p.Pos.Y) {
		t.Fatalf("gem at %+v, want pulled 20 units toward the player", g.Pos)
	}
}

func TestGemOutsideMagnetRadiusStays(t *testing.T) {
	w, p := lootWorld()
	start := Vec2{p.Pos.X + 150, p.Pos.Y}
	w.dropGem(start, 1)

	stepFor(w, 2)

	if g := w.Gems(); len(g) != 1 || g[0].Pos != start {
		t.Fatalf("gem outside magnet radius moved: %+v", g)
	}
	if p.XP != 0 {
		t.Fatal("no XP expected")
	}
}

func TestGemCollectedGivesXP(t *testing.T) {
	w, p := lootWorld()
	w.dropGem(Vec2{p.Pos.X + 90, p.Pos.Y}, 3)

	stepFor(w, 0.5) // 90 單位，0.25 秒內就到

	if len(w.Gems()) != 0 {
		t.Fatal("gem should be collected")
	}
	if p.XP != 3 || p.Level != 1 {
		t.Fatalf("xp=%d level=%d, want 3 / 1", p.XP, p.Level)
	}
}

func TestLevelUpCarriesOverXP(t *testing.T) {
	w, p := lootWorld()
	if got := w.cfg.Loot.XPToNext(1); got != 5 {
		t.Fatalf("XPToNext(1) = %d, want 5", got)
	}

	w.addXP(p, 7) // Lv1 需要 5 → Lv2，剩 2
	if p.Level != 2 || p.XP != 2 {
		t.Fatalf("after +7: level=%d xp=%d, want 2 / 2", p.Level, p.XP)
	}

	w.addXP(p, 33) // 2+33=35：Lv2 需要 10 → 25，Lv3 需要 15 → 10，Lv4 需要 20 → 不夠
	if p.Level != 4 || p.XP != 10 {
		t.Fatalf("after +33: level=%d xp=%d, want 4 / 10 (multiple level-ups at once)", p.Level, p.XP)
	}
}

func TestNearestPlayerCollectsGem(t *testing.T) {
	w, a := lootWorld()
	b := w.Player(w.AddPlayer())
	b.Pos = Vec2{a.Pos.X + 120, a.Pos.Y}
	w.dropGem(Vec2{a.Pos.X + 80, a.Pos.Y}, 1) // 離 a 80、離 b 40

	stepFor(w, 0.5)

	if a.XP != 0 || b.XP != 1 {
		t.Fatalf("xp a=%d b=%d, want the nearer player b to collect", a.XP, b.XP)
	}
}

func TestMaxGemsDropsOldest(t *testing.T) {
	w, _ := lootWorld()
	w.cfg.Loot.MaxGems = 3
	for i := range 5 {
		w.dropGem(Vec2{float64(100 + i), 100}, 1)
	}
	gems := w.Gems()
	if len(gems) != 3 || gems[0].ID != 3 || gems[2].ID != 5 {
		t.Fatalf("gems = %+v, want the newest 3 (ids 3..5)", gems)
	}
}

func TestLootClearedWhenNoPlayers(t *testing.T) {
	w, p := lootWorld()
	w.dropGem(Vec2{100, 100}, 1)
	w.spawnProjectile(Projectile{Pos: Vec2{100, 100}, TTL: 10})
	w.RemovePlayer(p.ID)
	w.Step(0.05)
	if len(w.Gems()) != 0 || len(w.Projectiles()) != 0 {
		t.Fatal("gems and projectiles should be cleared when nobody is playing")
	}
}
