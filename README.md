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
