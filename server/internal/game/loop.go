package game

import (
	"context"
	"log/slog"
	"time"

	gamev1 "aigaming/server/internal/pb/game/v1"
)

// Sink 是收快照的對象（實際上是一條 WebSocket 連線）。Send 必須不阻塞。
type Sink interface {
	Send(msg *gamev1.ServerMessage) error
}

// LoopConfig 是遊戲迴圈的參數。
type LoopConfig struct {
	TickRate int // 每秒 tick 數
	World    WorldConfig
}

// DefaultLoopConfig 是預設參數：20Hz。
var DefaultLoopConfig = LoopConfig{TickRate: 20, World: DefaultWorldConfig}

type member struct {
	id   PlayerID
	sink Sink
}

// command 是要在 Loop goroutine 上執行的操作。
// 其他 goroutine 不直接碰 World，而是把「要做的事」包成閉包丟進 channel，
// 這樣 World 永遠只有一個擁有者，完全不需要鎖。
type command func(l *Loop)

// Loop 以固定 tick 推進 World，並把每個 tick 的快照廣播給所有成員。
//
// 對外的 Join / Leave / Input 可以從任何 goroutine 呼叫（通常是各連線的讀取 goroutine），
// 它們只是把指令送進 channel；真正的狀態變更都在 Run 的 goroutine 中發生。
type Loop struct {
	log     *slog.Logger
	cfg     LoopConfig
	world   *World
	members map[string]*member // key：連線 ID
	cmds    chan command
	done    chan struct{}
	now     func() time.Time
}

// NewLoop 建立遊戲迴圈，需要呼叫 Run 才會開始。
func NewLoop(log *slog.Logger, cfg LoopConfig) *Loop {
	// 一筆輸入 = 一個 tick 的移動，兩者必須一致，客戶端預測才會與伺服器相同。
	cfg.World.InputDt = 1 / float64(cfg.TickRate)
	if cfg.World.MaxInputBudget <= 0 {
		cfg.World.MaxInputBudget = 5 * cfg.World.InputDt
	}
	return &Loop{
		log:     log,
		cfg:     cfg,
		world:   NewWorld(cfg.World),
		members: make(map[string]*member),
		cmds:    make(chan command, 256),
		done:    make(chan struct{}),
		now:     time.Now,
	}
}

// Run 執行遊戲迴圈直到 ctx 結束。
func (l *Loop) Run(ctx context.Context) {
	defer close(l.done)

	interval := time.Second / time.Duration(l.cfg.TickRate)
	dt := interval.Seconds()
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	l.log.Info("game loop started", "tick_rate", l.cfg.TickRate,
		"world", l.cfg.World.Width, "x", l.cfg.World.Height)

	for {
		select {
		case <-ctx.Done():
			l.log.Info("game loop stopped", "tick", l.world.Tick())
			return
		case cmd := <-l.cmds:
			// 指令一到就處理（例如 Join 能立刻收到 JoinAccepted），不用等下一個 tick。
			cmd(l)
		case <-ticker.C:
			start := l.now()
			l.handleDeaths(l.world.Step(dt))
			l.broadcast()
			if took := l.now().Sub(start); took > interval/2 {
				l.log.Warn("slow tick", "tick", l.world.Tick(), "took", took, "budget", interval)
			}
		}
	}
}

// Join 讓 key（連線 ID）加入遊戲。同一個 key 重複加入會被忽略。
// sink 會先收到 JoinAccepted，之後每個 tick 收到 WorldSnapshot。
func (l *Loop) Join(key string, sink Sink) {
	l.enqueue(func(l *Loop) {
		if _, ok := l.members[key]; ok {
			return
		}
		id := l.world.AddPlayer()
		l.members[key] = &member{id: id, sink: sink}
		_ = sink.Send(&gamev1.ServerMessage{Payload: &gamev1.ServerMessage_JoinAccepted{
			JoinAccepted: &gamev1.JoinAccepted{
				PlayerId:         uint32(id),
				WorldWidth:       float32(l.cfg.World.Width),
				WorldHeight:      float32(l.cfg.World.Height),
				TickRate:         uint32(l.cfg.TickRate),
				PlayerRadius:     float32(l.cfg.World.PlayerRadius),
				PlayerSpeed:      float32(l.cfg.World.PlayerSpeed),
				EnemyRadius:      float32(l.cfg.World.Combat.EnemyRadius),
				ProjectileRadius: float32(DefaultMagicBolt.Radius),
				GemRadius:        float32(l.cfg.World.Loot.GemRadius),
			},
		}})
		l.log.Info("player joined", "conn", key, "player", id, "players", len(l.members))
	})
}

