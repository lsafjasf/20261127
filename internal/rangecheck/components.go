package rangecheck

import (
	"math"
	"sort"
)

type component struct {
	rect  Rect
	count int
	sum   int64
	min   int
	max   int
}

type componentGroup struct {
	members []int
	rect    Rect
	count   int
	sum     int64
	min     int
	max     int
}

func detectRegions(frame Frame, settings Settings, bounds LevelBounds) ([]Region, error) {
	integral := buildIntegral(frame.Y, frame.Width, frame.Height, frame.StrideY)
	var regions []Region

	for _, kind := range []RegionKind{KindSuperBlack, KindSuperWhite} {
		mask := make([]uint8, frame.Width*frame.Height)
		for y := 0; y < frame.Height; y++ {
			for x := 0; x < frame.Width; x++ {
				v := int(frame.Y[y*frame.StrideY+x])
				if (kind == KindSuperBlack && v < bounds.Black) ||
					(kind == KindSuperWhite && v > bounds.White) {
					mask[y*frame.Width+x] = 1
				}
			}
		}
		components := connectedComponents(mask, frame.Y, frame.Width, frame.Height, frame.StrideY)
		components = filterAndRankComponents(components, settings.MinRegionPixels)
		groups := groupComponents(components, frame.Width, frame.Height)
		for gi, group := range groups {
			region := buildRegion(len(regions)+gi+1, kind, group, components, frame, integral)
			classifyRegion(&region, frame, settings)
			regions = append(regions, region)
		}
	}

	sort.Slice(regions, func(i, j int) bool {
		if regions[i].PixelCount != regions[j].PixelCount {
			return regions[i].PixelCount > regions[j].PixelCount
		}
		if regions[i].Kind != regions[j].Kind {
			return regions[i].Kind < regions[j].Kind
		}
		return regions[i].ID < regions[j].ID
	})
	for i := range regions {
		regions[i].ID = i + 1
	}
	if len(regions) > 12 {
		regions = regions[:12]
	}
	return regions, nil
}

func connectedComponents(mask, plane []byte, w, h, stride int) []component {
	visited := make([]uint8, w*h)
	var comps []component
	var stack []int
	for start := 0; start < w*h; start++ {
		if mask[start] == 0 || visited[start] != 0 {
			continue
		}
		visited[start] = 1
		stack = append(stack[:0], start)
		c := component{min: 256, max: -1}
		minX, minY, maxX, maxY := w, h, -1, -1
		for len(stack) > 0 {
			idx := stack[len(stack)-1]
			stack = stack[:len(stack)-1]
			x, y := idx%w, idx/w
			code := int(plane[y*stride+x])
			c.count++
			c.sum += int64(code)
			if code < c.min {
				c.min = code
			}
			if code > c.max {
				c.max = code
			}
			if x < minX {
				minX = x
			}
			if x > maxX {
				maxX = x
			}
			if y < minY {
				minY = y
			}
			if y > maxY {
				maxY = y
			}
			neighbors := [4]int{idx - 1, idx + 1, idx - w, idx + w}
			for ni, nidx := range neighbors {
				if nidx < 0 || nidx >= w*h || visited[nidx] != 0 || mask[nidx] == 0 {
					continue
				}
				if ni == 0 && x == 0 {
					continue
				}
				if ni == 1 && x == w-1 {
					continue
				}
				visited[nidx] = 1
				stack = append(stack, nidx)
			}
		}
		if c.count > 0 && maxX >= minX && maxY >= minY {
			c.rect = Rect{X: minX, Y: minY, Width: maxX - minX + 1, Height: maxY - minY + 1}
			comps = append(comps, c)
		}
	}
	return comps
}

func filterAndRankComponents(comps []component, minPixels int) []component {
	filtered := comps[:0]
	for _, c := range comps {
		if c.count >= max(3, minPixels/4) {
			filtered = append(filtered, c)
		}
	}
	sort.Slice(filtered, func(i, j int) bool { return filtered[i].count > filtered[j].count })
	if len(filtered) > 600 {
		filtered = filtered[:600]
	}
	return filtered
}

func groupComponents(comps []component, w, h int) []componentGroup {
	if len(comps) == 0 {
		return nil
	}
	uf := newUnionFind(len(comps))
	for i := 0; i < len(comps); i++ {
		for j := i + 1; j < len(comps); j++ {
			if rectsShouldGroup(comps[i].rect, comps[j].rect, w, h) {
				uf.union(i, j)
			}
		}
	}
	groupMap := make(map[int]*componentGroup)
	for i, c := range comps {
		root := uf.find(i)
		g := groupMap[root]
		if g == nil {
			g = &componentGroup{min: 256, max: -1}
			groupMap[root] = g
		}
		g.members = append(g.members, i)
		g.rect = unionRect(g.rect, c.rect, len(g.members) == 1)
		g.count += c.count
		g.sum += c.sum
		if c.min < g.min {
			g.min = c.min
		}
		if c.max > g.max {
			g.max = c.max
		}
	}
	groups := make([]componentGroup, 0, len(groupMap))
	for _, g := range groupMap {
		groups = append(groups, *g)
	}
	sort.Slice(groups, func(i, j int) bool { return groups[i].count > groups[j].count })
	if len(groups) > 80 {
		groups = groups[:80]
	}
	return groups
}

