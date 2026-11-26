package game

import (
	"math"
	"math/rand/v2"
	"testing"
)

func combatConfig() WorldConfig {
	cfg := testConfig()
	cfg.Width, cfg.Height = 3000, 2000
	cfg.Combat = CombatConfig{
		PlayerMaxHP:     100,
		SpawnProtection: 2,
		HurtInvuln:      0.5,
		ContactDamage:   10,
		EnemySpeed:      100,
		EnemyRadius:     10,
		EnemyHP:         20,
		MaxEnemies:      300,
		SpawnRateBase:   1,
		SpawnRateGrowth: 0.1,
		SpawnRateMax:    5,
		SpawnDistance:   600,
	}
	return cfg
}

// newCombatWorld 用固定種子，讓生成位置每次都一樣。
func newCombatWorld(cfg WorldConfig) *World {
	w := NewWorld(cfg)
	w.rng = rand.New(rand.NewPCG(1, 2))
	return w
}

// addEnemy 直接放一隻敵人（不經過生成），方便測試追蹤 / 碰撞。
func (w *World) addEnemy(pos Vec2) *Enemy {
	hp := max(w.cfg.Combat.EnemyHP, 1)
	e := &Enemy{ID: w.nextEnemyID, Pos: pos, HP: hp, MaxHP: hp}
	w.nextEnemyID++
	w.enemies = append(w.enemies, e)
	return e
}

func stepFor(w *World, seconds float64) []Death {
	var deaths []Death
	for range int(math.Round(seconds / 0.05)) {
		deaths = append(deaths, w.Step(0.05)...)
	}
	return deaths
}

func TestNewPlayerHasFullHPAndSpawnProtection(t *testing.T) {
	w := newCombatWorld(combatConfig())
	p := w.Player(w.AddPlayer())
	if p.HP != 100 || p.MaxHP != 100 || p.Invuln != 2 {
		t.Fatalf("new player hp=%d/%d invuln=%v; want 100/100, 2s protection", p.HP, p.MaxHP, p.Invuln)
	}
}

func TestEnemiesSpawnOffScreenInsideWorld(t *testing.T) {
	cfg := combatConfig()
	w := newCombatWorld(cfg)
	id := w.AddPlayer()
	center := w.Player(id).Pos

	stepFor(w, 5)

	es := w.Enemies()
	if len(es) == 0 {
		t.Fatal("no enemies spawned in 5s")
	}
	r := cfg.Combat.EnemyRadius
	for _, e := range es {
		if e.Pos.X < r || e.Pos.X > cfg.Width-r || e.Pos.Y < r || e.Pos.Y > cfg.Height-r {
			t.Fatalf("enemy %d at %+v is outside the world", e.ID, e.Pos)
		}
	}
	// 生成點本身剛好在 SpawnDistance（畫面外）。
	for range 100 {
		if d := dist(w.spawnPoint(center), center); !near(d, cfg.Combat.SpawnDistance) {
			t.Fatalf("spawn point %v away, want exactly %v", d, cfg.Combat.SpawnDistance)
		}
	}
}

func TestSpawnRateIncreasesWithSurvivalTime(t *testing.T) {
	cfg := combatConfig()
	cfg.Combat.SpawnProtection = 1e9 // 這個測試只看數量，不讓玩家死掉
	w := newCombatWorld(cfg)
	w.AddPlayer()

	count := func() int { return int(w.nextEnemyID) - 1 } // 生成過的總數
	stepFor(w, 10)
	first := count()
	stepFor(w, 10)
	second := count() - first

	// 0~10 秒平均速率 1.5 隻/秒 → 約 15；10~20 秒平均 2.5 隻/秒 → 約 25。
	if first < 13 || first > 16 || second < 23 || second > 26 {
		t.Fatalf("spawned %d in 0-10s and %d in 10-20s; want ~15 then ~25", first, second)
	}
	if got := cfg.Combat.SpawnRate(1000); got != 5 {
		t.Fatalf("spawn rate should be capped at 5, got %v", got)
	}
}

