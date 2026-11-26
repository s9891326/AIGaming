// Package game 是伺服器權威的遊戲模擬。
//
//   - World：純粹的遊戲狀態與規則（移動、邊界…），不碰網路、不開 goroutine，方便單元測試。
//   - Loop：唯一擁有 World 的 goroutine，以固定 tick 推進模擬並廣播快照。
package game

import (
	"math"
	"math/rand/v2"
	"sort"
)

// PlayerID 是角色識別碼，0 代表「沒有」。
type PlayerID uint32

// Vec2 是 2D 向量（遊戲單位）。
type Vec2 struct{ X, Y float64 }

// Add 回傳 v + o。
func (v Vec2) Add(o Vec2) Vec2 { return Vec2{v.X + o.X, v.Y + o.Y} }

// Scale 回傳 v * s。
func (v Vec2) Scale(s float64) Vec2 { return Vec2{v.X * s, v.Y * s} }

// Len 回傳向量長度。
func (v Vec2) Len() float64 { return math.Hypot(v.X, v.Y) }

// WorldConfig 是世界的規則參數。
type WorldConfig struct {
	Width, Height float64 // 世界大小
	PlayerSpeed   float64 // 角色移動速度（單位 / 秒）
	PlayerRadius  float64 // 角色半徑，角色中心離邊界至少這麼遠
	// InputDt 是一筆 PlayerInput 代表的移動時間（秒），等於 1 / tick rate。
	InputDt float64
	// MaxInputBudget 是可累積的輸入時間上限（秒）。
	// 每個 tick 預算增加 dt，每套用一筆輸入扣 InputDt；網路抖動時短暫爆量的輸入可以被吸收，
	// 但長期而言每秒最多只能移動 1 秒的量（客戶端無法靠多送輸入加速）。
	MaxInputBudget float64

	Combat CombatConfig
	Loot   LootConfig
	// StartingWeapons 建立新玩家一開始的武器；nil 代表沒有武器。
	StartingWeapons func() []Weapon

	// Seed 不為 0 時使用固定亂數種子（模擬、重現問題用）；0 代表每次隨機。
	Seed uint64
}

// DefaultWorldConfig 是預設的世界參數：世界比畫面（960×640）大，相機跟隨玩家。
var DefaultWorldConfig = WorldConfig{
	Width:          3000,
	Height:         2000,
	PlayerSpeed:    220,
	PlayerRadius:   16,
	InputDt:        1.0 / 20,
	MaxInputBudget: 0.25,
	Combat:         DefaultCombatConfig,
	Loot:           DefaultLootConfig,
	StartingWeapons: func() []Weapon {
		return []Weapon{NewMagicBolt(DefaultMagicBolt)}
	},
}

// Player 是一個角色。
type Player struct {
	ID           PlayerID
	Pos          Vec2
	LastInputSeq uint32  // 最後套用的輸入序號
	InputBudget  float64 // 還能套用多少秒的輸入

	HP, MaxHP  int
	Age        float64 // 存活秒數
	Invuln     float64 // 剩餘無敵秒數（加入保護、受傷後無敵）
	Kills      uint32
	XP, Level  int
	Weapons    []Weapon
	spawnAccum float64 // 累積的生成量，滿 1 就在這位玩家附近生成一隻敵人

	// 會被升級改變的個人數值。
	Speed        float64 // 移動速度（單位 / 秒）
	MagnetRadius float64 // 寶石吸取範圍
	Might        float64 // 傷害倍率（1 = 100%）

	// KillsBy 記錄每把武器的擊殺數（用來比較不同 build）。
	KillsBy     map[string]int
	nextEliteAt float64 // 下一隻精英的存活秒數（0 = 尚未排定）

	// 升級：PendingLevelUps 是還沒選的升級次數；Offer 不為 nil 代表正在選（被凍結）。
	PendingLevelUps int
	Offer           *Offer
	Upgrades        map[string]int // 每種升級已選的次數
}

// Choosing 回傳玩家是否正在選升級（被凍結）。
func (p *Player) Choosing() bool { return p.Offer != nil }

