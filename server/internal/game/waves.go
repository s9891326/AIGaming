package game

import "math"

// EnemyKind 是敵人種類，數值與 proto 的 game.v1.EnemyKind 相同。
type EnemyKind int32

const (
	KindGrunt  EnemyKind = 1 // 普通
	KindRunner EnemyKind = 2 // 疾行者：快速、脆弱
	KindBrute  EnemyKind = 3 // 巨漢：慢速、耐打、撞到很痛
	KindElite  EnemyKind = 4 // 精英：非常耐打，掉大量經驗
)

// EnemyKindDef 是一種敵人的基本數值（HP 會再乘上波次的成長倍率）。
type EnemyKindDef struct {
	Kind   EnemyKind
	HP     int
	Speed  float64
	Radius float64
	Damage int // 撞到玩家的傷害
	XP     int // 死亡時掉落的經驗值
}

// KindWeight 是某種敵人在一個波次中的出現權重。
type KindWeight struct {
	Kind   EnemyKind
	Weight int
}

// Wave 是波次表的一段：從玩家存活 From 秒開始生效，直到下一段。
type Wave struct {
	From    float64      // 開始秒數
	Rate    float64      // 這位玩家附近每秒生成幾隻
	Weights []KindWeight // 生成種類的權重（用 slice 而不是 map，順序固定才能重現）
}

// WaveConfig 是整張波次表。所有難度參數都在這裡，調整不用改程式。
type WaveConfig struct {
	Kinds             map[EnemyKind]EnemyKindDef
	Waves             []Wave  // 依 From 由小到大
	HPGrowthPerMinute float64 // 敵人 HP 每分鐘成長比例（0.12 = 每分鐘 +12%）
	EliteFirst        float64 // 第一隻精英出現的秒數（0 = 不出精英）
	EliteEvery        float64 // 之後每隔幾秒一隻
}

// DefaultWaves 是預設的波次表：約 10 分鐘內從零星的普通敵人，變成大量疾行者與巨漢混合，再加上定期的精英。
var DefaultWaves = WaveConfig{
	Kinds: map[EnemyKind]EnemyKindDef{
		KindGrunt:  {Kind: KindGrunt, HP: 20, Speed: 110, Radius: 14, Damage: 10, XP: 1},
		KindRunner: {Kind: KindRunner, HP: 10, Speed: 170, Radius: 11, Damage: 6, XP: 1},
		KindBrute:  {Kind: KindBrute, HP: 120, Speed: 70, Radius: 22, Damage: 20, XP: 4},
		KindElite:  {Kind: KindElite, HP: 400, Speed: 95, Radius: 26, Damage: 25, XP: 15},
	},
	Waves: []Wave{
		{From: 0, Rate: 0.8, Weights: []KindWeight{{KindGrunt, 1}}},
		{From: 30, Rate: 1.4, Weights: []KindWeight{{KindGrunt, 3}, {KindRunner, 1}}},
		{From: 90, Rate: 2.0, Weights: []KindWeight{{KindGrunt, 3}, {KindRunner, 2}, {KindBrute, 1}}},
		{From: 180, Rate: 2.8, Weights: []KindWeight{{KindGrunt, 2}, {KindRunner, 3}, {KindBrute, 2}}},
		{From: 300, Rate: 3.6, Weights: []KindWeight{{KindGrunt, 2}, {KindRunner, 3}, {KindBrute, 3}}},
		{From: 450, Rate: 4.5, Weights: []KindWeight{{KindGrunt, 1}, {KindRunner, 3}, {KindBrute, 4}}},
		{From: 600, Rate: 5.5, Weights: []KindWeight{{KindRunner, 3}, {KindBrute, 5}}},
	},
	HPGrowthPerMinute: 0.12,
	EliteFirst:        120,
	EliteEvery:        90,
}

// At 回傳玩家存活 age 秒時生效的波次。
func (wc *WaveConfig) At(age float64) *Wave {
	var cur *Wave
	for i := range wc.Waves {
		if wc.Waves[i].From <= age {
			cur = &wc.Waves[i]
		}
	}
	return cur
}

// HPMultiplier 回傳存活 age 秒時敵人 HP 的倍率。
func (wc *WaveConfig) HPMultiplier(age float64) float64 {
	return 1 + wc.HPGrowthPerMinute*age/60
}

// pick 依權重隨機選一種敵人；r 是 [0, 1) 的亂數。
func (wv *Wave) pick(r float64) EnemyKind {
	total := 0
	for _, kw := range wv.Weights {
		total += kw.Weight
	}
	if total <= 0 {
		return KindGrunt
	}
	n := int(r * float64(total))
	for _, kw := range wv.Weights {
		if n < kw.Weight {
			return kw.Kind
		}
		n -= kw.Weight
	}
	return wv.Weights[len(wv.Weights)-1].Kind
}

// spawnPlan 回傳某位玩家此刻的生成速率，以及用來選種類、決定 HP 倍率的函式。
// 沒有設定波次表時（舊的設定與測試），退回線性成長的速率與單一種普通敵人。
func (w *World) spawnPlan(age float64) (rate float64, pick func() EnemyKindDef) {
	c := w.cfg.Combat
	if c.Waves == nil {
		legacy := EnemyKindDef{
			Kind: KindGrunt, HP: c.EnemyHP, Speed: c.EnemySpeed, Radius: c.EnemyRadius,
			Damage: c.ContactDamage, XP: w.cfg.Loot.GemValue,
		}
		return c.SpawnRate(age), func() EnemyKindDef { return legacy }
	}
	wv := c.Waves.At(age)
	if wv == nil {
		return 0, nil
	}
	mult := c.Waves.HPMultiplier(age)
	return wv.Rate, func() EnemyKindDef {
		def := c.Waves.Kinds[wv.pick(w.rng.Float64())]
		def.HP = int(math.Round(float64(def.HP) * mult))
		return def
	}
}

// eliteDue 回傳這位玩家現在是否該出一隻精英，並排定下一隻的時間。
func (w *World) eliteDue(p *Player) (EnemyKindDef, bool) {
	wc := w.cfg.Combat.Waves
	if wc == nil || wc.EliteFirst <= 0 {
		return EnemyKindDef{}, false
	}
	if p.nextEliteAt == 0 {
		p.nextEliteAt = wc.EliteFirst
	}
	if p.Age < p.nextEliteAt {
		return EnemyKindDef{}, false
	}
	p.nextEliteAt += wc.EliteEvery
	def := wc.Kinds[KindElite]
	def.HP = int(math.Round(float64(def.HP) * wc.HPMultiplier(p.Age)))
	return def, true
}
