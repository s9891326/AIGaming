---
name: card-to-pr
description: AIGaming 的功能開發流程：從 GitHub Issue 卡片讀需求 → 開分支實作 → 自行測試 → 開 PR → 驗證後自行合併 → Issue 自動關閉。使用時機：使用者說「做下一張卡片」「開發 #N」「繼續下一個功能」，或要新增功能卡片時。
---

# Card → PR 開發流程

一張卡片（GitHub Issue，標籤 `enhancement`）= 一個分支 = 一個 PR。需求只從 Issue 讀，不憑記憶。

## 固定資訊

| 項目 | 值 |
|---|---|
| Repo | `s9891326/AIGaming` |
| 卡片 | 開啟中、標籤 `enhancement` 的 Issue；**編號由小到大**就是開發順序 |
| 開發中 | 加上標籤 `in-progress` |
| 完成 | PR 內文寫 `Closes #N`，合併後 Issue 自動關閉 |
| 合併方式 | squash merge，合併後刪除分支 |

Windows 上剛裝好的 gh 若不在 PATH：`export PATH="$PATH:/c/Program Files/GitHub CLI"`。

## 流程

### 1. 選卡片、讀需求

```bash
git switch main && git pull --ff-only
gh issue list --state open --label enhancement --json number,title,labels \
  --jq 'sort_by(.number) | .[] | "#\(.number) [\([.labels[].name] | join(","))] \(.title)"'
gh issue view <N> --comments
```

- 使用者指定 `#N` 就做 `#N`；否則做**編號最小的開啟中卡片**。若已有 `in-progress` 的卡片，先確認它是否有未完成的分支 / PR，優先把它做完。
- 先檢查工作目錄是乾淨的（`git status`）；有未提交的變更就停下來問使用者。
- 把 Issue 的「需求」「驗收條件」「不在範圍內」當成這次工作的完整規格。
- 需求有歧義或互相矛盾時：在 Issue 留言寫下疑問（`gh issue comment`），並停下來問使用者，不要猜。
- 驗收條件必須可驗證；太模糊就先補成具體條件（`gh issue edit --body-file`）再開始。

### 2. 開始開發

```bash
gh issue edit <N> --add-label in-progress
git switch -c feat/<N>-<簡短英文-slug>
```

- 只做卡片範圍內的事。順手發現的其他問題 → 開新卡片（見「新增卡片」），不要混進這個 PR。
- commit 小而有意義，訊息用 Conventional Commits（`feat:`、`fix:`、`chore:`、`docs:`、`test:`），結尾加上目前環境要求的 Co-Authored-By。
- 新的邏輯要有測試（Go：`_test.go`；前端之後有測試框架再補）。
- 程式碼註解與文件用繁體中文；識別字用英文。
- 完成時更新 README 的路線圖勾選與相關說明。

### 3. 自行測試（全部通過才可以開 PR）

1. `task check`：lint + format + test + build，必須全綠。
2. 實際跑起來驗證：
   - 開發模式：`task dev`（或分別 `task dev:server`、`task dev:web`），打開 http://localhost:5173。
   - 部署模式：`task up`，打開 http://localhost:3000，`docker compose ps` 兩個服務都 healthy，`docker compose logs server` 沒有錯誤。
3. 用瀏覽器逐條驗證 Issue 的「驗收條件」，記下怎麼驗、看到什麼（必要時截圖）。
4. 測完 `task down`，並停止自己啟動的背景程序。

任何一項失敗 → 修好再從頭測。修不好 → 不開 PR（或開 Draft PR），回報使用者卡在哪裡。

### 4. 開 PR

```bash
git push -u origin HEAD
gh pr create --title "<type>: <摘要> (#N)" --body-file <寫好的內文檔>
```

內文依照 `.github/pull_request_template.md`：
- 第一行 `Closes #N`（合併後自動關閉 Issue）。
- 「驗收結果」逐條對應 Issue 的驗收條件，寫實際驗證結果，不寫「應該可以」。
- 結尾加上目前環境要求的 PR 署名行。

### 5. 自我審查並合併

1. `gh pr diff` 從頭讀一遍：有沒有除錯殘留、範圍外的變更、敏感資訊（.env、token）。
2. 有 CI 時：`gh pr checks --watch`，必須全部通過。CI 失敗 → 修正、推送、重跑，不可跳過。
3. 合併：

```bash
gh pr merge --squash --delete-branch
git switch main && git pull --ff-only
gh issue view <N> --json state,labels   # 確認 state 為 CLOSED
gh issue edit <N> --remove-label in-progress
```

不可合併的情況（改為停下來回報使用者）：
- 測試或 CI 沒過、驗收條件有任何一條沒達成。
- 變更超出卡片範圍，或需要使用者做決定（技術選型改變、破壞性變更、刪資料）。

### 6. 回報

用 2～4 句話告訴使用者：完成了哪張卡片、PR 連結、怎麼驗證的、下一張卡片是什麼。

## 新增卡片

```bash
gh issue create --title "[Feature] <標題>" --label enhancement --body-file <內文檔>
```

內文照 `.github/ISSUE_TEMPLATE/feature.yml` 的段落寫：目標、需求、驗收條件（勾選清單，最後一項固定是「`task check` 通過」）、不在範圍內、技術筆記。
新卡片會排在最後；要插隊時告訴使用者，由使用者決定順序。