// World 是整個遊戲世界的狀態。不是並行安全的：只能由一個 goroutine（Loop）存取。
type World struct {
	cfg     WorldConfig
	tick    uint32
	nextID  PlayerID
	players map[PlayerID]*Player

	enemies     []*Enemy
	nextEnemyID EnemyID
	grid        enemyGrid // 敵人的空間格子，每個 tick 敵人移動後重建

	projectiles      []*Projectile
	nextProjectileID ProjectileID
	gems             []*Gem
	nextGemID        GemID
	nextOfferID      uint32

	rng *rand.Rand // 生成位置等隨機性；測試可換成固定種子
}

// NewWorld 建立空的世界。
func NewWorld(cfg WorldConfig) *World {
	return &World{
		cfg:              cfg,
		nextID:           1,
		players:          make(map[PlayerID]*Player),
		nextEnemyID:      1,
		nextProjectileID: 1,
		nextGemID:        1,
		nextOfferID:      1,
		rng:              newRand(cfg.Seed),
	}
}

func newRand(seed uint64) *rand.Rand {
	if seed == 0 {
		return rand.New(rand.NewPCG(rand.Uint64(), rand.Uint64()))
	}
	return rand.New(rand.NewPCG(seed, seed^0x9e3779b97f4a7c15))
}

// Config 回傳世界參數。
func (w *World) Config() WorldConfig { return w.cfg }

// Tick 回傳目前是第幾個 tick。
func (w *World) Tick() uint32 { return w.tick }

// Time 回傳以 tick 計算的世界時間（秒）。暫停時仍會前進，與客戶端用 tick 估計的伺服器時間一致。
func (w *World) Time() float64 { return float64(w.tick) * w.cfg.InputDt }

// AddPlayer 在世界中央加入新角色並回傳 ID。
func (w *World) AddPlayer() PlayerID {
	id := w.nextID
	w.nextID++
	var weapons []Weapon
	if w.cfg.StartingWeapons != nil {
		weapons = w.cfg.StartingWeapons()
	}
	w.players[id] = &Player{
		ID:           id,
		Pos:          Vec2{w.cfg.Width / 2, w.cfg.Height / 2},
		InputBudget:  w.cfg.MaxInputBudget,
		HP:           w.cfg.Combat.PlayerMaxHP,
		MaxHP:        w.cfg.Combat.PlayerMaxHP,
		Invuln:       w.cfg.Combat.SpawnProtection,
		Level:        1,
		Weapons:      weapons,
		Speed:        w.cfg.PlayerSpeed,
		MagnetRadius: w.cfg.Loot.MagnetRadius,
		Might:        1,
		KillsBy:      make(map[string]int),
		Upgrades:     make(map[string]int),
	}
	return id
}

// RemovePlayer 移除角色；不存在時什麼都不做。
func (w *World) RemovePlayer(id PlayerID) { delete(w.players, id) }

// Player 回傳角色（唯讀用途），不存在時回傳 nil。
func (w *World) Player(id PlayerID) *Player { return w.players[id] }

// InputResult 是 ApplyInput 的結果，方便呼叫端記錄與測試。
type InputResult int

const (
	InputApplied    InputResult = iota
	InputUnknown                // 角色不存在
	InputStale                  // 序號不比上一筆新
	InputOverBudget             // 超出時間預算（送太快）
	InputFrozen                 // 正在選升級，不能移動
)

// ApplyInput 讓角色往 (x, y) 方向移動一個 InputDt。
// 方向先經過 NormalizeInput；序號必須遞增；每筆輸入消耗 InputDt 的時間預算。
func (w *World) ApplyInput(id PlayerID, seq uint32, x, y float64) InputResult {
	p := w.players[id]
	switch {
	case p == nil:
		return InputUnknown
	case p.Choosing():
		return InputFrozen
	case seq <= p.LastInputSeq:
		return InputStale
	case p.InputBudget+1e-9 < w.cfg.InputDt:
		return InputOverBudget
	}
	p.InputBudget -= w.cfg.InputDt
	p.LastInputSeq = seq
	p.Pos = MoveStep(p.Pos, NormalizeInput(x, y), p.Speed, w.cfg)
	return InputApplied
}

// StepResult 是一個 tick 中發生、需要通知玩家的事件。
type StepResult struct {
	Deaths []Death      // 死亡的玩家（已從世界移除）
	Offers []OfferEvent // 新的升級選單
}

