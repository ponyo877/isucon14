// ref: https://qiita.com/EmptyBox_0/items/2f8e3cf7bd44e0f789d5
package mincostflow

import "container/heap"

type MinCostFlow struct {
	n   int
	pos [][2]int
	g   [][]_Edge
}

type _Edge struct {
	to   int
	rev  int
	capa int
	cost int
}

type Edge struct {
	from int
	to   int
	capa int
	flow int
	cost int
}

func (e Edge) From() int {
	return e.from
}

func (e Edge) To() int {
	return e.to
}

func (e Edge) Flow() int {
	return e.flow
}

func NewMinCostFlow(n int) *MinCostFlow {
	return &MinCostFlow{n: n, g: make([][]_Edge, n)}
}

func (mcf *MinCostFlow) AddEdge(from, to, capa, cost int) int {
	m := len(mcf.pos)
	mcf.pos = append(mcf.pos, [2]int{from, len(mcf.g[from])})
	mcf.g[from] = append(mcf.g[from], _Edge{to, len(mcf.g[to]), capa, cost})
	mcf.g[to] = append(mcf.g[to], _Edge{from, len(mcf.g[from]) - 1, 0, -cost})
	return m
}

func (mcf *MinCostFlow) GetEdge(i int) Edge {
	e := mcf.g[mcf.pos[i][0]][mcf.pos[i][1]]
	re := mcf.g[e.to][e.rev]
	return Edge{mcf.pos[i][0], e.to, e.capa + re.capa, re.capa, e.cost}
}

func (mcf *MinCostFlow) Edges() []Edge {
	m := len(mcf.pos)
	res := make([]Edge, m)
	for i := 0; i < m; i++ {
		res[i] = mcf.GetEdge(i)
	}
	return res
}

func (mcf *MinCostFlow) Flow(s, t int) [2]int {
	res := mcf.Slope(s, t)
	return res[len(res)-1]
}

func (mcf *MinCostFlow) FlowL(s, t, flowLim int) [2]int {
	res := mcf.SlopeL(s, t, flowLim)
	return res[len(res)-1]
}

func (mcf *MinCostFlow) Slope(s, t int) [][2]int {
	return mcf.SlopeL(s, t, int(1e+18))
}

func (mcf *MinCostFlow) SlopeL(s, t, flowLim int) [][2]int {
	dual, dist := make([]int, mcf.n), make([]int, mcf.n)
	pv, pe := make([]int, mcf.n), make([]int, mcf.n)
	vis := make([]bool, mcf.n)
	dualRef := func() bool {
		for i := 0; i < mcf.n; i++ {
			dist[i], pv[i], pe[i] = int(1e+18), -1, -1
			vis[i] = false
		}
		pq := make(PriorityQueue, 0)
		heap.Init(&pq)
		item := &Item{value: s, priority: 0}
		dist[s] = 0
		heap.Push(&pq, item)
		for pq.Len() != 0 {
			v := heap.Pop(&pq).(*Item).value
			if vis[v] {
				continue
			}
			vis[v] = true
			if v == t {
				break
			}
			for i := 0; i < len(mcf.g[v]); i++ {
				e := mcf.g[v][i]
				if vis[e.to] || e.capa == 0 {
					continue
				}
				cost := e.cost - dual[e.to] + dual[v]
				if dist[e.to]-dist[v] > cost {
					dist[e.to] = dist[v] + cost
					pv[e.to] = v
					pe[e.to] = i
					item := &Item{value: e.to, priority: dist[e.to]}
					heap.Push(&pq, item)
				}
			}
		}
		if !vis[t] {
			return false
		}
		for v := 0; v < mcf.n; v++ {
			if !vis[v] {
				continue
			}
			dual[v] -= dist[t] - dist[v]
		}
		return true
	}
	flow, cost, prevCost := 0, 0, -1
	res := make([][2]int, 0, mcf.n)
	res = append(res, [2]int{flow, cost})
	for flow < flowLim {
		if !dualRef() {
			break
		}
		c := flowLim - flow
		for v := t; v != s; v = pv[v] {
			c = mcf.Min(c, mcf.g[pv[v]][pe[v]].capa)
		}
		for v := t; v != s; v = pv[v] {
			mcf.g[pv[v]][pe[v]].capa -= c
			mcf.g[v][mcf.g[pv[v]][pe[v]].rev].capa += c
		}
		d := -dual[s]
		flow += c
		cost += c * d
		if prevCost == d {
			res = res[:len(res)-1]
		}
		res = append(res, [2]int{flow, cost})
		prevCost = cost
	}
	return res
}

func (mcf *MinCostFlow) Min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

type Item struct {
	value    int
	priority int
	index    int
}

type PriorityQueue []*Item

func (pq PriorityQueue) Len() int { return len(pq) }
func (pq PriorityQueue) Less(i, j int) bool {
	return pq[i].priority < pq[j].priority
}

func (pq PriorityQueue) Swap(i, j int) {
	pq[i], pq[j] = pq[j], pq[i]
	pq[i].index = i
	pq[j].index = j
}

func (pq *PriorityQueue) Push(x interface{}) {
	n := len(*pq)
	item := x.(*Item)
	item.index = n
	*pq = append(*pq, item)
}

func (pq *PriorityQueue) Pop() interface{} {
	old := *pq
	n := len(old)
	item := old[n-1]
	old[n-1] = nil
	item.index = -1
	*pq = old[0 : n-1]
	return item
}

func (pq *PriorityQueue) update(item *Item, value int, priority int) {
	item.value = value
	item.priority = priority
	heap.Fix(pq, item.index)
}
