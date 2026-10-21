package pipeline

// Temporary KPI test harness for TechArena Gas Pipeline Construction.
// Put this file in the same directory as contracts.go and pipe_strategy.go.
// This is test-only code: go test compiles it, go build does not.
// Run: go test -run '^TestOpenKPI$' -v -count=1 -timeout=3m

import (
	"bufio"
	"container/heap"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"testing"
)

// A test-only gap. Its old heap entries can remain after deletion; queries
// discard them lazily, so operations stay fast on the 200,000-command test.
type kpiGap struct {
	start, length int
	active        bool
}

type kpiHeap struct {
	data []*kpiGap
	kind int // 0: start asc, 1: length asc, 2: length desc
}

func (h *kpiHeap) Len() int { return len(h.data) }
func (h *kpiHeap) Less(i, j int) bool {
	a, b := h.data[i], h.data[j]
	if h.kind == 0 {
		return a.start < b.start
	}
	if a.length != b.length {
		if h.kind == 1 {
			return a.length < b.length
		}
		return a.length > b.length
	}
	return a.start < b.start
}
func (h *kpiHeap) Swap(i, j int) { h.data[i], h.data[j] = h.data[j], h.data[i] }
func (h *kpiHeap) Push(x any)    { h.data = append(h.data, x.(*kpiGap)) }
func (h *kpiHeap) Pop() any {
	n := len(h.data)
	x := h.data[n-1]
	h.data[n-1] = nil
	h.data = h.data[:n-1]
	return x
}
func (h *kpiHeap) first() *kpiGap {
	for len(h.data) > 0 {
		if h.data[0].active {
			return h.data[0]
		}
		heap.Pop(h)
	}
	return nil
}

type kpiLayout struct {
	length           int
	byStart          map[int]*kpiGap
	byEnd            map[int]*kpiGap
	bySize           [MaxPipeMeters + 1]*kpiHeap // size 40 groups all gaps >= 40
	longMin, longMax *kpiHeap
}

func newKPILayout() *kpiLayout {
	m := &kpiLayout{
		byStart: make(map[int]*kpiGap),
		byEnd:   make(map[int]*kpiGap),
		longMin: &kpiHeap{kind: 1},
		longMax: &kpiHeap{kind: 2},
	}
	for i := 1; i <= MaxPipeMeters; i++ {
		m.bySize[i] = &kpiHeap{kind: 0}
	}
	return m
}

func (m *kpiLayout) addGap(start, length int) {
	if length <= 0 {
		return
	}
	g := &kpiGap{start: start, length: length, active: true}
	m.byStart[start] = g
	m.byEnd[start+length] = g
	bucket := length
	if bucket > MaxPipeMeters {
		bucket = MaxPipeMeters
	}
	heap.Push(m.bySize[bucket], g)
	if length >= MaxPipeMeters {
		heap.Push(m.longMin, g)
		heap.Push(m.longMax, g)
	}
}

func (m *kpiLayout) dropGap(g *kpiGap) {
	if g == nil {
		return
	}
	g.active = false
	delete(m.byStart, g.start)
	delete(m.byEnd, g.start+g.length)
}

func (m *kpiLayout) tailFreeLength() int {
	if g := m.byEnd[m.length]; g != nil {
		return g.length
	}
	return 0
}

func (m *kpiLayout) extend(n int) error {
	if n < 0 || n > (MaxTrenchMeters-m.length)/DigStepMeters {
		return fmt.Errorf("invalid extension: %d units", n)
	}
	if n == 0 {
		return nil
	}
	start := m.length
	extra := n * DigStepMeters
	if g := m.byEnd[start]; g != nil {
		start = g.start
		extra += g.length
		m.dropGap(g)
	}
	m.length += n * DigStepMeters
	m.addGap(start, extra)
	return nil
}

// Our policy uses only the left edge of the chosen free gap.
func (m *kpiLayout) place(start, length int) error {
	g := m.byStart[start]
	if g == nil || length <= 0 || g.length < length {
		return fmt.Errorf("cannot place length %d at %d", length, start)
	}
	m.dropGap(g)
	m.addGap(start+length, g.length-length)
	return nil
}

func (m *kpiLayout) remove(start, length int) {
	end := start + length
	if g := m.byEnd[start]; g != nil {
		start = g.start
		m.dropGap(g)
	}
	if g := m.byStart[end]; g != nil {
		end += g.length
		m.dropGap(g)
	}
	m.addGap(start, end-start)
}

func (m *kpiLayout) candidates(length int) []Candidate {
	var first, best, worst *kpiGap
	// Find the true leftmost eligible gap among length buckets.
	for size := length; size <= MaxPipeMeters; size++ {
		if g := m.bySize[size].first(); g != nil &&
			(first == nil || g.start < first.start) {
			first = g
		}
	}
	// Best fit: smallest eligible length, then smallest coordinate.
	for size := length; size < MaxPipeMeters; size++ {
		if g := m.bySize[size].first(); g != nil {
			best = g
			break
		}
	}
	if best == nil {
		best = m.longMin.first()
	}
	// Worst fit: largest eligible length, then smallest coordinate.
	worst = m.longMax.first()
	if worst == nil {
		for size := MaxPipeMeters - 1; size >= length; size-- {
			if g := m.bySize[size].first(); g != nil {
				worst = g
				break
			}
		}
	}
	// Provide at most three distinct candidates. This is intentional:
	// first/best/worst fit are exact under this candidate-selection rule.
	result := make([]Candidate, 0, 3)
	seen := [3]int{-1, -1, -1}
	for _, g := range []*kpiGap{first, best, worst} {
		if g == nil {
			continue
		}
		duplicate := false
		for _, start := range seen {
			if start == g.start {
				duplicate = true
				break
			}
		}
		if duplicate {
			continue
		}
		seen[len(result)] = g.start
		result = append(result, Candidate{Start: g.start, FreeLength: g.length})
	}
	return result
}

