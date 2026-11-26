package game

import "math"

// Weapon 是玩家身上的一把武器。World 每個 tick 對每位玩家的每把武器呼叫一次 Update；
// 武器自己管理冷卻，並決定何時、往哪裡發射（透過 World.spawnProjectile）。
// 之後的新武器（環繞物、範圍光環…）都實作這個介面。
type Weapon interface {
	Name() string
	Update(w *World, owner *Player, dt float64)
}

// ProjectileID 是投射物識別碼。
type ProjectileID uint32

// Projectile 是飛行中的投射物。命中一隻敵人或飛行時間用完就消失。
type Projectile struct {
	ID     ProjectileID
	Owner  PlayerID // 擊殺記給誰
	Pos    Vec2
	Vel    Vec2 // 單位 / 秒
	Damage int
	Radius float64
	TTL    float64 // 剩餘飛行秒數
}

// MagicBoltConfig 是「魔法飛彈」的參數。
type MagicBoltConfig struct {
	Cooldown float64 // 兩次發射的間隔（秒）
	Range    float64 // 只對這個距離內的敵人發射
	Speed    float64
	Damage   int
	Radius   float64
	Lifetime float64 // 最長飛行秒數
	Count    int     // 每次同時射幾發（各打不同的敵人）
}

// DefaultMagicBolt 是預設的魔法飛彈：每 0.8 秒朝最近的敵人射一發，一發打死一隻普通敵人（HP 20）。
// 每秒可殺 1.25 隻；生成速率約 7.5 秒後超過它，之後要靠升級（#8）才撐得住。
var DefaultMagicBolt = MagicBoltConfig{
	Cooldown: 0.8,
	Range:    450,
	Speed:    450,
	Damage:   20,
	Radius:   6,
	Lifetime: 1.3,
	Count:    1,
}

// MagicBolt 朝最近的敵人直線發射投射物。
type MagicBolt struct {
	cfg      MagicBoltConfig
	cooldown float64 // 剩餘冷卻秒數，≤ 0 代表可以發射
}

// NewMagicBolt 建立魔法飛彈（一開始就可以發射）。
func NewMagicBolt(cfg MagicBoltConfig) *MagicBolt { return &MagicBolt{cfg: cfg} }

// Name 實作 Weapon。
func (m *MagicBolt) Name() string { return "magic_bolt" }

// Update 實作 Weapon：冷卻好了、射程內有敵人時，朝最近的 Count 隻敵人各發射一發
// （敵人不夠多時只射有目標的）。沒有目標時保持「已就緒」，敵人一進入射程就立刻發射。
func (m *MagicBolt) Update(w *World, owner *Player, dt float64) {
	m.cooldown = math.Max(0, m.cooldown-dt)
	if m.cooldown > timeEps {
		return
	}
	targets := w.nearestEnemies(owner.Pos, m.cfg.Range, max(1, m.cfg.Count))
	if len(targets) == 0 {
		return
	}
	for _, target := range targets {
		d := dist(target.Pos, owner.Pos)
		dir := Vec2{1, 0}
		if d > 1e-9 {
			dir = Vec2{(target.Pos.X - owner.Pos.X) / d, (target.Pos.Y - owner.Pos.Y) / d}
		}
		w.spawnProjectile(Projectile{
			Owner:  owner.ID,
			Pos:    owner.Pos,
			Vel:    dir.Scale(m.cfg.Speed),
			Damage: m.cfg.Damage,
			Radius: m.cfg.Radius,
			TTL:    m.cfg.Lifetime,
		})
	}
	m.cooldown = m.cfg.Cooldown
}

// AuraConfig 是「聖光環」的參數。
type AuraConfig struct {
	Radius   float64 // 傷害範圍（玩家中心到敵人邊緣）
	Damage   int
	Interval float64 // 每隔多久造成一次傷害
}

// DefaultAura 是預設的聖光環。
var DefaultAura = AuraConfig{Radius: 70, Damage: 10, Interval: 0.5}

// Aura 定期傷害玩家身邊範圍內的所有敵人，不需要瞄準、不產生投射物。
type Aura struct {
	cfg   AuraConfig
	timer float64
}

// NewAura 建立聖光環（一開始就會造成傷害）。
func NewAura(cfg AuraConfig) *Aura { return &Aura{cfg: cfg} }

// Name 實作 Weapon。
func (a *Aura) Name() string { return "aura" }

// Radius 回傳目前的傷害範圍（給快照繪製用）。
func (a *Aura) Radius() float64 { return a.cfg.Radius }

