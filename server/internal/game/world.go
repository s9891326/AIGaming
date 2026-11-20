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
}

// DefaultWorldConfig 是預設的世界參數。
// 目前還沒有相機跟隨（#5），所以世界大小剛好是一個畫面。
var DefaultWorldConfig = WorldConfig{
	Width:        960,
	Height:       640,
	PlayerSpeed:  220,
	PlayerRadius: 16,
}

// Player 是一個角色。
type Player struct {
	ID           PlayerID
	Pos          Vec2
	Move         Vec2   // 目前的移動方向（已正規化，長度 ≤ 1）
	LastInputSeq uint32 // 最後套用的輸入序號
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
	w.players[id] = &Player{ID: id, Pos: Vec2{w.cfg.Width / 2, w.cfg.Height / 2}}
	return id
}

// RemovePlayer 移除角色；不存在時什麼都不做。
func (w *World) RemovePlayer(id PlayerID) { delete(w.players, id) }

// Player 回傳角色（唯讀用途），不存在時回傳 nil。
func (w *World) Player(id PlayerID) *Player { return w.players[id] }

// SetInput 設定角色的移動方向。輸入會先經過 NormalizeInput。
// 序號比目前舊的輸入會被忽略（雖然 WebSocket 保證順序，這裡仍防禦性檢查）。
func (w *World) SetInput(id PlayerID, seq uint32, x, y float64) {
	p := w.players[id]
	if p == nil || (seq != 0 && seq < p.LastInputSeq) {
		return
	}
	p.Move = NormalizeInput(x, y)
	p.LastInputSeq = seq
}

// Step 推進模擬 dt 秒。
func (w *World) Step(dt float64) {
	w.tick++
	for _, p := range w.players {
		p.Pos = w.clamp(p.Pos.Add(p.Move.Scale(w.cfg.PlayerSpeed * dt)))
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

// clamp 讓角色整個身體都留在世界內。
func (w *World) clamp(p Vec2) Vec2 {
	r := w.cfg.PlayerRadius
	return Vec2{
		X: math.Min(math.Max(p.X, r), w.cfg.Width-r),
		Y: math.Min(math.Max(p.Y, r), w.cfg.Height-r),
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
