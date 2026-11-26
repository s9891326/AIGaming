package game

import "math"

// GemID 是經驗寶石識別碼。
type GemID uint32

// Gem 是地上的一顆經驗寶石。
type Gem struct {
	ID    GemID
	Pos   Vec2
	Value int
}

// LootConfig 是掉落物與經驗值的參數。
type LootConfig struct {
	GemValue     int     // 一隻敵人掉落的經驗值
	GemRadius    float64 // 寶石半徑（碰到玩家就收集）
	MagnetRadius float64 // 玩家多近時寶石會飛過來
	MagnetSpeed  float64 // 寶石飛向玩家的速度
	MaxGems      int     // 全世界寶石上限，超過丟掉最舊的
	XPPerLevel   int     // 升到下一級需要 XPPerLevel × 目前等級
}

// DefaultLootConfig 是預設的掉落參數。
var DefaultLootConfig = LootConfig{
	GemValue:     1,
	GemRadius:    7,
	MagnetRadius: 100,
	MagnetSpeed:  400,
	MaxGems:      500,
	XPPerLevel:   5,
}

// XPToNext 回傳 level 升到下一級需要的經驗值。
func (c LootConfig) XPToNext(level int) int { return c.XPPerLevel * level }

// Gems 回傳所有寶石的副本。
func (w *World) Gems() []Gem {
	out := make([]Gem, len(w.gems))
	for i, g := range w.gems {
		out[i] = *g
	}
	return out
}

func (w *World) dropGem(pos Vec2, value int) {
	if value <= 0 {
		return
	}
	if max := w.cfg.Loot.MaxGems; max > 0 && len(w.gems) >= max {
		w.gems = w.gems[1:] // 丟掉最舊的
	}
	w.gems = append(w.gems, &Gem{ID: w.nextGemID, Pos: pos, Value: value})
	w.nextGemID++
}

// updateGems 讓吸取範圍內的寶石飛向最近的玩家，碰到就收集成經驗值。
func (w *World) updateGems(players []*Player, dt float64) {
	c := w.cfg.Loot
	reach := w.cfg.PlayerRadius + c.GemRadius
	kept := w.gems[:0]
	for _, g := range w.gems {
		p := nearestPlayer(players, g.Pos)
		if p == nil {
			kept = append(kept, g)
			continue
		}
		d := dist(p.Pos, g.Pos)
		if d <= c.MagnetRadius && d > 1e-9 {
			move := math.Min(c.MagnetSpeed*dt, d)
			g.Pos = g.Pos.Add(Vec2{p.Pos.X - g.Pos.X, p.Pos.Y - g.Pos.Y}.Scale(move / d))
			d -= move
		}
		if d < reach {
			w.addXP(p, g.Value)
			continue
		}
		kept = append(kept, g)
	}
	clear(w.gems[len(kept):])
	w.gems = kept
}

// addXP 增加經驗值，滿了就升級（可以一次升好幾級），多出來的經驗保留到下一級。
// 升級時的三選一在 #8 實作，這裡只計算等級。
func (w *World) addXP(p *Player, amount int) {
	p.XP += amount
	for need := w.cfg.Loot.XPToNext(p.Level); need > 0 && p.XP >= need; need = w.cfg.Loot.XPToNext(p.Level) {
		p.XP -= need
		p.Level++
	}
}
