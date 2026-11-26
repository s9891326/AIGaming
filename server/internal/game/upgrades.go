package game

import (
	"errors"
	"fmt"
)

// UpgradeOption 是三選一裡的一個選項。
type UpgradeOption struct {
	ID          string
	Title       string
	Description string
	Rank        int // 選了之後這個項目會變成第幾級
}

// Offer 是一份等待玩家回覆的升級選單。
type Offer struct {
	ID      uint32
	Level   int // 升上的等級
	Options []UpgradeOption
}

// OfferEvent 是 Step 產生的新選單，Loop 會轉成 LevelUpOffer 送給該玩家。
type OfferEvent struct {
	Player PlayerID
	Offer  Offer
}

// 選擇升級時的錯誤。
var (
	ErrUnknownPlayer = errors.New("unknown player")
	ErrNoOffer       = errors.New("no pending level-up offer")
	ErrOfferMismatch = errors.New("offer id does not match the current offer")
	ErrBadIndex      = errors.New("option index out of range")
)

// upgradeDef 定義一種升級：最多可以選幾次、目前能不能出現、選了之後怎麼改變玩家。
type upgradeDef struct {
	id        string
	title     string
	describe  func(nextRank int) string
	maxRank   int // 0 代表不限
	available func(p *Player) bool
	apply     func(w *World, p *Player)
}

const optionsPerOffer = 3

// upgradeDefs 是所有升級。順序固定，隨機抽選時才不會受 map 走訪順序影響。
var upgradeDefs = []upgradeDef{
	{
		id: "bolt_damage", title: "魔法飛彈：威力", maxRank: 5,
		describe:  func(int) string { return "傷害 +10" },
		available: hasWeapon[*MagicBolt],
		apply:     func(_ *World, p *Player) { weaponOf[*MagicBolt](p).cfg.Damage += 10 },
	},
	{
		id: "bolt_cooldown", title: "魔法飛彈：連射", maxRank: 4,
		describe:  func(int) string { return "冷卻時間 −15%" },
		available: hasWeapon[*MagicBolt],
		apply:     func(_ *World, p *Player) { weaponOf[*MagicBolt](p).cfg.Cooldown *= 0.85 },
	},
	{
		id: "bolt_count", title: "魔法飛彈：分裂", maxRank: 3,
		describe:  func(r int) string { return fmt.Sprintf("每次同時射 %d 發，各打不同的敵人", r+1) },
		available: hasWeapon[*MagicBolt],
		apply:     func(_ *World, p *Player) { weaponOf[*MagicBolt](p).cfg.Count++ },
	},
	{
		id: "aura_new", title: "新武器：聖光環", maxRank: 1,
		describe: func(int) string {
			return fmt.Sprintf("持續傷害身邊 %.0f 內的敵人（每 %.1f 秒 %d）",
				DefaultAura.Radius, DefaultAura.Interval, DefaultAura.Damage)
		},
		available: func(p *Player) bool { return !hasWeapon[*Aura](p) },
		apply:     func(_ *World, p *Player) { p.Weapons = append(p.Weapons, NewAura(DefaultAura)) },
	},
	{
		id: "aura_up", title: "聖光環：擴散", maxRank: 4,
		describe:  func(int) string { return "範圍 +20%、傷害 +5" },
		available: hasWeapon[*Aura],
		apply: func(_ *World, p *Player) {
			a := weaponOf[*Aura](p)
			a.cfg.Radius *= 1.2
			a.cfg.Damage += 5
		},
	},
	{
		id: "move_speed", title: "疾行", maxRank: 5,
		describe:  func(int) string { return "移動速度 +10%" },
		available: always,
		apply:     func(_ *World, p *Player) { p.Speed *= 1.1 },
	},
	{
		id: "max_hp", title: "強健", maxRank: 5,
		describe:  func(int) string { return "最大血量 +20，並回復 20" },
		available: always,
		apply: func(_ *World, p *Player) {
			p.MaxHP += 20
			p.HP = min(p.HP+20, p.MaxHP)
		},
	},
	{
		id: "magnet", title: "磁力", maxRank: 4,
		describe:  func(int) string { return "寶石吸取範圍 +30%" },
		available: always,
		apply:     func(_ *World, p *Player) { p.MagnetRadius *= 1.3 },
	},
}