func rectsShouldGroup(a, b Rect, w, h int) bool {
	maxHeight := max(a.Height, b.Height)
	gapX := clampInt(maxHeight*2, 8, max(24, w/12))
	gapY := clampInt(maxHeight/2, 3, max(8, h/80))
	ea := Rect{X: a.X - gapX, Y: a.Y - gapY, Width: a.Width + 2*gapX, Height: a.Height + 2*gapY}
	eb := Rect{X: b.X - gapX, Y: b.Y - gapY, Width: b.Width + 2*gapX, Height: b.Height + 2*gapY}
	return rectsOverlap(ea, eb)
}

func rectsOverlap(a, b Rect) bool {
	return a.X < b.X+b.Width && a.X+a.Width > b.X &&
		a.Y < b.Y+b.Height && a.Y+a.Height > b.Y
}

func unionRect(a, b Rect, reset bool) Rect {
	if reset {
		return b
	}
	x1 := minInt(a.X, b.X)
	y1 := minInt(a.Y, b.Y)
	x2 := maxInt(a.X+a.Width, b.X+b.Width)
	y2 := maxInt(a.Y+a.Height, b.Y+b.Height)
	return Rect{X: x1, Y: y1, Width: x2 - x1, Height: y2 - y1}
}

func buildRegion(id int, kind RegionKind, group componentGroup, all []component, frame Frame, integral []int64) Region {
	r := group.rect
	mean := 0.0
	if group.count > 0 {
		mean = float64(group.sum) / float64(group.count)
	}
	fill := float64(group.count) / float64(max(1, r.Area()))
	pad := clampInt(minInt(r.Width, r.Height)/3, 4, 32)
	contextMean := rectangleContextMean(integral, frame.Width, frame.Height, r, pad)
	contrast := math.Abs(contextMean - mean)
	bottom := float64(r.Y) >= float64(frame.Height)*0.66 ||
		float64(r.Y+r.Height) >= float64(frame.Height)*0.86
	thin := minInt(r.Width, r.Height) <= max(5, frame.Height/140)
	edgeLike := (thin && contrast >= 24) || (fill < 0.08 && contrast >= 32)
	return Region{
		ID: id, Plane: "Y", Kind: kind, Rect: r, PixelCount: group.count,
		MinCode: group.min, MaxCode: group.max, MeanCode: mean,
		ContextMean: contextMean, LocalContrast: contrast, FillRatio: fill,
		BottomBand: bottom, EdgeLike: edgeLike, ComponentCount: len(group.members),
		Confidence: 0.2,
	}
}

func buildIntegral(plane []byte, w, h, stride int) []int64 {
	iw, ih := w+1, h+1
	integral := make([]int64, iw*ih)
	for y := 0; y < h; y++ {
		var rowSum int64
		for x := 0; x < w; x++ {
			rowSum += int64(plane[y*stride+x])
			idx := (y+1)*iw + (x + 1)
			integral[idx] = integral[y*iw+(x+1)] + rowSum
		}
	}
	return integral
}

func rectSum(integral []int64, iw, w, h int, r Rect) int64 {
	x0 := clampInt(r.X, 0, w)
	y0 := clampInt(r.Y, 0, h)
	x1 := clampInt(r.X+r.Width, 0, w)
	y1 := clampInt(r.Y+r.Height, 0, h)
	if x1 <= x0 || y1 <= y0 {
		return 0
	}
	return integral[y1*iw+x1] - integral[y0*iw+x1] - integral[y1*iw+x0] + integral[y0*iw+x0]
}

func rectangleContextMean(integral []int64, w, h int, r Rect, pad int) float64 {
	iw := w + 1
	outer := Rect{X: r.X - pad, Y: r.Y - pad, Width: r.Width + 2*pad, Height: r.Height + 2*pad}
	outer.X = maxInt(0, outer.X)
	outer.Y = maxInt(0, outer.Y)
	if outer.X+outer.Width > w {
		outer.Width = w - outer.X
	}
	if outer.Y+outer.Height > h {
		outer.Height = h - outer.Y
	}
	outerSum := rectSum(integral, iw, w, h, outer)
	innerSum := rectSum(integral, iw, w, h, r)
	outerArea := max(1, outer.Area()-r.Area())
	return float64(outerSum-innerSum) / float64(outerArea)
}

type unionFind struct {
	parent []int
	rank   []int
}

func newUnionFind(n int) *unionFind {
	u := &unionFind{parent: make([]int, n), rank: make([]int, n)}
	for i := range u.parent {
		u.parent[i] = i
	}
	return u
}

func (u *unionFind) find(x int) int {
	for u.parent[x] != x {
		u.parent[x] = u.parent[u.parent[x]]
		x = u.parent[x]
	}
	return x
}

func (u *unionFind) union(a, b int) {
	ra, rb := u.find(a), u.find(b)
	if ra == rb {
		return
	}
	if u.rank[ra] < u.rank[rb] {
		ra, rb = rb, ra
	}
	u.parent[rb] = ra
	if u.rank[ra] == u.rank[rb] {
		u.rank[ra]++
	}
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}
