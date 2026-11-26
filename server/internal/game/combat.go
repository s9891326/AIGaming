package game

import (
	"math"
)

// timeEps 是比較剩餘秒數時的容許誤差（遠小於一個 tick）。
const timeEps = 1e-6

// EnemyID 是敵人識別碼。
type EnemyID uint32

// Enemy 是一隻敵人。
type Enemy struct {
	ID  EnemyID
	Pos Vec2
}

// CombatConfig 是敵人與戰鬥相關的參數。
type CombatConfig struct {
	PlayerMaxHP     int
	SpawnProtection float64 // 加入後的無敵秒數
	HurtInvuln      float64 // 受傷後的無敵秒數
	ContactDamage   int     // 敵人碰到玩家的傷害

	EnemySpeed  float64
	EnemyRadius float64
	MaxEnemies  int // 全世界敵人上限

	// 每位玩家附近的生成速率（隻 / 秒）= min(SpawnRateBase + SpawnRateGrowth × 存活秒數, SpawnRateMax)
	SpawnRateBase   float64
	SpawnRateGrowth float64
	SpawnRateMax    float64
	// SpawnDistance 是生成點與玩家的距離。畫面 960×640 的半對角線約 577，所以要大於它才會在畫面外。
	SpawnDistance float64
}

// DefaultCombatConfig 是預設的戰鬥參數。
var DefaultCombatConfig = CombatConfig{
	PlayerMaxHP:     100,
	SpawnProtection: 2,
	HurtInvuln:      0.5,
	ContactDamage:   10,
	EnemySpeed:      110,
	EnemyRadius:     14,
	MaxEnemies:      300,
	SpawnRateBase:   0.8,
	SpawnRateGrowth: 0.06,
	SpawnRateMax:    5,
	SpawnDistance:   650,
}

// Death 是一位玩家死亡的紀錄。
type Death struct {
	ID         PlayerID
	SurvivedMs uint32
	Kills      uint32
}

// Enemies 回傳所有敵人的副本（依生成順序）。
func (w *World) Enemies() []Enemy {
	out := make([]Enemy, len(w.enemies))
	for i, e := range w.enemies {
		out[i] = *e
	}
	return out
}

// SpawnRate 回傳存活 age 秒的玩家附近的生成速率（隻 / 秒）。
func (c CombatConfig) SpawnRate(age float64) float64 {
	return math.Min(c.SpawnRateBase+c.SpawnRateGrowth*age, c.SpawnRateMax)
}

func (w *World) spawnEnemies(players []*Player, dt float64) {
	c := w.cfg.Combat
	for _, p := range players {
		p.spawnAccum += c.SpawnRate(p.Age) * dt
		for p.spawnAccum >= 1 {
			if len(w.enemies) >= c.MaxEnemies {
				// 到上限就丟掉累積量，避免之後一次湧出一大群。
				p.spawnAccum = 0
				break
			}
			p.spawnAccum--
			w.enemies = append(w.enemies, &Enemy{ID: w.nextEnemyID, Pos: w.spawnPoint(p.Pos)})
			w.nextEnemyID++
		}
	}
}

// spawnPoint 在 center 周圍 SpawnDistance 的隨機方向找生成點。
// 玩家靠近世界邊緣時，有些方向會出界：哪一軸出界就把那一軸的方向反過來（鏡像），
// 所以生成點仍然剛好在 SpawnDistance 外，而不是被夾到玩家身邊。
// 只有世界小於 2 × SpawnDistance 時鏡像也可能出界，最後再夾回世界內。
func (w *World) spawnPoint(center Vec2) Vec2 {
	r := w.cfg.Combat.EnemyRadius
	a := w.rng.Float64() * 2 * math.Pi
	dx := math.Cos(a) * w.cfg.Combat.SpawnDistance
	dy := math.Sin(a) * w.cfg.Combat.SpawnDistance
	if x := center.X + dx; x < r || x > w.cfg.Width-r {
		dx = -dx
	}
	if y := center.Y + dy; y < r || y > w.cfg.Height-r {
		dy = -dy
	}
	return w.clampToWorld(Vec2{center.X + dx, center.Y + dy}, r)
}