// healDef 只在其他升級都選滿時出現，保證永遠有東西可選。
var healDef = upgradeDef{
	id: "heal", title: "治療",
	describe:  func(int) string { return "回復 30 血量" },
	available: always,
	apply:     func(_ *World, p *Player) { p.HP = min(p.HP+30, p.MaxHP) },
}

func always(*Player) bool { return true }

func findUpgrade(id string) *upgradeDef {
	for i := range upgradeDefs {
		if upgradeDefs[i].id == id {
			return &upgradeDefs[i]
		}
	}
	if id == healDef.id {
		return &healDef
	}
	return nil
}

// weaponOf 回傳玩家身上第一把 T 型別的武器，沒有則回傳零值。
func weaponOf[T Weapon](p *Player) T {
	for _, w := range p.Weapons {
		if t, ok := w.(T); ok {
			return t
		}
	}
	var zero T
	return zero
}

func hasWeapon[T Weapon](p *Player) bool {
	for _, w := range p.Weapons {
		if _, ok := w.(T); ok {
			return true
		}
	}
	return false
}

// makeOffers 為有待選升級、還沒有選單的玩家產生三選一。
func (w *World) makeOffers(players []*Player) []OfferEvent {
	var out []OfferEvent
	for _, p := range players {
		if p.PendingLevelUps <= 0 || p.Offer != nil || w.players[p.ID] == nil {
			continue
		}
		p.Offer = w.newOffer(p)
		out = append(out, OfferEvent{Player: p.ID, Offer: *p.Offer})
	}
	return out
}

// newOffer 從目前可選的升級中隨機抽最多 3 個（不重複）；全部選滿時只給「治療」。
func (w *World) newOffer(p *Player) *Offer {
	var pool []*upgradeDef
	for i := range upgradeDefs {
		d := &upgradeDefs[i]
		if (d.maxRank == 0 || p.Upgrades[d.id] < d.maxRank) && d.available(p) {
			pool = append(pool, d)
		}
	}
	if len(pool) == 0 {
		pool = []*upgradeDef{&healDef}
	}
	w.rng.Shuffle(len(pool), func(i, j int) { pool[i], pool[j] = pool[j], pool[i] })
	pool = pool[:min(optionsPerOffer, len(pool))]

	offer := &Offer{ID: w.nextOfferID, Level: p.Level - p.PendingLevelUps + 1}
	w.nextOfferID++
	for _, d := range pool {
		rank := p.Upgrades[d.id] + 1
		offer.Options = append(offer.Options, UpgradeOption{
			ID: d.id, Title: d.title, Description: d.describe(rank), Rank: rank,
		})
	}
	return offer
}

// ChooseUpgrade 套用玩家在目前選單中選的選項。
// 只有「目前這份選單」的合法 index 會被接受；失敗時選單保持不變，玩家仍在選擇中。
// 還有排隊中的升級時，下一個 Step 會送出下一份選單。
func (w *World) ChooseUpgrade(id PlayerID, offerID uint32, index int) (UpgradeOption, error) {
	p := w.players[id]
	switch {
	case p == nil:
		return UpgradeOption{}, ErrUnknownPlayer
	case p.Offer == nil:
		return UpgradeOption{}, ErrNoOffer
	case p.Offer.ID != offerID:
		return UpgradeOption{}, ErrOfferMismatch
	case index < 0 || index >= len(p.Offer.Options):
		return UpgradeOption{}, ErrBadIndex
	}
	opt := p.Offer.Options[index]
	if d := findUpgrade(opt.ID); d != nil {
		d.apply(w, p)
	}
	p.Upgrades[opt.ID]++
	p.PendingLevelUps--
	p.Offer = nil
	return opt, nil
}
