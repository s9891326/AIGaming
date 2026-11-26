package game

import (
	"math"
	"math/rand/v2"
	"testing"
)

func TestWaveAt(t *testing.T) {
	wc := &DefaultWaves
	for _, tc := range []struct {
		age  float64
		from float64
	}{{0, 0}, {29.9, 0}, {30, 30}, {179, 90}, {599, 450}, {600, 600}, {3600, 600}} {
		if got := wc.At(tc.age); got == nil || got.From != tc.from {
			t.Errorf("At(%v) = %+v, want wave from %v", tc.age, got, tc.from)
		}
	}
}

func TestDefaultWavesRampUp(t *testing.T) {
	wc := &DefaultWaves
	prev := 0.0
	for i, wv := range wc.Waves {
		if i > 0 && wv.From <= wc.Waves[i-1].From {
			t.Fatalf("waves must be sorted by From: %v after %v", wv.From, wc.Waves[i-1].From)
		}
		if wv.Rate < prev {
			t.Fatalf("spawn rate decreases at %vs: %v < %v", wv.From, wv.Rate, prev)
		}
		prev = wv.Rate
		for _, kw := range wv.Weights {
			if _, ok := wc.Kinds[kw.Kind]; !ok || kw.Weight <= 0 {
				t.Fatalf("wave %v has unknown kind or non-positive weight: %+v", wv.From, kw)
			}
		}
	}
	if got := wc.HPMultiplier(600); !near(got, 2.2) {
		t.Fatalf("HP multiplier at 10 min = %v, want 2.2", got)
	}
}

func TestWavePickFollowsWeights(t *testing.T) {
	wv := Wave{Weights: []KindWeight{{KindGrunt, 1}, {KindRunner, 3}}}
	rng := rand.New(rand.NewPCG(7, 7))
	counts := map[EnemyKind]int{}
	for range 10000 {
		counts[wv.pick(rng.Float64())]++
	}
	if r := float64(counts[KindRunner]) / 10000; math.Abs(r-0.75) > 0.02 {
		t.Fatalf("runner share = %v, want ~0.75 (counts %v)", r, counts)
	}
}

func TestEliteSchedule(t *testing.T) {
	cfg := combatConfig()
	cfg.Combat.Waves = &DefaultWaves
	w := newCombatWorld(cfg)
	p := w.Player(w.AddPlayer())

	var at []float64
	for age := 0.0; age < 400; age += 0.05 {
		p.Age = age
		if _, ok := w.eliteDue(p); ok {
			at = append(at, math.Round(age))
		}
	}
	want := []float64{120, 210, 300, 390}
	if len(at) != len(want) {
		t.Fatalf("elites at %v, want %v", at, want)
	}
	for i := range want {
		if at[i] != want[i] {
			t.Fatalf("elites at %v, want %v", at, want)
		}
	}
}

// TestTenMinuteDifficultyCurve 模擬一位玩家存活 10 分鐘時的生成情況（只跑生成，不跑碰撞，所以很快），
// 確認每分鐘的生成數、耐打敵人的比例、平均 HP 都明顯上升。
func TestTenMinuteDifficultyCurve(t *testing.T) {
	cfg := combatConfig()
	cfg.Combat.Waves = &DefaultWaves
	cfg.Combat.MaxEnemies = 1 << 30
	w := newCombatWorld(cfg)
	p := w.Player(w.AddPlayer())

	type minute struct {
		spawned, brutes, elites, hp int
	}
	var mins [10]minute
	const dt = 0.05
	for age := 0.0; age < 600; age += dt {
		p.Age = age
		before := len(w.enemies)
		w.spawnEnemies([]*Player{p}, dt)
		m := &mins[int(age/60)]
		for _, e := range w.enemies[before:] {
			m.spawned++
			m.hp += e.MaxHP
			switch e.Kind {
			case KindBrute:
				m.brutes++
			case KindElite:
				m.elites++
			}
		}
		w.enemies = w.enemies[:0] // 只量生成，不累積
	}

	for i, m := range mins {
		t.Logf("minute %2d: spawned %3d  brutes %3d  elites %d  avg HP %5.1f",
			i+1, m.spawned, m.brutes, m.elites, float64(m.hp)/float64(max(m.spawned, 1)))
	}
	first, last := mins[0], mins[9]
	if last.spawned < 4*first.spawned {
		t.Errorf("minute 10 spawned %d, want ≥ 4× minute 1 (%d)", last.spawned, first.spawned)
	}
	if first.brutes != 0 || last.brutes*3 < last.spawned {
		t.Errorf("brutes: minute 1 = %d (want 0), minute 10 = %d of %d (want ≥ 1/3)", first.brutes, last.brutes, last.spawned)
	}
	avg := func(m minute) float64 { return float64(m.hp) / float64(m.spawned) }
	if avg(last) < 3*avg(first) {
		t.Errorf("average HP minute 10 = %.1f, want ≥ 3× minute 1 (%.1f)", avg(last), avg(first))
	}
	elites := 0
	for _, m := range mins {
		elites += m.elites
	}
	if elites < 5 {
		t.Errorf("elites in 10 minutes = %d, want ≥ 5", elites)
	}
}

func TestSpawnedEnemyUsesKindStats(t *testing.T) {
	cfg := combatConfig()
	cfg.Combat.Waves = &DefaultWaves
	w := newCombatWorld(cfg)
	def := DefaultWaves.Kinds[KindBrute]
	e := w.spawnEnemy(def, Vec2{1500, 1000})
	if e.Kind != KindBrute || e.Radius != def.Radius || e.Speed != def.Speed || e.HP != def.HP || e.XP != def.XP {
		t.Fatalf("enemy %+v does not match kind %+v", *e, def)
	}
}

func TestContactDamageUsesStrongestEnemy(t *testing.T) {
	w, p := upgradeWorld()
	p.Weapons = nil
	grunt := w.addEnemy(p.Pos)
	brute := w.addEnemy(p.Pos)
	brute.Damage, brute.Radius = 20, 22
	_ = grunt

	w.Step(0.05)

	if p.HP != 80 {
		t.Fatalf("hp = %d, want 80 (brute's 20 damage, not grunt's 10)", p.HP)
	}
}