func TestSpawnNearWorldCornerStaysAway(t *testing.T) {
	cfg := combatConfig()
	w := newCombatWorld(cfg)
	id := w.AddPlayer()
	w.Player(id).Pos = Vec2{20, 20} // 角落：大部分方向都會出界

	r := cfg.Combat.EnemyRadius
	for range 200 {
		p := w.spawnPoint(w.Player(id).Pos)
		if d := dist(p, Vec2{20, 20}); !near(d, cfg.Combat.SpawnDistance) {
			t.Fatalf("spawn point %+v is %v away from a player in the corner, want %v", p, d, cfg.Combat.SpawnDistance)
		}
		if p.X < r || p.Y < r {
			t.Fatalf("spawn point %+v outside the world", p)
		}
	}
}

func TestMaxEnemiesCap(t *testing.T) {
	cfg := combatConfig()
	cfg.Combat.MaxEnemies = 10
	cfg.Combat.SpawnRateBase = 100
	cfg.Combat.SpawnProtection = 1e9
	w := newCombatWorld(cfg)
	w.AddPlayer()
	stepFor(w, 3)
	if n := len(w.Enemies()); n != 10 {
		t.Fatalf("enemies = %d, want capped at 10", n)
	}
}

func TestEnemiesChaseNearestPlayer(t *testing.T) {
	cfg := combatConfig()
	cfg.Combat.SpawnRateBase, cfg.Combat.SpawnRateGrowth = 0, 0
	w := newCombatWorld(cfg)
	a, b := w.AddPlayer(), w.AddPlayer()
	w.Player(a).Pos = Vec2{500, 1000}
	w.Player(b).Pos = Vec2{2500, 1000}
	e := w.addEnemy(Vec2{800, 1000}) // 離 a 300、離 b 1700

	w.Step(0.5)

	// 往 a（左邊）走 100 × 0.5 = 50
	if !near(e.Pos.X, 750) || !near(e.Pos.Y, 1000) {
		t.Fatalf("enemy at %+v, want (750, 1000) moving toward nearest player", e.Pos)
	}
}

func TestEnemyDoesNotOvershootPlayer(t *testing.T) {
	cfg := combatConfig()
	cfg.Combat.SpawnRateBase, cfg.Combat.SpawnRateGrowth = 0, 0
	w := newCombatWorld(cfg)
	id := w.AddPlayer()
	pos := w.Player(id).Pos
	e := w.addEnemy(Vec2{pos.X + 3, pos.Y})

	w.Step(0.5) // 一步可以走 50，但距離只有 3

	if !near(e.Pos.X, pos.X) || !near(e.Pos.Y, pos.Y) {
		t.Fatalf("enemy overshot: %+v, player at %+v", e.Pos, pos)
	}
}

func TestEnemiesSeparate(t *testing.T) {
	cfg := combatConfig()
	cfg.Combat.SpawnRateBase, cfg.Combat.SpawnRateGrowth = 0, 0
	cfg.Combat.EnemySpeed = 0 // 只看分離
	w := newCombatWorld(cfg)
	w.AddPlayer()
	a := w.addEnemy(Vec2{100, 100})
	b := w.addEnemy(Vec2{100, 100}) // 完全重疊
	c := w.addEnemy(Vec2{105, 100}) // 部分重疊

	for range 20 {
		w.Step(0.05)
	}

	minD := 2 * cfg.Combat.EnemyRadius
	for _, pair := range [][2]*Enemy{{a, b}, {a, c}, {b, c}} {
		if d := dist(pair[0].Pos, pair[1].Pos); d < minD*0.95 {
			t.Fatalf("enemies %d and %d still overlap: distance %v (want ≥ %v)", pair[0].ID, pair[1].ID, d, minD)
		}
	}
}

