package game

import (
	"errors"
	"testing"
)

// upgradeWorld：有魔法飛彈、沒有自動生成，方便控制。
func upgradeWorld() (*World, *Player) {
	w, p := weaponWorld()
	p.Invuln = 0
	return w, p
}

// forceOffer 直接給玩家一份指定選項的選單（跳過隨機抽選）。
func forceOffer(w *World, p *Player, ids ...string) *Offer {
	o := &Offer{ID: w.nextOfferID, Level: p.Level}
	w.nextOfferID++
	for _, id := range ids {
		o.Options = append(o.Options, UpgradeOption{ID: id, Rank: p.Upgrades[id] + 1})
	}
	p.PendingLevelUps++
	p.Offer = o
	return o
}

func choose(t *testing.T, w *World, p *Player, id string) {
	t.Helper()
	o := forceOffer(w, p, id)
	if _, err := w.ChooseUpgrade(p.ID, o.ID, 0); err != nil {
		t.Fatalf("choose %s: %v", id, err)
	}
}

func TestLevelUpCreatesOfferWithThreeDistinctOptions(t *testing.T) {
	w, p := upgradeWorld()
	w.addXP(p, 5) // Lv1 → Lv2

	res := w.Step(0.05)

	if len(res.Offers) != 1 || res.Offers[0].Player != p.ID {
		t.Fatalf("offers = %+v, want one for player %d", res.Offers, p.ID)
	}
	o := res.Offers[0].Offer
	if o.Level != 2 || len(o.Options) != 3 {
		t.Fatalf("offer = %+v, want level 2 with 3 options", o)
	}
	seen := map[string]bool{}
	for _, op := range o.Options {
		if seen[op.ID] || findUpgrade(op.ID) == nil || op.Title == "" || op.Rank != 1 {
			t.Fatalf("bad or duplicate option %+v in %+v", op, o.Options)
		}
		seen[op.ID] = true
	}
	if !p.Choosing() {
		t.Fatal("player should be choosing")
	}
}

func TestSoloPlayerChoosingPausesWorld(t *testing.T) {
	w, p := upgradeWorld()
	w.cfg.Combat.EnemySpeed = 100
	e := w.addEnemy(Vec2{p.Pos.X + 300, p.Pos.Y})
	forceOffer(w, p, "move_speed")
	before, tick := e.Pos, w.Tick()

	stepFor(w, 1)

	if e.Pos != before || shotsFired(w) != 0 || p.Age != 0 {
		t.Fatalf("world should be paused: enemy %v→%v, shots %d, age %v", before, e.Pos, shotsFired(w), p.Age)
	}
	if w.Tick() != tick+20 {
		t.Fatalf("tick = %d, want %d (ticks keep counting while paused)", w.Tick(), tick+20)
	}
	if r := w.ApplyInput(p.ID, 1, 1, 0); r != InputFrozen {
		t.Fatalf("input while choosing = %v, want frozen", r)
	}
}

func TestOnlyChoosingPlayerFrozenInMultiplayer(t *testing.T) {
	w, a := upgradeWorld()
	b := w.Player(w.AddPlayer())
	b.Invuln = 0
	b.Pos = Vec2{2500, 1500} // 遠離 a
	w.cfg.Combat.EnemySpeed = 100
	w.cfg.StartingWeapons = nil
	a.Weapons, b.Weapons = nil, nil        // 不要讓武器把敵人打死
	nearA := w.addEnemy(a.Pos)             // 貼著 a
	chaser := w.addEnemy(Vec2{1600, 1500}) // a、b 中間偏 b
	forceOffer(w, a, "move_speed")

	stepFor(w, 1)

	if a.HP != a.MaxHP {
		t.Fatalf("choosing player took damage: hp %d", a.HP)
	}
	if b.Age == 0 || a.Age != 0 {
		t.Fatalf("ages a=%v b=%v; only b's time should advance", a.Age, b.Age)
	}
	// 敵人不追正在選的玩家：兩隻都往 b 走。
	if dist(chaser.Pos, b.Pos) >= 900 || dist(nearA.Pos, a.Pos) < 50 {
		t.Fatalf("enemies should ignore the choosing player and chase b (chaser %v, nearA %v)", chaser.Pos, nearA.Pos)
	}
}