type kpiPipe struct {
	start, length int
	live          bool
}

type kpiScenario struct {
	weight   float64
	commands []int
}

func readKPIScenario(path string) (kpiScenario, error) {
	f, err := os.Open(path)
	if err != nil {
		return kpiScenario{}, err
	}
	defer f.Close()
	in := bufio.NewReaderSize(f, 1<<16)
	var weight float64
	var count int
	if _, err := fmt.Fscan(in, &weight, &count); err != nil {
		return kpiScenario{}, fmt.Errorf("header: %w", err)
	}
	if count < 0 || count > 200000 {
		return kpiScenario{}, fmt.Errorf("invalid command count %d", count)
	}
	cmds := make([]int, count)
	for i := range cmds {
		if _, err := fmt.Fscan(in, &cmds[i]); err != nil {
			return kpiScenario{}, fmt.Errorf("command %d: %w", i+1, err)
		}
	}
	return kpiScenario{weight: weight, commands: cmds}, nil
}

func runKPIScenario(s kpiScenario, mode FitMode) (float64, int, error) {
	layout := newKPILayout()
	policy := ClassicStrategy{Mode: mode}
	pipes := []kpiPipe{{}} // PID numbering starts at 1
	occupied := 0
	for i, instruction := range s.commands {
		switch {
		case instruction > 0:
			options := layout.candidates(instruction)
			placement, err := policy.ChoosePlacement(
				instruction, options, layout.length, layout.tailFreeLength(),
			)
			if err != nil {
				return 0, layout.length, fmt.Errorf("command %d: strategy: %w", i+1, err)
			}
			if err := layout.extend(placement.DigUnits); err != nil {
				return 0, layout.length, fmt.Errorf("command %d: %w", i+1, err)
			}
			if err := layout.place(placement.Start, instruction); err != nil {
				return 0, layout.length, fmt.Errorf("command %d: %w", i+1, err)
			}
			pipes = append(pipes, kpiPipe{start: placement.Start, length: instruction, live: true})
			occupied += instruction
		case instruction < 0:
			pid := -instruction
			if pid <= 0 || pid >= len(pipes) || !pipes[pid].live {
				return 0, layout.length, fmt.Errorf("command %d: invalid PID %d", i+1, pid)
			}
			p := pipes[pid]
			layout.remove(p.start, p.length)
			pipes[pid].live = false
			occupied -= p.length
		default:
			return 0, layout.length, fmt.Errorf("unexpected command 0 at %d", i+1)
		}
	}
	if layout.length == 0 {
		return 0, 0, fmt.Errorf("empty final trench")
	}
	return float64(occupied) / float64(layout.length), layout.length, nil
}

// Run from your project root. By default, input is in testdata/kpi_open/*.in.
// To use an existing tests/ folder instead, set TECHAREA_TESTS=tests.
func TestOpenKPI(t *testing.T) {
	dir := os.Getenv("TECHAREA_TESTS")
	if dir == "" {
		dir = filepath.Join("testdata", "kpi_open")
	}
	paths, err := filepath.Glob(filepath.Join(dir, "*.in"))
	if err != nil {
		t.Fatal(err)
	}
	if len(paths) == 0 {
		t.Fatalf("no *.in files in %q; extract 2.zip tests there", dir)
	}
	if len(paths) != 15 {
		t.Fatalf("expected 15 open tests, found %d in %s", len(paths), dir)
	}
	sort.Slice(paths, func(i, j int) bool {
		var a, b int
		fmt.Sscanf(filepath.Base(paths[i]), "%d.in", &a)
		fmt.Sscanf(filepath.Base(paths[j]), "%d.in", &b)
		return a < b
	})
	modes := []struct {
		name  string
		value FitMode
	}{
		{"First Fit", FirstFit}, {"Best Fit", BestFit}, {"Worst Fit", WorstFit},
	}
	var totals [3]float64
	fmt.Println("\n OPEN TEST KPI (KPI in %, score = KPI * weight)")
	fmt.Printf("%-8s %9s %9s %9s %8s\n", "Test", "First", "Best", "Worst", "Weight")
	for _, path := range paths {
		scenario, err := readKPIScenario(path)
		if err != nil {
			t.Fatalf("read %s: %v", path, err)
		}
		var kpis [3]float64
		for i, mode := range modes {
			var trench int
			kpis[i], trench, err = runKPIScenario(scenario, mode.value)
			if err != nil {
				t.Fatalf("%s %s: %v", filepath.Base(path), mode.name, err)
			}
			_ = trench
			totals[i] += kpis[i] * scenario.weight
		}
		fmt.Printf("%-8s %8.2f%% %8.2f%% %8.2f%% %8.1f\n", filepath.Base(path), kpis[0]*100, kpis[1]*100, kpis[2]*100, scenario.weight)
	}
	fmt.Printf("WEIGHTED SCORE:  First = %.6f | Best = %.6f | Worst = %.6f\n\n", totals[0], totals[1], totals[2])
}
