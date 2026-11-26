# AIGaming

[![CI](https://github.com/s9891326/AIGaming/actions/workflows/ci.yml/badge.svg)](https://github.com/s9891326/AIGaming/actions/workflows/ci.yml)

一個 **survivors-like**（割草 / 倖存者類）網頁遊戲的學習專案，目的是了解即時網頁遊戲如何運作：

- **前端**：React + TypeScript + Vite（`web/`），遊戲畫面用 Canvas 2D 繪製
- **後端**：Go + Gin（`server/`），採用伺服器權威（server-authoritative）的遊戲迴圈
- **通訊**：WebSocket 長連線，訊息格式使用 Protocol Buffers（`proto/`）
- **部署**：Docker Compose（nginx 提供前端並代理 `/api`、`/ws` 到後端）

## 架構概念

```
 瀏覽器 (React)                         Go Server (Gin)
 ┌────────────────┐   PlayerInput     ┌──────────────────────┐
 │ 鍵盤輸入        │ ───────────────▶ │ 收集輸入              │
 │                │   (WebSocket,    │ 固定 tick 遊戲迴圈     │
 │ Canvas 繪製     │    protobuf)     │  - 移動 / 生怪 / 碰撞   │
 │ 快照插值        │ ◀─────────────── │  - 傷害 / 經驗 / 升級   │
 └────────────────┘   WorldSnapshot   └──────────────────────┘
```

1. 客戶端只送「輸入」（往哪個方向移動），不送「我在哪裡」。
2. 伺服器以固定頻率（tick）模擬整個世界，是唯一的真相來源。
3. 伺服器把世界快照推給客戶端，客戶端在兩個快照之間插值，讓畫面保持順暢。

## 專案結構

```
AIGaming/
├── proto/                  # .proto 協議定義（前後端共用）+ buf 設定
├── server/                 # Go + Gin 後端
│   ├── cmd/server/         #   進入點
│   ├── internal/
│   │   ├── httpserver/     #   Gin 路由、中介層
│   │   ├── ws/             #   WebSocket：升級、讀寫 goroutine、心跳、連線管理
│   │   ├── session/        #   訊息分派：Welcome、Ping/Pong、Join/Input → 遊戲迴圈
│   │   ├── game/           #   遊戲模擬：World（純規則）、Loop（20Hz tick + 廣播）
│   │   └── pb/             #   ⚙ protoc-gen-go 產生的程式碼
│   └── pkg/logger/         #   可重用的小套件：彩色 slog logger
├── web/                    # React 前端（nginx.conf 給 docker 用）
│   └── src/
│       ├── net/            #   GameSocket（連線、重連）、useGameSocket hook
│       ├── game/           #   GameClient（預測 / 插值）、GameCanvas（相機、HUD）、鍵盤輸入
│       ├── components/     #   UI 元件（連線狀態、debug 面板…）
│       └── gen/            #   ⚙ protobuf-es 產生的程式碼
├── compose.yaml        # docker compose
├── Taskfile.yml        # 所有開發指令
├── .vscode/            # VS Code 設定、偵錯、工作
└── .claude/skills/     # Claude Code 開發流程（card-to-pr）
```

## 環境需求

- Go 1.25+、Node.js 22+、Docker Desktop
- [Task](https://taskfile.dev)（跨平台的 Makefile 替代品，Windows 不需要另外裝 make）：

```powershell
go install github.com/go-task/task/v3/cmd/task@latest
```

## 快速開始

```powershell
task dev   # 同時啟動後端 :8080 與前端 :5173，Ctrl+C 一起停止
```

打開 http://localhost:5173 ，看到「伺服器狀態 ● ok」就代表前後端串接成功。
開發時 Vite 會把 `/api`、`/ws` 代理到後端，瀏覽器只看到同一個來源，後端不需要 CORS。

### 用 Docker Compose 啟動

```powershell
task up     # 建置映像檔並在背景啟動，打開 http://localhost:3000
task logs   # 追蹤日誌
task down   # 停止
```

### 所有指令

執行 `task --list` 查看，常用的：

| 指令 | 說明 |
|---|---|
| `task dev` | 同時啟動前後端（開發模式） |
| `task dev:server` / `task dev:web` | 只啟動其中一邊 |
| `task check` | lint + format 檢查 + test（Go + vitest）+ build，開 PR 前必跑 |
| `task test:race` | 用 Linux 容器跑 `go test -race`（Windows 本機沒有 cgo） |
| `task format` | 自動排版（gofmt、prettier） |
| `task up` / `task down` / `task logs` | Docker Compose |

## VS Code

打開專案後安裝推薦的擴充套件（右下角會提示，或看 `.vscode/extensions.json`）。

- **F5 偵錯**：
  - `Server (Go)`：用 Delve 偵錯後端，可以下中斷點
  - `Web (Chrome)`：自動啟動 Vite 並開 Chrome，可在 TSX 下中斷點
  - `Full Stack`：兩個一起
- **工作**（`Ctrl+Shift+P` → `Tasks: Run Task`）：`dev: server + web`、`check`、`docker: up` 等，都是呼叫 Taskfile。
- `Ctrl+Shift+B` 直接執行 `dev: server + web`。

## 通訊協議（WebSocket + Protobuf）

前端連到 `/ws`（開發時經 Vite、部署時經 nginx 代理到後端），之後雙方只傳 **binary frame**，每個 frame 是一個 protobuf 訊息：

| 方向 | 信封 | 目前的內容（oneof） |
|---|---|---|
| 客戶端 → 伺服器 | `ClientMessage` | `Ping`、`JoinRequest`、`PlayerInput`、`ChooseUpgrade` |
| 伺服器 → 客戶端 | `ServerMessage` | `Welcome`、`Pong`、`JoinAccepted`、`WorldSnapshot`、`GameOver`、`LevelUpOffer` |

```
瀏覽器                                   Go server
  │ ── HTTP GET /ws (Upgrade) ─────────▶ │  Origin 必須與 Host 同源
  │ ◀──────── 101 Switching Protocols ── │
  │ ◀──────── Welcome{connection_id} ─── │
  │ ── JoinRequest ────────────────────▶ │
  │ ◀── JoinAccepted{player_id, world} ─ │  保證在第一個含自己的快照之前
  │ ◀── WorldSnapshot{tick, players[], enemies[], projectiles[], gems[]} │
  │                                      │  每個 tick（20Hz），含每人的 hp / xp / level / kills、last_input_seq
  │ ◀── LevelUpOffer{offer_id, options[3]} │  升級時；選單開著時自己被凍結（單人時整個世界暫停）
  │ ── ChooseUpgrade{offer_id, index} ──▶ │  不合法時伺服器拒絕並重送同一份選單
  │ ◀──── GameOver{survived_ms, kills} ── │  自己 HP 歸零時；之後不再收到快照，重送 JoinRequest 即再來一局
  │ ── PlayerInput{seq, move_x, move_y} ▶ │  移動時每個 tick 一筆（= 移動 1/20 秒），不動時不送
  │ ── Ping{client_time_ms} ───────────▶ │  每秒一次
  │ ◀──── Pong{client_time_ms, server} ─ │  RTT = now - client_time_ms
  │ ◀──────────── (WebSocket ping) ───── │  每 54 秒心跳，60 秒沒回應就斷線
```

- 後端每條連線有兩個 goroutine：`readPump`（唯一的讀取者）與 `writePump`（唯一的寫入者，也送心跳），其他地方只把訊息丟進 send channel。
- 前端斷線會以 0.5s → 1s → 2s …（最多 5s）自動重連。
- 開發模式下頁面底部有 **協議 Debug** 面板，把收發的訊息解碼成 JSON；部署版本在網址加 `?debug` 也能打開。

修改協議：編輯 `proto/game/v1/*.proto`，然後執行

```powershell
task proto   # buf lint + format 檢查 + 產生 Go / TS 程式碼
```

產生的程式碼要一起提交，CI 的 `proto` check 會確認沒有漏掉。

### 用 Postman 測試（JSON 除錯模式）

正式前端用 binary protobuf；手動測試時在網址加上 `?format=json`，這條連線就改用 **text frame + JSON**（[protojson](https://protobuf.dev/programming-guides/json/)，欄位名稱是 camelCase，預設值也會輸出）。

1. Postman → **New → WebSocket**，網址填 `ws://localhost:8080/ws?format=json`（直連後端，需 `task dev:server`），按 **Connect**
   - 也可以走 Vite 代理：`ws://localhost:5173/ws?format=json`
   - docker 模式（`GIN_MODE=release`）預設**關閉**，會回 400；要開就在 `compose.yaml` 的 server 加 `WS_ALLOW_JSON: "true"`
   - 若回 403：Postman 帶了不同源的 Origin 標頭，在 Headers 加上 `Origin: http://localhost:8080`
2. 連上後會收到 `{"welcome":{...}}`，接著依序送出（訊息類型選 **Text** / JSON）：

| 目的 | 送出 | 會收到 |
|---|---|---|
| 加入遊戲 | `{"join":{}}` | `{"joinAccepted":{"playerId":1,"worldWidth":3000,...}}`，之後每秒 20 則 `{"snapshot":{...}}` |
| 往右走一個 tick | `{"input":{"seq":1,"moveX":1,"moveY":0}}` | 下一則 snapshot 中自己的 `x` +11、`lastInputSeq` 變 1 |
| 往左下走 | `{"input":{"seq":2,"moveX":-1,"moveY":1}}` | `seq` 必須遞增，重複或變小會被忽略 |
| 量延遲 | `{"ping":{"clientTimeMs":1000}}` | `{"pong":{"clientTimeMs":1000,"serverTimeMs":...}}` |

- 快照每秒 20 則會很快洗版，可以用 Postman 訊息列表的搜尋 / 篩選（例如篩 `joinAccepted`、`pong`）。
- 送出格式錯誤的 JSON 不會斷線，伺服器日誌會記 `WRN ignoring malformed message`。
- 一次狂送超過 5 筆 `input`，超出時間預算的會被丟棄（防加速）。

## 遊戲迴圈（伺服器權威）

```
連線 goroutine（每條一個）            Loop goroutine（唯一擁有 World）
  Join / Leave / Input ──閉包──▶ cmds channel ──▶ 立即執行（改 World）
                                       ticker 20Hz ──▶ World.Step(0.05s) ──▶ 廣播 WorldSnapshot
```

- `server/internal/game/world.go`、`combat.go`、`waves.go`、`weapons.go`、`loot.go`、`upgrades.go`、`spatial.go`：純遊戲規則（移動、輸入正規化、邊界、敵人、傷害），不碰網路、不開 goroutine，單元測試直接呼叫。
- `server/internal/game/loop.go`：其他 goroutine 只把「要做的事」包成閉包送進 channel，World 永遠只有一個擁有者，不需要鎖。
- 客戶端只送「想往哪走」；位置一律由伺服器算，長度超過 1 的輸入會被正規化（斜走不會比較快）、NaN 會被丟棄。
- **移動由輸入驅動**：一筆 `PlayerInput` = 往該方向移動一個 tick（`MoveStep`），伺服器收到就套用；`Step` 只推進 tick 並補充每人的「輸入時間預算」。每筆輸入扣 1/20 秒預算、上限 0.25 秒，所以網路抖動造成的短暫爆量能被吸收，但長期每秒最多只能移動 1 秒的量（多送輸入無法加速）。

## 敵人與戰鬥

規則都在 `server/internal/game/combat.go`（參數集中在 `DefaultCombatConfig`）：

| 項目 | 規則 |
|---|---|
| 生成 | 每位玩家各自累積，在他周圍 **650** 單位（畫面外）的隨機方向生成；靠近世界邊緣時把出界的軸鏡像，距離不變；全世界最多 300 隻，沒有玩家時清空 |
| 波次表 | `waves.go`（資料驅動）：依存活時間分 7 段，速率 0.8 → 5.5 隻/秒、種類組成逐漸變成疾行者與巨漢；敵人 HP 每分鐘 +12%；2 分鐘起每 90 秒一隻精英 |
| 敵人種類 | 普通（HP 20、速度 110）／疾行者（HP 10、速度 170）／巨漢（HP 120、速度 70、撞到 20 傷害、掉 4 經驗）／精英（HP 400、掉 15 經驗的金色大寶石）；碰撞用各自的半徑 |
| 追蹤 | 朝最近的（沒有在選升級的）玩家直線前進（玩家速度 220，可以甩開大部分敵人） |
| 分離 | 重疊的敵人互相推開，避免疊成一點（目前 O(n²)，#10 換空間格子） |
| 傷害 | 碰到就扣 10；受傷後無敵 0.5 秒、加入後保護 2 秒；HP 100 |
| 死亡 | 該玩家移出世界、收到 `GameOver`，其他玩家不受影響；按「再來一局」（或 Enter）重新加入 |
| 武器 | `Weapon` 介面（`weapons.go`），每 tick 由 World 呼叫。**魔法飛彈**：每 0.8 秒朝 450 內最近的敵人射擊（可升級傷害、冷卻、分裂、穿透）；**聖光環**：身邊 100 內每 0.4 秒 14 傷害；**旋刃**：3 片刀刃繞 95 單位旋轉、碰到 22 傷害（角度只由世界時間決定，客戶端用同一公式畫）；被動**力量**：所有傷害 +15% |
| 敵人死亡 | 敵人 HP 20（一發打死）；擊殺記給發射者，原地掉一顆經驗寶石 |
| 升級三選一 | 每升一級排一份選單（`upgrades.go`），從可用的升級中隨機抽 3 個；選單開著時該玩家被凍結（不能動、不受傷、敵人不追），**所有人都在選時整個世界暫停**（tick 照常遞增）。選擇要帶 `offer_id`，過期 / 超出範圍會被拒絕並重送選單 |
| 升級項目 | 魔法飛彈：傷害 +10、冷卻 −15%、多射 1 發；新武器聖光環（身邊 70 內每 0.5 秒 10 傷害）與其強化；移速 +10%、最大血量 +20、吸取範圍 +30%；都選滿時出現「治療」 |
| 寶石與經驗 | 玩家 100 內的寶石以 400/秒飛過來，碰到就收集（`loot.go`）；升級需要 `5 × 等級` 經驗，多的保留；最多 500 顆 |

- 敵人不會推動玩家，所以客戶端預測不受影響（玩家位置仍只由輸入決定）。
- 亂數來源可替換：測試用固定種子，同樣的種子 → 同樣的結果。
- 前端：敵人與其他玩家一樣用插值；血量從快照比較前後值，下降時角色閃白、畫面邊緣閃紅（`web/src/game/health.ts`）。

### 效能（#10）

| 量測 | 指令 / 位置 | 300 隻 | 1000 隻 | 2000 隻 |
|---|---|---|---|---|
| 伺服器 tick（優化前 → 後） | `go test -bench Step -run ^$ ./internal/game/` | 0.38 → 0.18 ms | 3.9 → **0.9 ms** | 14.9 → **2.4 ms** |
| 每位玩家的快照（優化前 → 後） | `go test -run PerfReport -v ./internal/game/` | 10.1 → 4.1 KB | 33.9 → 13.3 KB（聚集）／5.2 KB（散布） | 67.9 → 10.1 KB（散布） |
| 前端每幀繪製 | DevTools：`(await import('/src/game/drawBench.ts')).runDrawBench({ enemies: 1000 })` | ≈ 1.2 ms | ≈ 1.7 ms | ≈ 2.6 ms |

- **空間格子**（`spatial.go`）：世界切成 64 單位的格子，分離、命中、光環、旋刃、接觸傷害、找最近敵人都只看附近的格子，從 O(n²) 降到約 O(n)。`TestGridMatchesBruteForce` 確保結果與逐一比對完全相同。
- **快照**：敵人 / 投射物 / 寶石的座標量化成整數（× 10，varint 約 2～3 bytes，float 固定 5 bytes）；敵人半徑只在加入時送一次種類表；血量只送「已損失百分比」（滿血不送）；每位玩家只收到畫面附近（再加 160 邊距）的實體（AOI）。玩家座標維持 float，預測需要精確值。
- **前端**：量測後發現逐個畫圓本身就很快；「合併成一條大 path」與「預先畫成小圖再 drawImage」實測都更慢，所以只做了畫面外剔除與投射物光暈圖（取代 `shadowBlur`）。量測時不要每幀 `getImageData`：Chrome 會把 canvas 改成 CPU 繪製，數字會失真（`drawBench.ts` 只在最後讀回一次）。

### 平衡模擬（`cmd/sim`）

```powershell
cd server
go run ./cmd/sim -minutes 10 -seed 7
```

用固定種子跑三種 build（飛彈流 / 光環流 / 旋刃流）的無頭模擬：AI 繞圈走位、撿附近的寶石、閃避靠太近的敵人，升級依 build 偏好選擇。輸出存活時間、等級、擊殺數、**各武器擊殺占比**與場上敵人數。調整數值後跑一次，確認沒有某個 build 過強或過弱。`TestBuildsPlayDifferently` 用 4 分鐘的模擬確保每種 build 的主力武器是它的招牌武器。

## 客戶端預測、校正與插值

20Hz 的快照如果直接畫，畫面會一格一格跳；如果等伺服器回應才移動，延遲 150ms 時按鍵會「黏黏的」。所以前端分兩種情況處理（`web/src/game/`）：

| 對象 | 做法 | 檔案 |
|---|---|---|
| **自己** | **預測**：每個輸入 tick 產生一筆輸入，立刻用與伺服器相同的 `moveStep` 在本地套用，並記在 pending；**校正**：快照回來時從伺服器位置重播 `seq > last_input_seq` 的輸入。規則一致，所以正常情況校正量是 0 | `prediction.ts`、`sim.ts` |
| **其他人** | **插值**：快照放進緩衝，畫「伺服器時間 − 100ms」的位置，在前後兩個快照間線性插值；看到的別人比實際晚 ~100ms，但完全平順 | `interpolation.ts` |

- `GameClient.ts` 把兩者串起來：每幀 `update(now)` 依累積時間產生輸入（從靜止起步時立刻產生一筆，不等下一個 tick），`view(now)` 回傳要畫的位置；自己的位置在兩個輸入 tick 之間平滑、被校正時誤差在 ~80ms 內滑過去，而不是瞬移。
- 伺服器時間用「快照 tick × 50ms」，並以最近 2 秒內延遲最小的快照估計時鐘差（`ServerClock`）。
- 相機以自己為中心，世界 3000×2000、畫面 960×640，只畫看得到的格線。
- `sim.ts` 的 `moveStep` / `normalizeInput` 必須與 `server/internal/game/world.go` 的 `MoveStep` / `NormalizeInput` 一致，否則預測會一直被校正（HUD 的 correction 會變黃）。

### 除錯工具

- **延遲模擬**：網址加 `?lag=150`，送出與收到的訊息各延遲 75ms。Chrome DevTools 的網路節流不會套用到 WebSocket，所以用這個測試預測與插值。
- **HUD**（畫面左上）：FPS、tick、RTT、players、pending inputs（還沒被伺服器確認的輸入數 ≈ RTT / 50ms）、correction（最近一次校正距離）、自己的座標。
- **DevTools console**（只在開發模式）：`__aigaming.game.view(performance.now())` 查看目前的預測 / 插值狀態。

## 後端日誌

`server/pkg/logger` 是自訂的 `slog.Handler`，開發時輸出彩色、一行一筆的易讀格式：

```
17:13:19.088 INF main.go:64 server listening addr=:8080 mode=debug version=dev
17:13:20.680 DBG GET /api/healthz status=200 latency=152µs ip=::1
17:13:20.740 WRN GET /api/nope status=404 latency=0s ip=::1
```

Gin 的請求日誌、panic recovery、`[GIN-debug]` 訊息都改走這個 logger。

| 環境變數 | 預設 | 說明 |
|---|---|---|
| `PORT` | `8080` | 監聽 port |
| `GIN_MODE` | `debug` | `debug` / `release` |
| `APP_VERSION` | `dev` | 顯示於 `/api/healthz` |
| `LOG_LEVEL` | `debug` | `debug` / `info` / `warn` / `error` |
| `LOG_FORMAT` | `pretty` | `pretty`（給人看）/ `json`（給日誌系統） |
| `LOG_COLOR` | `auto` | `auto`（終端機才上色）/ `always` / `never`；也遵循 `NO_COLOR` |
| `WS_ALLOW_JSON` | 非 release 為 `true` | 允許 `/ws?format=json`（Postman 除錯用）；release 模式預設關閉 |

## 開發流程

需求寫在 [GitHub Issues](https://github.com/s9891326/AIGaming/issues?q=is%3Aopen+label%3Aenhancement) 的卡片上（標籤 `enhancement`，編號由小到大即開發順序），一張卡片 = 一個 PR：

1. 讀編號最小的開啟中卡片，加上 `in-progress` 標籤，開 `feat/<issue>-<slug>` 分支
2. 實作 + 測試，`task check` 全綠，並實際啟動逐條驗證驗收條件
3. 開 PR（`Closes #N`），自我審查、CI 通過後 squash merge
4. Issue 自動關閉

完整步驟寫在 [`.claude/skills/card-to-pr/SKILL.md`](.claude/skills/card-to-pr/SKILL.md)，在 Claude Code 裡說「做下一張卡片」就會照這個流程執行。

## 開發路線圖

- [x] 專案骨架與開發環境（Gin、React、logger、Task、VS Code、Docker Compose）
- [x] GitHub Actions CI（server / web / proto / docker 四個 check）
- [x] Proto + 程式碼產生 + WebSocket 連線、Ping/Pong 延遲顯示
- [x] 遊戲迴圈 + 玩家移動（輸入 → tick → 快照）
- [x] 客戶端插值、預測 / 校正與相機跟隨
- [x] 敵人生成、追蹤、碰撞扣血、Game Over
- [x] 自動攻擊武器、敵人死亡、經驗寶石
- [x] 升級系統（三選一）
- [x] 更多武器 / 被動、敵人種類、隨時間增強的波次
- [x] 效能優化（空間格子、快照量化 + AOI）
- [ ] 多人合作房間

美術、英雄、技能、介面等後續規劃見 [#29](https://github.com/s9891326/AIGaming/issues/29)（#23～#28）。