func TestInvalidChoicesAreRejected(t *testing.T) {
	w, p := upgradeWorld()

	if _, err := w.ChooseUpgrade(p.ID, 1, 0); !errors.Is(err, ErrNoOffer) {
		t.Fatalf("no offer: err = %v", err)
	}
	if _, err := w.ChooseUpgrade(999, 1, 0); !errors.Is(err, ErrUnknownPlayer) {
		t.Fatalf("unknown player: err = %v", err)
	}

	o := forceOffer(w, p, "move_speed", "magnet", "max_hp")
	for _, tc := range []struct {
		offer uint32
		index int
		want  error
	}{
		{o.ID + 1, 0, ErrOfferMismatch}, // 不是這份選單（過期或亂送）
		{o.ID, 3, ErrBadIndex},          // 只有 0..2
		{o.ID, -1, ErrBadIndex},
	} {
		if _, err := w.ChooseUpgrade(p.ID, tc.offer, tc.index); !errors.Is(err, tc.want) {
			t.Errorf("choose(%d, %d) err = %v, want %v", tc.offer, tc.index, err, tc.want)
		}
	}
	if p.Offer != o || p.Speed != w.cfg.PlayerSpeed || p.PendingLevelUps != 1 {
		t.Fatal("rejected choices must not change anything; the offer stays open")
	}

	// 合法的選擇之後，同一份選單不能再選一次。
	if _, err := w.ChooseUpgrade(p.ID, o.ID, 1); err != nil {
		t.Fatalf("valid choice: %v", err)
	}
	if _, err := w.ChooseUpgrade(p.ID, o.ID, 1); !errors.Is(err, ErrNoOffer) {
		t.Fatalf("second choice on the same offer: err = %v, want ErrNoOffer", err)
	}
}

func TestChoosingResumesWorld(t *testing.T) {
	w, p := upgradeWorld()
	w.cfg.Combat.EnemySpeed = 100
	e := w.addEnemy(Vec2{p.Pos.X + 600, p.Pos.Y}) // 射程外，不會被打死
	o := forceOffer(w, p, "move_speed")
	stepFor(w, 0.5)
	before := e.Pos

	if _, err := w.ChooseUpgrade(p.ID, o.ID, 0); err != nil {
		t.Fatal(err)
	}
	w.Step(0.05)

	if e.Pos == before || p.Choosing() {
		t.Fatal("world should resume immediately after choosing")
	}
}

