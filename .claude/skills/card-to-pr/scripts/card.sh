#!/usr/bin/env bash
# GitHub Project 卡片操作小工具（需要 gh，且 token 具備 project 權限：gh auth refresh -s project）
#
#   card.sh list                         列出所有卡片：issue 編號、狀態、標題
#   card.sh next                         印出第一張 Todo 卡片的 issue 編號（沒有則不輸出、exit 1）
#   card.sh status <issue> <狀態>         把卡片移到指定狀態，例如 "In Progress"、"Done"
#   card.sh add <issue>                  把 issue 加進 Project（狀態設為 Todo）
#
# 可用環境變數覆寫：CARD_OWNER、CARD_PROJECT（Project 編號）
set -euo pipefail

OWNER="${CARD_OWNER:-s9891326}"
PROJECT="${CARD_PROJECT:-__PROJECT_NUMBER__}"
REPO="${CARD_REPO:-s9891326/AIGaming}"

# Windows 上剛安裝的 gh 可能還不在目前 shell 的 PATH 裡。
if ! command -v gh >/dev/null 2>&1 && [ -x "/c/Program Files/GitHub CLI/gh.exe" ]; then
  export PATH="$PATH:/c/Program Files/GitHub CLI"
fi

items_json() {
  gh project item-list "$PROJECT" --owner "$OWNER" --format json --limit 500
}

# 這台機器沒有 jq，JSON 一律交給 node 處理。
item_id_of() { # <issue>
  items_json | node -e '
    let s=""; process.stdin.on("data",d=>s+=d).on("end",()=>{
      const it = JSON.parse(s).items.find(i => i.content?.number === Number(process.argv[1]));
      if (it) console.log(it.id);
    });' "$1"
}

cmd_list() {
  items_json | node -e '
    let s=""; process.stdin.on("data",d=>s+=d).on("end",()=>{
      for (const i of JSON.parse(s).items) {
        const n = i.content?.number ? "#" + i.content.number : "(draft)";
        console.log(`${n.padEnd(6)} ${(i.status ?? "-").padEnd(12)} ${i.title}`);
      }
    });'
}

cmd_next() {
  local n
  n=$(items_json | node -e '
    let s=""; process.stdin.on("data",d=>s+=d).on("end",()=>{
      const it = JSON.parse(s).items.find(i => (i.status ?? "Todo") === "Todo" && i.content?.number);
      if (it) console.log(it.content.number);
    });')
  [ -n "$n" ] || exit 1
  echo "$n"
}

cmd_status() { # <issue> <status>
  local issue="$1" status="$2"
  local project_id field_json field_id option_id item_id
  project_id=$(gh project view "$PROJECT" --owner "$OWNER" --format json --jq .id)
  field_json=$(gh project field-list "$PROJECT" --owner "$OWNER" --format json)
  field_id=$(node -e 'const f=JSON.parse(process.argv[1]).fields.find(f=>f.name==="Status"); console.log(f.id)' "$field_json")
  option_id=$(node -e '
    const f=JSON.parse(process.argv[1]).fields.find(f=>f.name==="Status");
    const o=f.options.find(o=>o.name.toLowerCase()===process.argv[2].toLowerCase());
    if(!o){console.error("unknown status: "+process.argv[2]+"; options: "+f.options.map(o=>o.name).join(", "));process.exit(1)}
    console.log(o.id)' "$field_json" "$status")
  item_id=$(item_id_of "$issue")
  if [ -z "$item_id" ]; then
    echo "issue #$issue is not on project $PROJECT; add it with: card.sh add $issue" >&2
    exit 1
  fi
  gh project item-edit --id "$item_id" --project-id "$project_id" \
    --field-id "$field_id" --single-select-option-id "$option_id" >/dev/null
  echo "#$issue → $status"
}

cmd_add() { # <issue>
  gh project item-add "$PROJECT" --owner "$OWNER" --url "https://github.com/$REPO/issues/$1" >/dev/null
  cmd_status "$1" "Todo"
}

case "${1:-}" in
  list) cmd_list ;;
  next) cmd_next ;;
  status) [ $# -eq 3 ] || { echo "usage: card.sh status <issue> <status>" >&2; exit 2; }; cmd_status "$2" "$3" ;;
  add) [ $# -eq 2 ] || { echo "usage: card.sh add <issue>" >&2; exit 2; }; cmd_add "$2" ;;
  *) sed -n '2,9p' "$0"; exit 2 ;;
esac
