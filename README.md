# AIGaming

一個 **survivors-like**（割草 / 倖存者類）網頁遊戲的學習專案，目的是了解即時網頁遊戲如何運作：

- **前端**：React + TypeScript + Vite（`web/`），遊戲畫面用 Canvas 2D 繪製
- **後端**：Go + Gin（`server/`），採用伺服器權威（server-authoritative）的遊戲迴圈
- **通訊**：WebSocket 長連線，訊息格式使用 Protocol Buffers（`proto/`）

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
├── proto/    # .proto 協議定義（前後端共用）
├── server/   # Go + Gin 後端
└── web/      # React 前端
```

## 快速開始

需求：Go 1.25+、Node.js 22+

開兩個終端機，分別啟動後端與前端：

```powershell
# 終端機 1：後端（http://localhost:8080）
cd server
go run ./cmd/server
```

```powershell
# 終端機 2：前端（http://localhost:5173）
cd web
npm install
npm run dev
```

打開 http://localhost:5173 ，看到「伺服器狀態 ● ok」就代表前後端串接成功。

開發時 Vite 會把 `/api`、`/ws` 代理到後端，因此瀏覽器只會看到同一個來源，後端不需要設定 CORS。

### 常用指令

| 位置 | 指令 | 說明 |
|---|---|---|
| `server/` | `go test ./...` | 執行後端測試 |
| `server/` | `go vet ./...` | 靜態檢查 |
| `web/` | `npm run build` | 型別檢查 + 打包 |
| `web/` | `npm run lint` | oxlint 檢查 |

### 環境變數（後端）

| 變數 | 預設 | 說明 |
|---|---|---|
| `PORT` | `8080` | 監聽 port |
| `GIN_MODE` | `debug` | `debug` / `release` |
| `APP_VERSION` | `dev` | 顯示於 `/api/healthz` |

## 開發路線圖

每個功能一個 PR，逐步迭代：

- [ ] PR 1：專案骨架（Gin `/api/healthz`、Vite React、開發代理）
- [ ] PR 2：Proto + 程式碼產生 + WebSocket 連線、Ping/Pong 延遲顯示
- [ ] PR 3：遊戲迴圈 + 玩家移動（輸入 → tick → 快照）
- [ ] PR 4：客戶端插值與相機跟隨
- [ ] PR 5：敵人生成、追蹤、碰撞扣血、Game Over
- [ ] PR 6：自動攻擊武器、敵人死亡、經驗寶石
- [ ] PR 7：升級系統（三選一）
- [ ] PR 8：更多武器 / 被動、隨時間增強的波次
- [ ] PR 9：效能優化（空間格子、快照壓縮）
- [ ] PR 10：GitHub Actions CI
- [ ] 之後：多人合作房間