// Leave 移除 key 的角色；沒加入過時什麼都不做。
func (l *Loop) Leave(key string) {
	l.enqueue(func(l *Loop) {
		m, ok := l.members[key]
		if !ok {
			return
		}
		delete(l.members, key)
		l.world.RemovePlayer(m.id)
		l.log.Info("player left", "conn", key, "player", m.id, "players", len(l.members))
	})
}

// Input 讓 key 的角色往 (x, y) 移動一個 tick 的量（收到就套用，結果出現在下一個快照）。
func (l *Loop) Input(key string, seq uint32, x, y float64) {
	l.enqueue(func(l *Loop) {
		m, ok := l.members[key]
		if !ok {
			return
		}
		if r := l.world.ApplyInput(m.id, seq, x, y); r == InputOverBudget {
			// 偶爾發生是網路抖動；持續發生代表客戶端送太快（時鐘偏快或作弊）。
			l.log.Debug("input over budget", "conn", key, "player", m.id, "seq", seq)
		}
	})
}

// handleDeaths 通知死亡的玩家並把他們移出成員：之後不再收到快照，
// 連線保持開著，客戶端想再玩就重新送 JoinRequest（Join 會以新角色加入）。
func (l *Loop) handleDeaths(deaths []Death) {
	for _, d := range deaths {
		for key, m := range l.members {
			if m.id != d.ID {
				continue
			}
			delete(l.members, key)
			_ = m.sink.Send(&gamev1.ServerMessage{Payload: &gamev1.ServerMessage_GameOver{
				GameOver: &gamev1.GameOver{SurvivedMs: d.SurvivedMs, Kills: d.Kills},
			}})
			l.log.Info("player died", "conn", key, "player", d.ID,
				"survived", time.Duration(d.SurvivedMs)*time.Millisecond, "kills", d.Kills,
				"players", len(l.members))
			break
		}
	}
}

// enqueue 把指令送進 Loop；Loop 已停止時直接丟棄，避免呼叫端永遠卡住。
func (l *Loop) enqueue(cmd command) {
	select {
	case l.cmds <- cmd:
	case <-l.done:
	}
}

func (l *Loop) broadcast() {
	if len(l.members) == 0 {
		return
	}
	players := l.world.Players()
	states := make([]*gamev1.PlayerState, len(players))
	for i, p := range players {
		states[i] = &gamev1.PlayerState{
			Id:           uint32(p.ID),
			X:            float32(p.Pos.X),
			Y:            float32(p.Pos.Y),
			LastInputSeq: p.LastInputSeq,
			Hp:           uint32(p.HP),
			MaxHp:        uint32(p.MaxHP),
			Xp:           uint32(p.XP),
			XpToNext:     uint32(l.cfg.World.Loot.XPToNext(p.Level)),
			Level:        uint32(p.Level),
			Kills:        p.Kills,
		}
	}
	enemies := l.world.Enemies()
	enemyStates := make([]*gamev1.EnemyState, len(enemies))
	for i, e := range enemies {
		enemyStates[i] = &gamev1.EnemyState{
			Id: uint32(e.ID), X: float32(e.Pos.X), Y: float32(e.Pos.Y),
			Hp: uint32(e.HP), MaxHp: uint32(e.MaxHP),
		}
	}
	projectiles := l.world.Projectiles()
	projectileStates := make([]*gamev1.ProjectileState, len(projectiles))
	for i, p := range projectiles {
		projectileStates[i] = &gamev1.ProjectileState{Id: uint32(p.ID), X: float32(p.Pos.X), Y: float32(p.Pos.Y)}
	}
	gems := l.world.Gems()
	gemStates := make([]*gamev1.GemState, len(gems))
	for i, g := range gems {
		gemStates[i] = &gamev1.GemState{Id: uint32(g.ID), X: float32(g.Pos.X), Y: float32(g.Pos.Y), Value: uint32(g.Value)}
	}
	msg := &gamev1.ServerMessage{Payload: &gamev1.ServerMessage_Snapshot{Snapshot: &gamev1.WorldSnapshot{
		Tick:         l.world.Tick(),
		ServerTimeMs: float64(l.now().UnixMilli()),
		Players:      states,
		Enemies:      enemyStates,
		Projectiles:  projectileStates,
		Gems:         gemStates,
	}}}
	for _, m := range l.members {
		// Send 不會阻塞；失敗代表連線正在關閉，稍後會收到 Leave。
		_ = m.sink.Send(msg)
	}
}
