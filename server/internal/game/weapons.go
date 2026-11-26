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

// Update 實作 Weapon：冷卻好了、射程內有敵人時，朝最近的敵人發射一發。
// 沒有目標時保持「已就緒」，敵人一進入射程就立刻發射。
func (m *MagicBolt) Update(w *World, owner *Player, dt float64) {
	m.cooldown = math.Max(0, m.cooldown-dt)
	if m.cooldown > timeEps {
		return
	}
	target := w.nearestEnemy(owner.Pos, m.cfg.Range)
	if target == nil {
		return
	}
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
	m.cooldown = m.cfg.Cooldown
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

// nearestEnemy 回傳 maxDist 內最近的存活敵人，沒有則回傳 nil。
func (w *World) nearestEnemy(pos Vec2, maxDist float64) *Enemy {
	var best *Enemy
	bestD := maxDist
	for _, e := range w.enemies {
		if e.HP <= 0 {
			continue
		}
		if d := dist(e.Pos, pos); d <= bestD {
			best, bestD = e, d
		}
	}
	return best
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
			e.HP = max(0, e.HP-p.Damage)
			if e.HP == 0 {
				w.onEnemyKilled(e, p.Owner)
			}
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
