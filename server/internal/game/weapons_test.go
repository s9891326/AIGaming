package game

import (
	"math"
	"testing"
)

var _ Weapon = (*MagicBolt)(nil) // 編譯期確認實作了介面

// weaponWorld：沒有自動生成、敵人不動，只測武器與投射物。
func weaponWorld() (*World, *Player) {
	cfg := combatConfig()
	cfg.Combat.SpawnRateBase, cfg.Combat.SpawnRateGrowth = 0, 0
	cfg.Combat.EnemySpeed = 0
	cfg.Combat.SpawnProtection = 1e9
	cfg.Loot = DefaultLootConfig
	cfg.StartingWeapons = func() []Weapon { return []Weapon{NewMagicBolt(DefaultMagicBolt)} }
	w := newCombatWorld(cfg)
	return w, w.Player(w.AddPlayer())
}

func shotsFired(w *World) int { return int(w.nextProjectileID) - 1 }

func TestMagicBoltFiresOnCooldownWhenEnemyInRange(t *testing.T) {
	w, p := weaponWorld()
	e := w.addEnemy(Vec2{p.Pos.X + 400, p.Pos.Y}) // 射程 450 內
	e.HP, e.MaxHP = 1000, 1000                    // 打不死，持續當靶

	stepFor(w, 2)

	// 冷卻 0.8 秒：第 1 個 tick（0.05s）、0.85s、1.65s 各一發。
	if n := shotsFired(w); n != 3 {
		t.Fatalf("fired %d shots in 2s, want 3 (cooldown 0.8s)", n)
	}
}

func TestMagicBoltHoldsFireWithoutTarget(t *testing.T) {
	w, p := weaponWorld()
	w.addEnemy(Vec2{p.Pos.X + 600, p.Pos.Y}) // 射程外

	stepFor(w, 2)
	if n := shotsFired(w); n != 0 {
		t.Fatalf("fired %d shots with no enemy in range, want 0", n)
	}

	// 敵人一進入射程就立刻發射（冷卻早就好了）。
	w.addEnemy(Vec2{p.Pos.X, p.Pos.Y + 300})
	w.Step(0.05)
	if n := shotsFired(w); n != 1 {
		t.Fatalf("fired %d shots right after an enemy entered range, want 1", n)
	}
}

func TestMagicBoltAimsAtNearestEnemy(t *testing.T) {
	w, p := weaponWorld()
	w.addEnemy(Vec2{p.Pos.X + 400, p.Pos.Y}) // 右邊，比較遠
	w.addEnemy(Vec2{p.Pos.X, p.Pos.Y - 200}) // 上面，比較近

	w.Step(0.05)

	ps := w.Projectiles()
	if len(ps) != 1 {
		t.Fatalf("projectiles = %d, want 1", len(ps))
	}
	v := ps[0].Vel
	if !near(v.X, 0) || !near(v.Y, -DefaultMagicBolt.Speed) {
		t.Fatalf("velocity %+v, want straight up at speed %v", v, DefaultMagicBolt.Speed)
	}
}

func TestProjectileHitsKillsAndDropsGem(t *testing.T) {
	w, p := weaponWorld()
	target := Vec2{p.Pos.X + 200, p.Pos.Y}
	e := w.addEnemy(target)
	dmg := DefaultMagicBolt.Damage
	e.HP, e.MaxHP = 2*dmg, 2*dmg // 兩發才打死

	stepFor(w, 0.6) // 第一發飛到（約 0.4 秒）
	if e.HP != dmg {
		t.Fatalf("enemy hp = %d after first hit, want %d", e.HP, dmg)
	}
	if len(w.Projectiles()) != 0 {
		t.Fatal("projectile should disappear after hitting")
	}

	stepFor(w, 1) // 第二發（0.85 秒發射）命中
	if len(w.Enemies()) != 0 {
		t.Fatalf("enemy should be dead and removed, got %+v", w.Enemies())
	}
	if p.Kills != 1 {
		t.Fatalf("kills = %d, want 1", p.Kills)
	}
	gems := w.Gems()
	if len(gems) != 1 || gems[0].Value != 1 || dist(gems[0].Pos, target) > 1e-6 {
		t.Fatalf("gems = %+v, want one gem of value 1 at %+v", gems, target)
	}
}

func TestProjectileExpires(t *testing.T) {
	w, p := weaponWorld()
	w.spawnProjectile(Projectile{Owner: p.ID, Pos: p.Pos, Vel: Vec2{100, 0}, Damage: 10, Radius: 6, TTL: 0.3})

	stepFor(w, 0.25)
	if len(w.Projectiles()) != 1 {
		t.Fatal("projectile should still be flying")
	}
	stepFor(w, 0.1)
	if len(w.Projectiles()) != 0 {
		t.Fatal("projectile should expire after its lifetime")
	}
}

func TestProjectileRemovedOutsideWorld(t *testing.T) {
	w, p := weaponWorld()
	w.spawnProjectile(Projectile{Owner: p.ID, Pos: Vec2{5, 5}, Vel: Vec2{-1000, 0}, Damage: 10, Radius: 6, TTL: 10})
	w.Step(0.05)
	if len(w.Projectiles()) != 0 {
		t.Fatal("projectile that left the world should be removed")
	}
}

func TestKillByDepartedPlayerStillDropsGem(t *testing.T) {
	w, p := weaponWorld()
	other := w.Player(w.AddPlayer()) // 讓世界裡還有人，模擬才會繼續
	e := w.addEnemy(Vec2{1000, 1000})
	e.HP = 5
	w.spawnProjectile(Projectile{Owner: p.ID, Pos: Vec2{990, 1000}, Vel: Vec2{1, 0}, Damage: 10, Radius: 6, TTL: 1})
	w.RemovePlayer(p.ID)

	w.Step(0.05)

	if len(w.Enemies()) != 0 || len(w.Gems()) != 1 {
		t.Fatalf("enemies=%d gems=%d; want enemy killed and gem dropped", len(w.Enemies()), len(w.Gems()))
	}
	if other.Kills != 0 {
		t.Fatal("kill must not be credited to someone else")
	}
}

func TestWeaponsKillEnemiesEndToEnd(t *testing.T) {
	cfg := combatConfig()
	cfg.Combat.SpawnProtection = 1e9 // 不讓玩家死，只看擊殺與經驗
	cfg.Loot = DefaultLootConfig
	cfg.StartingWeapons = DefaultWorldConfig.StartingWeapons
	w := newCombatWorld(cfg)
	p := w.Player(w.AddPlayer())

	stepFor(w, 20)

	if p.Kills == 0 {
		t.Fatal("no kills after 20s with a weapon")
	}
	totalXP := p.XP
	for lv := 1; lv < p.Level; lv++ {
		totalXP += cfg.Loot.XPToNext(lv)
	}
	if totalXP == 0 {
		t.Fatal("no XP gained; enemies die next to the player so gems should be collected")
	}
	if math.Abs(float64(totalXP)-float64(p.Kills)) > float64(p.Kills) {
		t.Fatalf("xp %d is far from kills %d", totalXP, p.Kills)
	}
	t.Logf("20s: kills=%d level=%d xp=%d enemies alive=%d gems on ground=%d",
		p.Kills, p.Level, p.XP, len(w.Enemies()), len(w.Gems()))
}
