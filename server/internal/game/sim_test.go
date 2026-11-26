package game

import "testing"

// TestBuildsPlayDifferently 用無頭模擬（固定種子、AI 走位）比較三種 build：
// 每種 build 的主力（擊殺最多的）武器必須是它的招牌武器，且占過半擊殺。
// 完整的 10 分鐘比較用 `go run ./cmd/sim -minutes 10`。
func TestBuildsPlayDifferently(t *testing.T) {
	if testing.Short() {
		t.Skip("simulation")
	}
	cfg := DefaultWorldConfig
	cfg.Seed = 7
	want := map[string]string{"飛彈流": "magic_bolt", "光環流": "aura", "旋刃流": "orbit"}

	for _, b := range DefaultBuilds {
		r := SimulateBuild(cfg, b, 240)
		top, topKills := "", 0
		for name, k := range r.KillsBy {
			if k > topKills {
				top, topKills = name, k
			}
		}
		t.Logf("%s: survived %.0fs level %d kills %d by %v peak enemies %d", r.Build, r.Survived, r.Level, r.Kills, r.KillsBy, r.PeakAlive)
		if top != want[b.Name] || topKills*2 < r.Kills {
			t.Errorf("%s: main weapon %s (%d of %d kills), want %s with > 50%%", b.Name, top, topKills, r.Kills, want[b.Name])
		}
	}
}

func TestSimulationIsDeterministic(t *testing.T) {
	cfg := DefaultWorldConfig
	cfg.Seed = 99
	a := SimulateBuild(cfg, DefaultBuilds[1], 60)
	b := SimulateBuild(cfg, DefaultBuilds[1], 60)
	if a.Kills != b.Kills || a.Level != b.Level || a.Survived != b.Survived || len(a.Upgrades) != len(b.Upgrades) {
		t.Fatalf("same seed gave different results: %+v vs %+v", a, b)
	}
}
