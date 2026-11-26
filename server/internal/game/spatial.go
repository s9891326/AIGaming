package game

import "math"

// gridCellSize 是空間格子的邊長。要不小於「兩隻最大敵人的半徑和」（精英 26 × 2 = 52），
// 這樣分離時只要看自己與相鄰的 3×3 格就不會漏掉重疊的對象。
const gridCellSize = 64

// enemyGrid 是敵人的空間雜湊（spatial hash）：把世界切成固定大小的格子，每格記錄裡面的敵人。
// 「找某個範圍內的敵人」只要看範圍覆蓋到的幾格，而不是掃過全部敵人 —— 碰撞從 O(n²) 降到約 O(n)。
//
// 格子存的是 *Enemy：同一個 tick 中被打死的敵人仍在格子裡（HP ≤ 0，查詢的人自行略過），
// 從 slice 移除死掉的敵人也不會讓格子失效。每個 tick 在敵人移動後重建一次（O(n)）。
type enemyGrid struct {
	cols, rows int
	cells      [][]*Enemy
	maxRadius  float64 // 目前最大的敵人半徑（查詢時擴大範圍用）
}

func (g *enemyGrid) rebuild(w *World) {
	cols := int(math.Ceil(w.cfg.Width/gridCellSize)) + 1
	rows := int(math.Ceil(w.cfg.Height/gridCellSize)) + 1
	if g.cols != cols || g.rows != rows {
		g.cols, g.rows = cols, rows
		g.cells = make([][]*Enemy, cols*rows)
	}
	for i := range g.cells {
		g.cells[i] = g.cells[i][:0] // 保留容量，避免每個 tick 重新配置記憶體
	}
	g.maxRadius = 0
	for _, e := range w.enemies {
		c := g.cellIndex(e.Pos)
		g.cells[c] = append(g.cells[c], e)
		g.maxRadius = math.Max(g.maxRadius, e.Radius)
	}
}

func (g *enemyGrid) cellCoord(v float64, n int) int {
	return min(max(int(v/gridCellSize), 0), n-1)
}

func (g *enemyGrid) cellIndex(p Vec2) int {
	return g.cellCoord(p.Y, g.rows)*g.cols + g.cellCoord(p.X, g.cols)
}

// query 對中心 center、半徑 radius 的正方形範圍所覆蓋的格子裡的每隻敵人呼叫 fn（還沒做精確的距離判斷）。
// fn 回傳 false 時提前結束。走訪順序固定（由上到下、由左到右、格內依加入順序），結果可重現。
func (g *enemyGrid) query(center Vec2, radius float64, fn func(e *Enemy) bool) {
	if len(g.cells) == 0 {
		return
	}
	x0, x1 := g.cellCoord(center.X-radius, g.cols), g.cellCoord(center.X+radius, g.cols)
	y0, y1 := g.cellCoord(center.Y-radius, g.rows), g.cellCoord(center.Y+radius, g.rows)
	for y := y0; y <= y1; y++ {
		row := g.cells[y*g.cols : (y+1)*g.cols]
		for x := x0; x <= x1; x++ {
			for _, e := range row[x] {
				if !fn(e) {
					return
				}
			}
		}
	}
}

// enemiesNear 對與圓（center, radius）相交的存活敵人呼叫 fn（已做精確的「圓與圓」判斷）。
func (w *World) enemiesNear(center Vec2, radius float64, fn func(e *Enemy) bool) {
	w.grid.query(center, radius+w.grid.maxRadius, func(e *Enemy) bool {
		if e.HP <= 0 || dist(e.Pos, center) >= radius+e.Radius {
			return true
		}
		return fn(e)
	})
}