// Step 推進模擬 dt 秒。
//
// 玩家的移動完全由 ApplyInput 驅動（客戶端可以用同樣的算法預測自己的位置）；
// Step 負責時間相關的事，依序：輸入預算與無敵時間 → 敵人生成 / 移動 / 分離 →
// 武器發射 → 投射物移動與命中（敵人死亡、掉寶石）→ 寶石吸取與經驗 → 接觸傷害 → 升級選單。
//
// 正在選升級的玩家被凍結，不參與模擬；所有人都在選時整個世界暫停（只有 tick 照常遞增，
// 讓客戶端用 tick 估計的伺服器時間在恢復後仍然連續）。
func (w *World) Step(dt float64) StepResult {
	w.tick++
	if len(w.players) == 0 {
		// 沒有人在玩：清空敵人與掉落物，下一位加入的玩家從乾淨的世界開始。
		w.enemies = w.enemies[:0]
		w.projectiles = w.projectiles[:0]
		w.gems = w.gems[:0]
		return StepResult{}
	}
	active := w.activePlayers()
	if len(active) == 0 {
		return StepResult{} // 大家都在選升級：暫停
	}
	for _, p := range active {
		p.InputBudget = math.Min(p.InputBudget+dt, w.cfg.MaxInputBudget)
		p.Age += dt
		p.Invuln = math.Max(0, p.Invuln-dt)
	}
	w.spawnEnemies(active, dt)
	w.moveEnemies(active, dt)
	w.grid.rebuild(w)
	w.separateEnemies()
	w.grid.rebuild(w) // 分離後位置變了，武器 / 投射物 / 接觸傷害用最新的格子
	w.updateWeapons(active, dt)
	w.updateProjectiles(dt) // 打死的敵人在接觸傷害之前就移除
	w.updateGems(active, dt)
	deaths := w.applyContactDamage(active)
	return StepResult{Deaths: deaths, Offers: w.makeOffers(active)}
}

// activePlayers 回傳沒有在選升級的玩家，依 ID 排序。
func (w *World) activePlayers() []*Player {
	all := w.sortedPlayers()
	out := all[:0]
	for _, p := range all {
		if !p.Choosing() {
			out = append(out, p)
		}
	}
	return out
}

// sortedPlayers 依 ID 排序，讓亂數的使用順序固定（同樣的種子 → 同樣的結果，測試才穩定）。
func (w *World) sortedPlayers() []*Player {
	out := make([]*Player, 0, len(w.players))
	for _, p := range w.players {
		out = append(out, p)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

// Players 回傳所有角色的副本，依 ID 排序（快照內容穩定，方便比對與測試）。
func (w *World) Players() []Player {
	out := make([]Player, 0, len(w.players))
	for _, p := range w.players {
		out = append(out, *p)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

// MoveStep 是一筆輸入的移動規則：沿 dir 走 speed × InputDt，再把整個身體夾回世界內。
// 前端 web/src/game/sim.ts 的 moveStep 必須與此一致，否則客戶端預測會一直被校正。
func MoveStep(pos, dir Vec2, speed float64, cfg WorldConfig) Vec2 {
	next := pos.Add(dir.Scale(speed * cfg.InputDt))
	r := cfg.PlayerRadius
	return Vec2{
		X: math.Min(math.Max(next.X, r), cfg.Width-r),
		Y: math.Min(math.Max(next.Y, r), cfg.Height-r),
	}
}

// NormalizeInput 把客戶端送來的方向變成合法的移動向量：
//   - NaN / Inf 視為 0（不信任客戶端）
//   - 長度超過 1 時縮放成 1，所以斜向（1, 1）不會比直走快
//   - 長度不到 1 的類比輸入（例如搖桿推一半）保留原長度
func NormalizeInput(x, y float64) Vec2 {
	if !finite(x) || !finite(y) {
		return Vec2{}
	}
	v := Vec2{x, y}
	if l := v.Len(); l > 1 {
		return v.Scale(1 / l)
	}
	return v
}

func finite(f float64) bool { return !math.IsNaN(f) && !math.IsInf(f, 0) }