// Update 實作 Weapon：每 Interval 秒對範圍內的每隻敵人造成一次傷害。
func (a *Aura) Update(w *World, owner *Player, dt float64) {
	a.timer = math.Max(0, a.timer-dt)
	if a.timer > timeEps {
		return
	}
	reach := a.cfg.Radius + w.cfg.Combat.EnemyRadius
	for _, e := range w.enemies {
		if e.HP > 0 && dist(e.Pos, owner.Pos) < reach {
			w.damageEnemy(e, a.cfg.Damage, owner.ID)
		}
	}
	a.timer = a.cfg.Interval
}

// Projectiles 回傳所有投射物的副本。
func (w *World) Projectiles() []Projectile {
	out := make([]Projectile, len(w.projectiles))
	for i, p := range w.projectiles {
		out[i] = *p
	}
	return out
}

func (w *World) spawnProjectile(p Projectile) {
	p.ID = w.nextProjectileID
	w.nextProjectileID++
	w.projectiles = append(w.projectiles, &p)
}

// nearestEnemies 回傳 maxDist 內最近的 n 隻存活敵人（由近到遠）。
func (w *World) nearestEnemies(pos Vec2, maxDist float64, n int) []*Enemy {
	type cand struct {
		e *Enemy
		d float64
	}
	var cs []cand
	for _, e := range w.enemies {
		if e.HP <= 0 {
			continue
		}
		if d := dist(e.Pos, pos); d <= maxDist {
			cs = append(cs, cand{e, d})
		}
	}
	// n 很小（≤ 4），部分選擇排序就夠了。
	out := make([]*Enemy, 0, min(n, len(cs)))
	for k := 0; k < n && k < len(cs); k++ {
		best := k
		for i := k + 1; i < len(cs); i++ {
			if cs[i].d < cs[best].d || (cs[i].d == cs[best].d && cs[i].e.ID < cs[best].e.ID) {
				best = i
			}
		}
		cs[k], cs[best] = cs[best], cs[k]
		out = append(out, cs[k].e)
	}
	return out
}

// damageEnemy 對敵人造成傷害；HP 歸零時記錄擊殺並掉寶石（實際移除在 removeDeadEnemies）。
func (w *World) damageEnemy(e *Enemy, damage int, owner PlayerID) {
	if e.HP <= 0 {
		return
	}
	e.HP = max(0, e.HP-damage)
	if e.HP == 0 {
		w.onEnemyKilled(e, owner)
	}
}

func (w *World) updateWeapons(players []*Player, dt float64) {
	for _, p := range players {
		for _, wp := range p.Weapons {
			wp.Update(w, p, dt)
		}
	}
}

// updateProjectiles 移動投射物並處理命中：每發投射物命中第一隻碰到的敵人後消失；
// 敵人 HP 歸零就死亡、掉落寶石，擊殺記給發射者。
//
// 每個 tick 的位移（450 × 0.05 = 22.5）小於投射物與敵人的半徑和的兩倍（40），所以不會「穿過」敵人；
// 之後若有更快的投射物，要改成掃掠（swept）碰撞。
func (w *World) updateProjectiles(dt float64) {
	er := w.cfg.Combat.EnemyRadius
	kept := w.projectiles[:0]
	for _, p := range w.projectiles {
		p.Pos = p.Pos.Add(p.Vel.Scale(dt))
		p.TTL -= dt

		hit := false
		for _, e := range w.enemies {
			if e.HP <= 0 || dist(e.Pos, p.Pos) >= p.Radius+er {
				continue
			}
			w.damageEnemy(e, p.Damage, p.Owner)
			hit = true
			break
		}

		outside := p.Pos.X < 0 || p.Pos.Y < 0 || p.Pos.X > w.cfg.Width || p.Pos.Y > w.cfg.Height
		if !hit && p.TTL > timeEps && !outside {
			kept = append(kept, p)
		}
	}
	clear(w.projectiles[len(kept):]) // 讓被移除的投射物可以被 GC
	w.projectiles = kept
	w.removeDeadEnemies()
}

// onEnemyKilled 記錄擊殺並在敵人位置掉落寶石。發射者已離開或死亡時不記擊殺，但仍會掉寶石。
func (w *World) onEnemyKilled(e *Enemy, killer PlayerID) {
	if p := w.players[killer]; p != nil {
		p.Kills++
	}
	w.dropGem(e.Pos, w.cfg.Loot.GemValue)
}

func (w *World) removeDeadEnemies() {
	kept := w.enemies[:0]
	for _, e := range w.enemies {
		if e.HP > 0 {
			kept = append(kept, e)
		}
	}
	clear(w.enemies[len(kept):])
	w.enemies = kept
}
