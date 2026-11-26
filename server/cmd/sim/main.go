// Command sim 用無頭模擬比較不同 build：固定種子、AI 繞圈走位、依偏好選升級，
// 印出存活時間、等級、擊殺數與各武器的擊殺占比。用來調整數值與驗證平衡。
//
//	go run ./cmd/sim -minutes 5 -seed 42
package main

import (
	"flag"
	"fmt"
	"os"
	"sort"
	"strings"
	"text/tabwriter"

	"aigaming/server/internal/game"
)

func main() {
	minutes := flag.Float64("minutes", 5, "simulated minutes per build")
	seed := flag.Uint64("seed", 42, "random seed (same seed = same result)")
	flag.Parse()

	cfg := game.DefaultWorldConfig
	cfg.Seed = *seed

	tw := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "build\t存活\t等級\t擊殺\t各武器擊殺占比\t場上敵人（最多）\t升級順序（前 8 個）")
	for _, b := range game.DefaultBuilds {
		r := game.SimulateBuild(cfg, b, *minutes*60)
		survived := fmt.Sprintf("%d:%02d", int(r.Survived)/60, int(r.Survived)%60)
		if !r.Died {
			survived += "（活到最後）"
		}
		ups := r.Upgrades
		if len(ups) > 8 {
			ups = ups[:8]
		}
		fmt.Fprintf(tw, "%s\t%s\t%d\t%d\t%s\t%d\t%s\n",
			r.Build, survived, r.Level, r.Kills, shares(r.KillsBy, r.Kills), r.PeakAlive, strings.Join(ups, ","))
	}
	_ = tw.Flush()
}

func shares(by map[string]int, total int) string {
	if total == 0 {
		return "-"
	}
	names := make([]string, 0, len(by))
	for n := range by {
		names = append(names, n)
	}
	sort.Slice(names, func(i, j int) bool { return by[names[i]] > by[names[j]] })
	parts := make([]string, len(names))
	for i, n := range names {
		parts[i] = fmt.Sprintf("%s %d%%", n, by[n]*100/total)
	}
	return strings.Join(parts, " / ")
}
