package game

import (
	"math"
	"slices"
)

// Build 是無頭模擬用的升級偏好：升級時從選單中挑在 Prefer 裡最前面的項目，都沒有就選第一個。
type Build struct {
	Name   string
	Prefer []string
}

// DefaultBuilds 是三種典型的 build，用來比較不同玩法。
var DefaultBuilds = []Build{
	{Name: "飛彈流", Prefer: []string{"bolt_count", "bolt_pierce", "bolt_damage", "bolt_cooldown", "might", "max_hp", "move_speed", "magnet"}},
	{Name: "光環流", Prefer: []string{"aura_new", "aura_up", "max_hp", "might", "move_speed", "magnet"}},
	{Name: "旋刃流", Prefer: []string{"orbit_new", "orbit_up", "might", "max_hp", "move_speed", "magnet"}},
}

// BuildResult 是一次模擬的結果。
type BuildResult struct {
	Build      string
	Survived   float64 // 存活秒數（活到最後就是模擬長度）
	Died       bool
	Level      int
	Kills      int
	KillsBy    map[string]int
	Upgrades   []string // 依序選了哪些升級
	PeakAlive  int      // 同時存活的敵人數最大值
	FinalAlive int
}

// SimulateBuild 用固定種子跑一局單人遊戲：AI 繞著世界中央轉圈（典型的「風箏」走位），
// 升級依 build 偏好選擇。整個模擬是確定性的，同樣的參數一定得到同樣的結果。
func SimulateBuild(cfg WorldConfig, b Build, seconds float64) BuildResult {
	const tickRate = 20
	cfg.InputDt = 1.0 / tickRate
	w := NewWorld(cfg)
	id := w.AddPlayer()
	res := BuildResult{Build: b.Name}

	center := Vec2{cfg.Width / 2, cfg.Height / 2}
	const orbitR = 450.0
	seq := uint32(0)
	for tick := 0; float64(tick)/tickRate < seconds; tick++ {
		p := w.Player(id)
		if p == nil {
			res.Died = true
			break
		}
		if p.Offer != nil {
			i := pickOption(p.Offer, b.Prefer)
			res.Upgrades = append(res.Upgrades, p.Offer.Options[i].ID)
			_, _ = w.ChooseUpgrade(id, p.Offer.ID, i)
		} else {
			d := w.botSteer(p, center, orbitR)
			seq++
			w.ApplyInput(id, seq, d.X, d.Y)
		}
		for _, d := range w.Step(cfg.InputDt).Deaths {
			if d.ID == id {
				res.Survived = float64(d.SurvivedMs) / 1000
			}
		}
		res.PeakAlive = max(res.PeakAlive, len(w.enemies))
		if p := w.Player(id); p != nil {
			res.Survived, res.Level, res.Kills = p.Age, p.Level, int(p.Kills)
			res.KillsBy = p.KillsBy
		}
	}
	res.FinalAlive = len(w.enemies)
	return res
}

// botSteer 是模擬玩家的走位：繞著中央轉圈（基本路線）、被附近的寶石吸引、
// 被靠太近的敵人推開，三者加權後正規化。粗糙但比只會繞圈更接近真人，升級數才有參考價值。
func (w *World) botSteer(p *Player, center Vec2, orbitR float64) Vec2 {
	rx, ry := p.Pos.X-center.X, p.Pos.Y-center.Y
	r := math.Hypot(rx, ry)
	if r < 1 {
		rx, ry, r = 1, 0, 1
	}
	corr := (r - orbitR) / orbitR
	d := Vec2{-ry/r - corr*rx/r, rx/r - corr*ry/r}

	if g := w.nearestGem(p.Pos, 300); g != nil {
		gd := dist(g.Pos, p.Pos)
		d = d.Add(Vec2{(g.Pos.X - p.Pos.X) / gd, (g.Pos.Y - p.Pos.Y) / gd}.Scale(1.5))
	}
	const danger = 160.0
	for _, e := range w.enemies {
		ed := dist(e.Pos, p.Pos)
		if ed < danger && ed > 1e-6 {
			push := 2.5 * (danger - ed) / danger
			d = d.Add(Vec2{(p.Pos.X - e.Pos.X) / ed, (p.Pos.Y - e.Pos.Y) / ed}.Scale(push))
		}
	}
	if l := d.Len(); l > 1e-9 {
		d = d.Scale(1 / l)
	}
	return d
}

func (w *World) nearestGem(pos Vec2, maxDist float64) *Gem {
	var best *Gem
	for _, g := range w.gems {
		if d := dist(g.Pos, pos); d <= maxDist {
			best, maxDist = g, d
		}
	}
	return best
}

func pickOption(o *Offer, prefer []string) int {
	best, bestRank := 0, len(prefer)
	for i, op := range o.Options {
		if r := slices.Index(prefer, op.ID); r >= 0 && r < bestRank {
			best, bestRank = i, r
		}
	}
	return best
}
