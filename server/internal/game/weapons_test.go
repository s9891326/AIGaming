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

func TestOrbitBladesRotateAndHit(t *testing.T) {
	w, p := weaponWorld()
	p.Weapons = []Weapon{NewOrbit(DefaultOrbit)}
	o := weaponOf[*Orbit](p)

	c := DefaultOrbit

	// t=0 時刀刃平均分布，第一片在正右方。
	b := o.BladePositions(p.Pos, 0)
	if len(b) != c.Count || !near(b[0].X, p.Pos.X+c.Radius) || !near(b[0].Y, p.Pos.Y) {
		t.Fatalf("blades at t=0: %+v", b)
	}
	for i := range b {
		if !near(dist(b[i], p.Pos), c.Radius) {
			t.Fatalf("blade %d not on the orbit: %+v", i, b[i])
		}
	}

	// 放一隻敵人在軌道上：2 秒內刀刃經過約 Count × 轉數 次，每次命中至少間隔 HitInterval。
	e := w.addEnemy(Vec2{p.Pos.X, p.Pos.Y + c.Radius})
	e.HP, e.MaxHP = 10000, 10000
	stepFor(w, 2)
	hits := (10000 - e.HP) / c.Damage
	passes := float64(c.Count) * c.Speed * 2 / (2 * math.Pi)
	if float64(hits) < passes-1.5 || float64(hits) > passes+1.5 || float64(hits) > 2/c.HitInterval+1 {
		t.Fatalf("enemy hit %d times in 2s, want about %.1f (blade passes) and ≤ %.0f (hit interval)", hits, passes, 2/c.HitInterval+1)
	}
}

func TestOrbitKillsAreAttributed(t *testing.T) {
	w, p := weaponWorld()
	p.Weapons = []Weapon{NewOrbit(DefaultOrbit)}
	w.addEnemy(Vec2{p.Pos.X + DefaultOrbit.Radius, p.Pos.Y}) // 在軌道上，HP 20
	stepFor(w, 3)
	if p.KillsBy["orbit"] != 1 || p.Kills != 1 {
		t.Fatalf("kills by %v (total %d), want 1 by orbit", p.KillsBy, p.Kills)
	}
}

func TestPierceHitsMultipleEnemies(t *testing.T) {
	w, p := weaponWorld()
	p.Weapons = nil
	a := w.addEnemy(Vec2{p.Pos.X + 100, p.Pos.Y})
	b := w.addEnemy(Vec2{p.Pos.X + 160, p.Pos.Y})
	c := w.addEnemy(Vec2{p.Pos.X + 220, p.Pos.Y})
	for _, e := range []*Enemy{a, b, c} {
		e.HP, e.MaxHP = 100, 100
	}
	w.spawnProjectile(Projectile{Owner: p.ID, Source: "magic_bolt", Pos: p.Pos, Vel: Vec2{450, 0},
		Damage: 10, Radius: 6, TTL: 2, Pierce: 1})

	stepFor(w, 1)

	if a.HP != 90 || b.HP != 90 || c.HP != 100 {
		t.Fatalf("hp a=%d b=%d c=%d; pierce 1 should hit the first two only", a.HP, b.HP, c.HP)
	}
	if len(w.Projectiles()) != 0 {
		t.Fatal("projectile should be gone after using its pierce")
	}
}

func TestMightScalesDamage(t *testing.T) {
	w, p := weaponWorld()
	p.Might = 1.15
	e := w.addEnemy(Vec2{1, 1})
	e.HP, e.MaxHP = 100, 100
	w.damageEnemy(e, 20, p.ID, "test")
	if e.HP != 77 { // 20 × 1.15 = 23
		t.Fatalf("hp = %d, want 77", e.HP)
	}
}

func TestEliteDropsBigGem(t *testing.T) {
	w, p := weaponWorld()
	e := w.spawnEnemy(DefaultWaves.Kinds[KindElite], Vec2{100, 100})
	w.damageEnemy(e, 10000, p.ID, "test")
	gems := w.Gems()
	if len(gems) != 1 || gems[0].Value != 15 {
		t.Fatalf("gems = %+v, want one gem worth 15", gems)
	}
}
