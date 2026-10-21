package controller

import (
	"bytes"
	"errors"
	"fmt"
	"sort"
	"strings"
	"testing"

	"gas-pipeline/internal/pipeline"
)

type fakeLayout struct {
	length int
	pipes  map[int]Pipe // keyed by start coordinate
}

func newFakeLayout() *fakeLayout {
	return &fakeLayout{pipes: make(map[int]Pipe)}
}

func (l *fakeLayout) TrenchLength() int { return l.length }

func (l *fakeLayout) TailFreeLength() int {
	rightmostEnd := 0
	for _, p := range l.pipes {
		if end := p.Start + p.Length; end > rightmostEnd {
			rightmostEnd = end
		}
	}
	return l.length - rightmostEnd
}

func (l *fakeLayout) FindCandidates(length int) []pipeline.Candidate {
	starts := make([]int, 0, len(l.pipes))
	for start := range l.pipes {
		starts = append(starts, start)
	}
	sort.Ints(starts)

	var result []pipeline.Candidate
	cursor := 0
	for _, start := range starts {
		p := l.pipes[start]
		if start-cursor >= length {
			result = append(result, pipeline.Candidate{Start: cursor, FreeLength: start - cursor})
		}
		if end := p.Start + p.Length; end > cursor {
			cursor = end
		}
	}
	if l.length-cursor >= length {
		result = append(result, pipeline.Candidate{Start: cursor, FreeLength: l.length - cursor})
	}
	return result
}

func (l *fakeLayout) Place(start, length int) error {
	if start < 0 || start+length > l.length {
		return fmt.Errorf("pipe outside trench")
	}
	for _, p := range l.pipes {
		if start < p.Start+p.Length && p.Start < start+length {
			return fmt.Errorf("overlap")
		}
	}
	l.pipes[start] = Pipe{Start: start, Length: length}
	return nil
}

func (l *fakeLayout) Remove(start, length int) error {
	p, ok := l.pipes[start]
	if !ok || p.Length != length {
		return errors.New("pipe not found")
	}
	delete(l.pipes, start)
	return nil
}

func (l *fakeLayout) Extend(units int) error {
	if units < 1 || l.length+units*pipeline.DigStepMeters > pipeline.MaxTrenchMeters {
		return errors.New("invalid extension")
	}
	l.length += units * pipeline.DigStepMeters
	return nil
}

type bestFitStrategy struct{}

func (bestFitStrategy) ChoosePlacement(length int, candidates []pipeline.Candidate, trenchLength int, tailFreeLength int) (pipeline.Placement, error) {
	if len(candidates) == 0 {
		// Reuse the already-dug free tail before asking the machine to extend it.
		start := trenchLength - tailFreeLength
		remaining := length - tailFreeLength
		units := 0
		if remaining > 0 {
			units = (remaining + pipeline.DigStepMeters - 1) / pipeline.DigStepMeters
		}
		return pipeline.Placement{Start: start, DigUnits: units}, nil
	}
	best := candidates[0]
	for _, c := range candidates[1:] {
		if c.FreeLength-length < best.FreeLength-length ||
			(c.FreeLength == best.FreeLength && c.Start < best.Start) {
			best = c
		}
	}
	return pipeline.Placement{Start: best.Start, DigUnits: 0}, nil
}

type flushCountingWriter struct {
	bytes.Buffer
	flushes int
}

func (w *flushCountingWriter) Flush() error {
	w.flushes++
	return nil
}

func TestRunSampleSequence(t *testing.T) {
	input := "1\n4\n15\n-2\n3\n-1\n0\n"
	out := &flushCountingWriter{}
	layout := newFakeLayout()

	if err := Run(strings.NewReader(input), out, layout, bestFitStrategy{}); err != nil {
		t.Fatalf("Run returned error: %v", err)
	}
	want := "1 0\n0 1\n0 5\n1\n0 1\n0\n"
	if got := out.String(); got != want {
		t.Fatalf("output mismatch\n got: %q\nwant: %q", got, want)
	}
	if out.flushes != 6 {
		t.Fatalf("Flush called %d times; want 6 (once per response)", out.flushes)
	}
	if layout.length != 20 {
		t.Fatalf("trench length = %d; want 20", layout.length)
	}
	if len(layout.pipes) != 2 {
		t.Fatalf("active pipe count = %d; want 2", len(layout.pipes))
	}
}

func TestDiggingCanReuseExistingFreeTail(t *testing.T) {
	input := "10\n25\n0\n"
	out := &flushCountingWriter{}
	layout := newFakeLayout()

	if err := Run(strings.NewReader(input), out, layout, bestFitStrategy{}); err != nil {
		t.Fatalf("Run returned error: %v", err)
	}
	// First pipe extends the empty trench to 20m and starts at 0.
	// Then 10m of the old trench is a free tail. The 25m pipe starts at 10,
	// reuses that 10m, and needs only one additional 20m dig.
	want := "1 0\n1 10\n"
	if got := out.String(); got != want {
		t.Fatalf("output mismatch\n got: %q\nwant: %q", got, want)
	}
	if layout.length != 40 {
		t.Fatalf("trench length = %d; want 40", layout.length)
	}
	if len(layout.pipes) != 2 {
		t.Fatalf("active pipe count = %d; want 2", len(layout.pipes))
	}
}

func TestZeroProducesNoResponse(t *testing.T) {
	out := &flushCountingWriter{}
	if err := Run(strings.NewReader("0\n"), out, newFakeLayout(), bestFitStrategy{}); err != nil {
		t.Fatalf("Run returned error: %v", err)
	}
	if out.Len() != 0 || out.flushes != 0 {
		t.Fatalf("command 0 must produce no response; output=%q flushes=%d", out.String(), out.flushes)
	}
}

func TestRemovingUnknownPIDReturnsError(t *testing.T) {
	out := &flushCountingWriter{}
	err := Run(strings.NewReader("-1\n0\n"), out, newFakeLayout(), bestFitStrategy{})
	if err == nil {
		t.Fatal("expected error for unknown PID")
	}
	if out.Len() != 0 {
		t.Fatalf("unexpected output: %q", out.String())
	}
}

func TestEOFWithoutZeroReturnsError(t *testing.T) {
	out := &flushCountingWriter{}
	err := Run(strings.NewReader("1\n"), out, newFakeLayout(), bestFitStrategy{})
	if err == nil {
		t.Fatal("expected protocol error on EOF without 0")
	}
}

func TestRejectsTooMuchDigging(t *testing.T) {
	out := &flushCountingWriter{}
	strategy := strategyFunc(func(length int, candidates []pipeline.Candidate, trenchLength int, tailFreeLength int) (pipeline.Placement, error) {
		return pipeline.Placement{Start: trenchLength - tailFreeLength, DigUnits: pipeline.MaxTrenchMeters/pipeline.DigStepMeters + 1}, nil
	})
	err := Run(strings.NewReader("40\n0\n"), out, newFakeLayout(), strategy)
	if err == nil {
		t.Fatal("expected invalid plan error")
	}
	if out.Len() != 0 {
		t.Fatalf("invalid plan should not be printed, got %q", out.String())
	}
}

type strategyFunc func(length int, candidates []pipeline.Candidate, trenchLength int, tailFreeLength int) (pipeline.Placement, error)

func (f strategyFunc) ChoosePlacement(length int, candidates []pipeline.Candidate, trenchLength int, tailFreeLength int) (pipeline.Placement, error) {
	return f(length, candidates, trenchLength, tailFreeLength)
}