func TestUpgradeEffects(t *testing.T) {
	t.Run("move_speed", func(t *testing.T) {
		w, p := upgradeWorld()
		choose(t, w, p, "move_speed")
		start := p.Pos.X
		w.ApplyInput(p.ID, 1, 1, 0)
		if moved := p.Pos.X - start; !near(moved, w.cfg.PlayerSpeed*1.1*w.cfg.InputDt) {
			t.Fatalf("moved %v, want 10%% faster", moved)
		}
	})
	t.Run("max_hp", func(t *testing.T) {
		w, p := upgradeWorld()
		p.HP = 50
		choose(t, w, p, "max_hp")
		if p.MaxHP != 120 || p.HP != 70 {
			t.Fatalf("hp %d/%d, want 70/120", p.HP, p.MaxHP)
		}
	})
	t.Run("magnet", func(t *testing.T) {
		w, p := upgradeWorld()
		choose(t, w, p, "magnet")
		w.dropGem(Vec2{p.Pos.X + 120, p.Pos.Y}, 1) // 原本 100 外，現在 130 內
		w.Step(0.05)
		if g := w.Gems(); len(g) != 1 || g[0].Pos.X >= p.Pos.X+120 {
			t.Fatalf("gem at 120 should now be pulled in: %+v", g)
		}
	})
	t.Run("bolt_damage", func(t *testing.T) {
		w, p := upgradeWorld()
		choose(t, w, p, "bolt_damage")
		if d := weaponOf[*MagicBolt](p).cfg.Damage; d != DefaultMagicBolt.Damage+10 {
			t.Fatalf("damage = %d", d)
		}
	})
	t.Run("bolt_cooldown", func(t *testing.T) {
		w, p := upgradeWorld()
		choose(t, w, p, "bolt_cooldown")
		if c := weaponOf[*MagicBolt](p).cfg.Cooldown; !near(c, DefaultMagicBolt.Cooldown*0.85) {
			t.Fatalf("cooldown = %v", c)
		}
	})
	t.Run("bolt_count", func(t *testing.T) {
		w, p := upgradeWorld()
		choose(t, w, p, "bolt_count")
		w.addEnemy(Vec2{p.Pos.X + 300, p.Pos.Y})
		w.addEnemy(Vec2{p.Pos.X - 300, p.Pos.Y})
		w.Step(0.05)
		ps := w.Projectiles()
		if len(ps) != 2 || ps[0].Vel.X*ps[1].Vel.X >= 0 {
			t.Fatalf("want 2 projectiles at two different enemies, got %+v", ps)
		}
	})
	t.Run("aura_new_and_up", func(t *testing.T) {
		w, p := upgradeWorld()
		p.Weapons = nil // 只看聖光環
		choose(t, w, p, "aura_new")
		e := w.addEnemy(Vec2{p.Pos.X + 50, p.Pos.Y}) // 70 內
		e.HP, e.MaxHP = 100, 100
		far := w.addEnemy(Vec2{p.Pos.X + 200, p.Pos.Y})
		far.HP, far.MaxHP = 100, 100

		stepFor(w, 1) // 第 1 個 tick、0.55 秒各一次

		if e.HP != 80 || far.HP != 100 {
			t.Fatalf("aura hp near=%d far=%d, want 80 / 100", e.HP, far.HP)
		}
		choose(t, w, p, "aura_up")
		a := weaponOf[*Aura](p)
		if !near(a.Radius(), 84) || a.cfg.Damage != 15 {
			t.Fatalf("aura after upgrade radius=%v damage=%d, want 84 / 15", a.Radius(), a.cfg.Damage)
		}
	})
}

func TestQueuedLevelUpsOfferOneAtATime(t *testing.T) {
	w, p := upgradeWorld()
	w.addXP(p, 5+10) // Lv1 → Lv3：兩次升級

	first := w.Step(0.05).Offers
	if len(first) != 1 || first[0].Offer.Level != 2 || p.PendingLevelUps != 2 {
		t.Fatalf("first offer %+v pending %d; want level 2 with 2 pending", first, p.PendingLevelUps)
	}
	if again := w.Step(0.05).Offers; len(again) != 0 {
		t.Fatal("must not send another offer while one is open")
	}

	if _, err := w.ChooseUpgrade(p.ID, first[0].Offer.ID, 0); err != nil {
		t.Fatal(err)
	}
	second := w.Step(0.05).Offers
	if len(second) != 1 || second[0].Offer.Level != 3 || second[0].Offer.ID == first[0].Offer.ID {
		t.Fatalf("second offer %+v; want level 3 with a new id", second)
	}
}

func TestMaxedUpgradesAreNotOffered(t *testing.T) {
	w, p := upgradeWorld()
	for _, d := range upgradeDefs {
		if d.id != "magnet" {
			p.Upgrades[d.id] = d.maxRank
		}
	}
	p.Weapons = append(p.Weapons, NewAura(DefaultAura)) // aura_new 不可再出現

	o := w.newOffer(p)
	if len(o.Options) != 1 || o.Options[0].ID != "magnet" {
		t.Fatalf("options = %+v, want only magnet", o.Options)
	}

	p.Upgrades["magnet"] = 4
	o = w.newOffer(p)
	if len(o.Options) != 1 || o.Options[0].ID != "heal" {
		t.Fatalf("options = %+v, want heal when everything is maxed", o.Options)
	}
}

func TestWeaponUpgradesNeedTheWeapon(t *testing.T) {
	w, p := upgradeWorld()
	p.Weapons = nil
	for range 50 {
		for _, op := range w.newOffer(p).Options {
			if op.ID == "bolt_damage" || op.ID == "aura_up" {
				t.Fatalf("offered %s without owning the weapon", op.ID)
			}
		}
	}
}
