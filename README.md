# AIGaming

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
├── proto/              # .proto 協議定義（前後端共用）
├── server/             # Go + Gin 後端
│   ├── cmd/server/     #   進入點
│   ├── internal/       #   專案內部套件（config、httpserver…）
│   └── pkg/logger/     #   可重用的小套件：彩色 slog logger
├── web/                # React 前端（nginx.conf 給 docker 用）
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
| `task check` | lint + format 檢查 + test + build，開 PR 前必跑 |
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

## 開發流程

需求寫在 [GitHub Project 看板](https://github.com/s9891326?tab=projects)的卡片（Issue）上，一張卡片 = 一個 PR：

1. 從看板最上面的 Todo 卡片讀需求，移到 In Progress，開 `feat/<issue>-<slug>` 分支
2. 實作 + 測試，`task check` 全綠，並實際啟動逐條驗證驗收條件
3. 開 PR（`Closes #N`），自我審查、CI 通過後 squash merge
4. 卡片自動移到 Done

完整步驟寫在 [`.claude/skills/card-to-pr/SKILL.md`](.claude/skills/card-to-pr/SKILL.md)，在 Claude Code 裡說「做下一張卡片」就會照這個流程執行。

## 開發路線圖

- [x] 專案骨架與開發環境（Gin、React、logger、Task、VS Code、Docker Compose）
- [ ] GitHub Actions CI
- [ ] Proto + 程式碼產生 + WebSocket 連線、Ping/Pong 延遲顯示
- [ ] 遊戲迴圈 + 玩家移動（輸入 → tick → 快照）
- [ ] 客戶端插值與相機跟隨
- [ ] 敵人生成、追蹤、碰撞扣血、Game Over
- [ ] 自動攻擊武器、敵人死亡、經驗寶石
- [ ] 升級系統（三選一）
- [ ] 更多武器 / 被動、隨時間增強的波次
- [ ] 效能優化（空間格子、快照壓縮）
- [ ] 多人合作房間
