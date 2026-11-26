package game

import (
	"math"

	gamev1 "aigaming/server/internal/pb/game/v1"
)

// PositionScale 是敵人 / 投射物 / 寶石座標的量化倍率：qx = round(x × 10)，精度 0.1 單位。
// varint 編碼下 3000 × 10 = 30000 只要 3 bytes，float32 固定要 5 bytes（含欄位標頭）。
const PositionScale = 10

// AOI（area of interest）是每位玩家會收到的實體範圍：以他為中心的矩形。
// 畫面是 960 × 640（半寬 480、半高 320），再加 160 的邊距——敵人在插值延遲（約 0.1 秒）內
// 最多走 17 單位，從邊距外走進畫面前一定已經出現在快照裡。
type AOI struct {
	HalfWidth, HalfHeight float64
}

// DefaultAOI 是預設的 AOI。
var DefaultAOI = AOI{HalfWidth: 480 + 160, HalfHeight: 320 + 160}

func (a AOI) contains(center, p Vec2) bool {
	return math.Abs(p.X-center.X) <= a.HalfWidth && math.Abs(p.Y-center.Y) <= a.HalfHeight
}

// snapshotBuilder 每個 tick 建立一次：玩家清單所有人共用，敵人 / 投射物 / 寶石依每位觀看者過濾。
type snapshotBuilder struct {
	w            *World
	aoi          AOI
	serverTimeMs float64
	players      []*gamev1.PlayerState
}

func newSnapshotBuilder(w *World, aoi AOI, serverTimeMs float64) *snapshotBuilder {
	b := &snapshotBuilder{w: w, aoi: aoi, serverTimeMs: serverTimeMs}
	for _, p := range w.Players() {
		s := &gamev1.PlayerState{
			Id:           uint32(p.ID),
			X:            float32(p.Pos.X),
			Y:            float32(p.Pos.Y),
			LastInputSeq: p.LastInputSeq,
			Hp:           uint32(p.HP),
			MaxHp:        uint32(p.MaxHP),
			Xp:           uint32(p.XP),
			XpToNext:     uint32(w.cfg.Loot.XPToNext(p.Level)),
			Level:        uint32(p.Level),
			Kills:        p.Kills,
			Speed:        float32(p.Speed),
			Choosing:     p.Choosing(),
			AuraRadius:   float32(auraRadius(p)),
			SurvivedMs:   uint32(p.Age * 1000),
		}
		if o := weaponOf[*Orbit](&p); o != nil {
			c := o.Config()
			s.OrbitCount = uint32(c.Count)
			s.OrbitRadius = float32(c.Radius)
			s.OrbitSpeed = float32(c.Speed)
			s.OrbitBladeRadius = float32(c.BladeRadius)
		}
		b.players = append(b.players, s)
	}
	return b
}

// For 回傳位於 viewer 的玩家會收到的快照。
func (b *snapshotBuilder) For(viewer Vec2) *gamev1.WorldSnapshot {
	w := b.w
	snap := &gamev1.WorldSnapshot{
		Tick:         w.Tick(),
		ServerTimeMs: b.serverTimeMs,
		Players:      b.players,
	}
	// 敵人用空間格子只查 AOI 附近的格子。
	w.grid.query(viewer, math.Max(b.aoi.HalfWidth, b.aoi.HalfHeight), func(e *Enemy) bool {
		if e.HP > 0 && b.aoi.contains(viewer, e.Pos) {
			snap.Enemies = append(snap.Enemies, &gamev1.EnemyState{
				Id:        uint32(e.ID),
				Kind:      gamev1.EnemyKind(e.Kind),
				Qx:        quantize(e.Pos.X),
				Qy:        quantize(e.Pos.Y),
				DamagePct: damagePct(e.HP, e.MaxHP),
			})
		}
		return true
	})
	for _, p := range w.projectiles {
		if b.aoi.contains(viewer, p.Pos) {
			snap.Projectiles = append(snap.Projectiles, &gamev1.ProjectileState{
				Id: uint32(p.ID), Qx: quantize(p.Pos.X), Qy: quantize(p.Pos.Y),
			})
		}
	}
	for _, g := range w.gems {
		if b.aoi.contains(viewer, g.Pos) {
			snap.Gems = append(snap.Gems, &gamev1.GemState{
				Id: uint32(g.ID), Value: uint32(g.Value), Qx: quantize(g.Pos.X), Qy: quantize(g.Pos.Y),
			})
		}
	}
	return snap
}

// quantize 把座標轉成 PositionScale 倍的整數（座標不會是負的）。
func quantize(v float64) uint32 {
	return uint32(math.Round(math.Max(v, 0) * PositionScale))
}

// damagePct 回傳已損失的血量百分比，無條件進位：只要受過傷就至少 1%（前端靠它觸發受擊閃白）。
func damagePct(hp, maxHP int) uint32 {
	if maxHP <= 0 || hp >= maxHP {
		return 0
	}
	lost := maxHP - max(hp, 0)
	return uint32((lost*100 + maxHP - 1) / maxHP)
}

// enemyKindInfos 回傳各種敵人的外觀資訊（放在 JoinAccepted，只送一次）。
func enemyKindInfos(c CombatConfig) []*gamev1.EnemyKindInfo {
	if c.Waves == nil {
		return []*gamev1.EnemyKindInfo{{Kind: gamev1.EnemyKind(KindGrunt), Radius: float32(c.EnemyRadius)}}
	}
	out := make([]*gamev1.EnemyKindInfo, 0, len(c.Waves.Kinds))
	for _, k := range []EnemyKind{KindGrunt, KindRunner, KindBrute, KindElite} {
		if def, ok := c.Waves.Kinds[k]; ok {
			out = append(out, &gamev1.EnemyKindInfo{Kind: gamev1.EnemyKind(k), Radius: float32(def.Radius)})
		}
	}
	return out
}

func auraRadius(p Player) float64 {
	if a := weaponOf[*Aura](&p); a != nil {
		return a.Radius()
	}
	return 0
}
