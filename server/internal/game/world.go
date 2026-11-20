// Package game 是伺服器權威的遊戲模擬。
//
//   - World：純粹的遊戲狀態與規則（移動、邊界…），不碰網路、不開 goroutine，方便單元測試。
//   - Loop：唯一擁有 World 的 goroutine，以固定 tick 推進模擬並廣播快照。
package game

import (
	"math"
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
}

// DefaultWorldConfig 是預設的世界參數：世界比畫面（960×640）大，相機跟隨玩家。
var DefaultWorldConfig = WorldConfig{
	Width:          3000,
	Height:         2000,
	PlayerSpeed:    220,
	PlayerRadius:   16,
	InputDt:        1.0 / 20,
	MaxInputBudget: 0.25,
}

// Player 是一個角色。
type Player struct {
	ID           PlayerID
	Pos          Vec2
	LastInputSeq uint32  // 最後套用的輸入序號
	InputBudget  float64 // 還能套用多少秒的輸入
}

// World 是整個遊戲世界的狀態。不是並行安全的：只能由一個 goroutine（Loop）存取。
type World struct {
	cfg     WorldConfig
	tick    uint32
	nextID  PlayerID
	players map[PlayerID]*Player
}

// NewWorld 建立空的世界。
func NewWorld(cfg WorldConfig) *World {
	return &World{cfg: cfg, nextID: 1, players: make(map[PlayerID]*Player)}
}

// Config 回傳世界參數。
func (w *World) Config() WorldConfig { return w.cfg }

// Tick 回傳目前是第幾個 tick。
func (w *World) Tick() uint32 { return w.tick }

// AddPlayer 在世界中央加入新角色並回傳 ID。
func (w *World) AddPlayer() PlayerID {
	id := w.nextID
	w.nextID++
	w.players[id] = &Player{
		ID:          id,
		Pos:         Vec2{w.cfg.Width / 2, w.cfg.Height / 2},
		InputBudget: w.cfg.MaxInputBudget,
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
)

// ApplyInput 讓角色往 (x, y) 方向移動一個 InputDt。
// 方向先經過 NormalizeInput；序號必須遞增；每筆輸入消耗 InputDt 的時間預算。
func (w *World) ApplyInput(id PlayerID, seq uint32, x, y float64) InputResult {
	p := w.players[id]
	switch {
	case p == nil:
		return InputUnknown
	case seq <= p.LastInputSeq:
		return InputStale
	case p.InputBudget+1e-9 < w.cfg.InputDt:
		return InputOverBudget
	}
	p.InputBudget -= w.cfg.InputDt
	p.LastInputSeq = seq
	p.Pos = MoveStep(p.Pos, NormalizeInput(x, y), w.cfg)
	return InputApplied
}

// Step 推進模擬 dt 秒：tick 加一，並補充每個角色的輸入預算。
// 移動完全由 ApplyInput 驅動，所以客戶端可以用同樣的算法預測自己的位置。
func (w *World) Step(dt float64) {
	w.tick++
	for _, p := range w.players {
		p.InputBudget = math.Min(p.InputBudget+dt, w.cfg.MaxInputBudget)
	}
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
func MoveStep(pos, dir Vec2, cfg WorldConfig) Vec2 {
	next := pos.Add(dir.Scale(cfg.PlayerSpeed * cfg.InputDt))
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