func TestContactDamageWithInvulnerability(t *testing.T) {
	cfg := combatConfig()
	cfg.Combat.SpawnRateBase, cfg.Combat.SpawnRateGrowth = 0, 0
	w := newCombatWorld(cfg)
	id := w.AddPlayer()
	p := w.Player(id)
	w.addEnemy(p.Pos) // 一直貼著玩家

	stepFor(w, 1.9)
	if p.HP != 100 {
		t.Fatalf("hp = %d during spawn protection, want 100", p.HP)
	}

	stepFor(w, 0.15) // 保護結束後第一次碰撞
	if p.HP != 90 {
		t.Fatalf("hp = %d after first hit, want 90", p.HP)
	}
	stepFor(w, 0.4) // 受傷後 0.5 秒無敵
	if p.HP != 90 {
		t.Fatalf("hp = %d during hurt invulnerability, want 90", p.HP)
	}
	stepFor(w, 0.2)
	if p.HP != 80 {
		t.Fatalf("hp = %d after invulnerability ends, want 80", p.HP)
	}
}

func TestNoDamageWhenNotTouching(t *testing.T) {
	cfg := combatConfig()
	cfg.Combat.SpawnRateBase, cfg.Combat.SpawnRateGrowth = 0, 0
	cfg.Combat.EnemySpeed = 0
	cfg.Combat.SpawnProtection = 0
	w := newCombatWorld(cfg)
	id := w.AddPlayer()
	p := w.Player(id)
	reach := cfg.PlayerRadius + cfg.Combat.EnemyRadius
	w.addEnemy(Vec2{p.Pos.X + reach + 0.5, p.Pos.Y}) // 差一點點碰到

	stepFor(w, 2)
	if p.HP != 100 {
		t.Fatalf("hp = %d, want 100 (enemy never touched)", p.HP)
	}
}

func TestDeathRemovesPlayerAndReportsSurvival(t *testing.T) {
	cfg := combatConfig()
	cfg.Combat.SpawnRateBase, cfg.Combat.SpawnRateGrowth = 0, 0
	cfg.Combat.SpawnProtection = 0
	w := newCombatWorld(cfg)
	id := w.AddPlayer()
	other := w.AddPlayer()
	w.Player(other).Pos = Vec2{100, 100} // 遠離敵人
	w.addEnemy(w.Player(id).Pos)

	// 10 次 × 10 傷害，每次間隔 0.5 秒：第一次在第 1 個 tick，最後一次約在 4.55 秒。
	deaths := stepFor(w, 6)

	if len(deaths) != 1 || deaths[0].ID != id {
		t.Fatalf("deaths = %+v, want exactly player %d", deaths, id)
	}
	if ms := deaths[0].SurvivedMs; ms < 4400 || ms > 4700 {
		t.Fatalf("survived %d ms, want ~4550", ms)
	}
	if w.Player(id) != nil {
		t.Fatal("dead player must be removed from the world")
	}
	if w.Player(other) == nil || w.Player(other).HP != 100 {
		t.Fatal("other player should be unaffected")
	}
}

func TestEnemiesClearedWhenNoPlayers(t *testing.T) {
	w := newCombatWorld(combatConfig())
	id := w.AddPlayer()
	stepFor(w, 3)
	if len(w.Enemies()) == 0 {
		t.Fatal("setup: want some enemies")
	}
	w.RemovePlayer(id)
	w.Step(0.05)
	if n := len(w.Enemies()); n != 0 {
		t.Fatalf("enemies = %d after everyone left, want 0", n)
	}
}

func TestSameSeedSameResult(t *testing.T) {
	run := func() []Enemy {
		w := newCombatWorld(combatConfig())
		w.AddPlayer()
		w.AddPlayer()
		stepFor(w, 5)
		return w.Enemies()
	}
	a, b := run(), run()
	if len(a) != len(b) {
		t.Fatalf("different enemy counts: %d vs %d", len(a), len(b))
	}
	for i := range a {
		if a[i] != b[i] {
			t.Fatalf("enemy %d differs: %+v vs %+v (simulation must be deterministic for a given seed)", i, a[i], b[i])
		}
	}
}
