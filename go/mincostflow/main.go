package mincostflow

import "fmt"

// お試し用 packageをmainに換えれば動作確認できる
// ref: https://qiita.com/drken/items/e805e3f514acceb87602#1-2-%E9%87%8D%E3%81%BF%E3%81%A4%E3%81%8D%E6%9C%80%E5%A4%A7%E4%BA%8C%E9%83%A8%E3%83%9E%E3%83%83%E3%83%81%E3%83%B3%E3%82%B0%E5%95%8F%E9%A1%8C
func main() {
	n := 8
	mcf := NewMinCostFlow(n)
	mcf.AddEdge(0, 1, 1, 0)
	mcf.AddEdge(0, 2, 1, 0)
	mcf.AddEdge(0, 3, 1, 0)
	mcf.AddEdge(1, 4, 1, -2)
	mcf.AddEdge(1, 5, 1, -4)
	mcf.AddEdge(1, 6, 1, -5)
	mcf.AddEdge(2, 4, 1, -5)
	mcf.AddEdge(2, 5, 1, -1)
	mcf.AddEdge(2, 6, 1, -3)
	mcf.AddEdge(3, 4, 1, -6)
	mcf.AddEdge(3, 5, 1, -2)
	mcf.AddEdge(3, 6, 1, -3)
	mcf.AddEdge(4, 7, 1, 0)
	mcf.AddEdge(5, 7, 1, 0)
	mcf.AddEdge(6, 7, 1, 0)
	res := mcf.FlowL(0, n-1, 3)
	fmt.Printf("flow: %d, cost: %d\n", res[0], -res[1])
	for _, e := range mcf.Edges() {
		if e.flow == 0 || e.from == 0 || e.to == n-1 {
			continue
		}
		fmt.Printf("from: %d, to: %d\n", e.from, e.to)
	}
}