// moveEnemies 讓每隻敵人朝最近的玩家直線前進。
func (w *World) moveEnemies(players []*Player, dt float64) {
	step := w.cfg.Combat.EnemySpeed * dt
	for _, e := range w.enemies {
		target := nearestPlayer(players, e.Pos)
		d := dist(target.Pos, e.Pos)
		if d < 1e-9 {
			continue
		}
		move := math.Min(step, d) // 不要走過頭
		e.Pos = e.Pos.Add(Vec2{target.Pos.X - e.Pos.X, target.Pos.Y - e.Pos.Y}.Scale(move / d))
	}
}

// separateEnemies 把重疊的敵人互相推開（各推一半）。每個 tick 只做一輪，
// 不保證完全不重疊，但足以讓一大群敵人散開成一團而不是疊成一點。
// 目前是 O(n²)，n ≤ 300 時一個 tick 約 4.5 萬次距離計算；#10 會換成空間格子。
func (w *World) separateEnemies() {
	r := w.cfg.Combat.EnemyRadius
	minD := 2 * r
	for i := range w.enemies {
		a := w.enemies[i]
		for j := i + 1; j < len(w.enemies); j++ {
			b := w.enemies[j]
			dx, dy := b.Pos.X-a.Pos.X, b.Pos.Y-a.Pos.Y
			d := math.Hypot(dx, dy)
			if d >= minD {
				continue
			}
			var nx, ny float64
			if d < 1e-9 {
				// 完全重疊時沒有方向，用 ID 決定一個固定的方向。
				ang := float64(a.ID*7919+b.ID) * 0.6180339887
				nx, ny = math.Cos(ang), math.Sin(ang)
			} else {
				nx, ny = dx/d, dy/d
			}
			push := (minD - d) / 2
			a.Pos = Vec2{a.Pos.X - nx*push, a.Pos.Y - ny*push}
			b.Pos = Vec2{b.Pos.X + nx*push, b.Pos.Y + ny*push}
		}
	}
	for _, e := range w.enemies {
		e.Pos = w.clampToWorld(e.Pos, r)
	}
}

// applyContactDamage 讓碰到敵人的玩家受傷；血量歸零的玩家從世界移除並回傳。
// 每次受傷後有短暫無敵，所以被一群敵人包圍時也是每 HurtInvuln 秒扣一次，而不是每個 tick 扣好幾次。
func (w *World) applyContactDamage(players []*Player) []Death {
	c := w.cfg.Combat
	reach := w.cfg.PlayerRadius + c.EnemyRadius
	var deaths []Death
	for _, p := range players {
		// 用容許誤差比較：0.5 − 0.05 × 10 在浮點數下是極小的正數而不是 0，
		// 直接比 > 0 會讓每次無敵多一個 tick。
		if p.Invuln > timeEps || !w.touchingEnemy(p.Pos, reach) {
			continue
		}
		p.HP = max(0, p.HP-c.ContactDamage)
		p.Invuln = c.HurtInvuln
		if p.HP == 0 {
			deaths = append(deaths, Death{ID: p.ID, SurvivedMs: uint32(p.Age * 1000), Kills: p.Kills})
			delete(w.players, p.ID)
		}
	}
	return deaths
}

func (w *World) touchingEnemy(pos Vec2, reach float64) bool {
	for _, e := range w.enemies {
		if dist(e.Pos, pos) < reach {
			return true
		}
	}
	return false
}

func nearestPlayer(players []*Player, pos Vec2) *Player {
	var best *Player
	bestD := math.Inf(1)
	for _, p := range players {
		if d := dist(p.Pos, pos); d < bestD {
			best, bestD = p, d
		}
	}
	return best
}

func (w *World) clampToWorld(p Vec2, r float64) Vec2 {
	return Vec2{
		X: math.Min(math.Max(p.X, r), w.cfg.Width-r),
		Y: math.Min(math.Max(p.Y, r), w.cfg.Height-r),
	}
}

func dist(a, b Vec2) float64 { return math.Hypot(a.X-b.X, a.Y-b.Y) }
