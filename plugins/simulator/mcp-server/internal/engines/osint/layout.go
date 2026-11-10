package osint

import "math"

// layoutOf places the entities of a case so that what is connected stands
// together. Each connected group is drawn as a tree growing to the right from
// its most connected entity: an entity's neighbours are one column on, and a
// parent sits level with the middle of its children. Groups are stacked one
// under another, and entities linked to nothing fill rows at the bottom.
//
// Links and payments both count as connections. A link that is not part of
// the tree is still drawn; it just was not used to place anyone.
func layoutOf(c *Case, ix *index) map[string][2]int {
	adj := map[string][]string{}
	seenPair := map[[2]string]bool{}
	link := func(a, b string) {
		if a == b || ix.entities[a] == nil || ix.entities[b] == nil || seenPair[[2]string{a, b}] {
			return
		}
		seenPair[[2]string{a, b}], seenPair[[2]string{b, a}] = true, true
		adj[a] = append(adj[a], b)
		adj[b] = append(adj[b], a)
	}
	for _, r := range c.Relations {
		link(r.From, r.To)
	}
	for _, t := range c.Transactions {
		from, to := ix.accounts[t.From], ix.accounts[t.To]
		if from != nil && to != nil {
			link(from.Owner, to.Owner)
		}
	}

	pos := map[string][2]int{}
	placed := map[string]bool{}
	row := 0.0 // the next free row, in units of planRowStep
	var alone []string
	for _, e := range c.Entities {
		if placed[e.ID] || ix.entities[e.ID] == nil {
			continue
		}
		if len(adj[e.ID]) == 0 {
			placed[e.ID] = true
			alone = append(alone, e.ID)
			continue
		}
		// the group this entity is in, and its most connected member
		group := []string{e.ID}
		inGroup := map[string]bool{e.ID: true}
		for i := 0; i < len(group); i++ {
			for _, n := range adj[group[i]] {
				if !inGroup[n] {
					inGroup[n] = true
					group = append(group, n)
				}
			}
		}
		root := group[0]
		for _, id := range group {
			if len(adj[id]) > len(adj[root]) {
				root = id
			}
		}
		// a breadth-first tree from the root: each entity hangs from the first
		// placed neighbour that reached it, so it stands one column from it
		children := map[string][]string{}
		depth := map[string]int{root: 0}
		order := []string{root}
		placed[root] = true
		for i := 0; i < len(order); i++ {
			for _, n := range adj[order[i]] {
				if !placed[n] {
					placed[n] = true
					depth[n] = depth[order[i]] + 1
					children[order[i]] = append(children[order[i]], n)
					order = append(order, n)
				}
			}
		}
		// rows: a leaf takes the next free one, a parent the middle of its children
		rows := map[string]float64{}
		var assign func(id string) float64
		assign = func(id string) float64 {
			kids := children[id]
			if len(kids) == 0 {
				rows[id] = row
				row++
				return rows[id]
			}
			first := assign(kids[0])
			last := first
			for _, k := range kids[1:] {
				last = assign(k)
			}
			rows[id] = (first + last) / 2
			return rows[id]
		}
		assign(root)
		for _, id := range order {
			pos[id] = [2]int{depth[id] * planColStep, snap(rows[id] * planRowStep)}
		}
		row++ // a gap before the next group
	}
	const perRow = 6
	for i, id := range alone {
		pos[id] = [2]int{(i % perRow) * planColStep, snap((row + float64(i/perRow)) * planRowStep)}
	}
	return pos
}

// snap rounds a coordinate to the layer's grid.
func snap(v float64) int { return int(math.Round(v/layerCell)) * layerCell }
