// Package controller implements the interactive protocol and owns PID state.
package controller

import (
	"bufio"
	"errors"
	"fmt"
	"io"

	"gas-pipeline/internal/pipeline"
)

// FlushWriter is implemented by *bufio.Writer. Making it an interface also
// allows tests to verify that every response is flushed.
type FlushWriter interface {
	io.Writer
	Flush() error
}

// Pipe stores the active pipe's interval. The PID is the key in the active map.
type Pipe struct {
	Start  int
	Length int
}

// Run reads commands until command 0. Positive commands are pipe lengths;
// negative commands remove PID=-command. Every answer is flushed immediately.
//
// EOF without a terminating 0 is treated as a protocol error: in the official
// interaction the interactor always sends 0 after the last scripted command.
func Run(in io.Reader, out FlushWriter, layout pipeline.Layout, strategy pipeline.Strategy) error {
	if in == nil || out == nil || layout == nil || strategy == nil {
		return errors.New("controller: input, output, layout and strategy must be non-nil")
	}

	reader := bufio.NewReader(in)
	active := make(map[int]Pipe)
	nextPID := 1

	for {
		var command int
		if _, err := fmt.Fscan(reader, &command); err != nil {
			if errors.Is(err, io.EOF) {
				return errors.New("controller: input ended before the terminating command 0")
			}
			return fmt.Errorf("controller: read command: %w", err)
		}

		if command == 0 {
			// The protocol explicitly requires no response to 0.
			return nil
		}

		if command > 0 {
			if err := handleLay(command, out, layout, strategy, active, nextPID); err != nil {
				return fmt.Errorf("controller: lay PID %d (length %d): %w", nextPID, command, err)
			}
			nextPID++
			continue
		}

		pid := -command
		pipe, ok := active[pid]
		if !ok {
			return fmt.Errorf("controller: cannot remove PID %d: pipe is unknown or already removed", pid)
		}
		if err := layout.Remove(pipe.Start, pipe.Length); err != nil {
			return fmt.Errorf("controller: Layout.Remove for PID %d at %d length %d: %w", pid, pipe.Start, pipe.Length, err)
		}
		if _, err := fmt.Fprintln(out, pipe.Start); err != nil {
			return fmt.Errorf("controller: write removal response for PID %d: %w", pid, err)
		}
		if err := out.Flush(); err != nil {
			return fmt.Errorf("controller: flush removal response for PID %d: %w", pid, err)
		}
		delete(active, pid)
	}
}

func handleLay(
	length int,
	out FlushWriter,
	layout pipeline.Layout,
	strategy pipeline.Strategy,
	active map[int]Pipe,
	pid int,
) error {
	if err := pipeline.ValidatePipeLength(length); err != nil {
		return err
	}

	oldTrenchLength := layout.TrenchLength()
	if oldTrenchLength < 0 || oldTrenchLength > pipeline.MaxTrenchMeters {
		return fmt.Errorf("invalid current trench length %d", oldTrenchLength)
	}

	tailFreeLength := layout.TailFreeLength()
	if tailFreeLength < 0 || tailFreeLength > oldTrenchLength {
		return fmt.Errorf("invalid tail free length %d for trench length %d", tailFreeLength, oldTrenchLength)
	}

	candidates := layout.FindCandidates(length)
	plan, err := strategy.ChoosePlacement(length, candidates, oldTrenchLength, tailFreeLength)
	if err != nil {
		return fmt.Errorf("strategy: %w", err)
	}
	if err := validatePlan(length, oldTrenchLength, tailFreeLength, candidates, plan); err != nil {
		return fmt.Errorf("invalid strategy plan: %w", err)
	}

	if plan.DigUnits > 0 {
		if err := layout.Extend(plan.DigUnits); err != nil {
			return fmt.Errorf("Layout.Extend(%d): %w", plan.DigUnits, err)
		}
		expectedLength := oldTrenchLength + plan.DigUnits*pipeline.DigStepMeters
		if got := layout.TrenchLength(); got != expectedLength {
			return fmt.Errorf("Layout.Extend contract violation: trench length is %d, expected %d", got, expectedLength)
		}
	}

	if err := layout.Place(plan.Start, length); err != nil {
		return fmt.Errorf("Layout.Place(start=%d, length=%d): %w", plan.Start, length, err)
	}

	// A PID is assigned for every positive input command and is never reused,
	// even after that pipe is removed. This matches the interactor's convention.
	active[pid] = Pipe{Start: plan.Start, Length: length}

	if _, err := fmt.Fprintf(out, "%d %d\n", plan.DigUnits, plan.Start); err != nil {
		return fmt.Errorf("write laying response: %w", err)
	}
	if err := out.Flush(); err != nil {
		return fmt.Errorf("flush laying response: %w", err)
	}
	return nil
}

func validatePlan(length, trenchLength, tailFreeLength int, candidates []pipeline.Candidate, plan pipeline.Placement) error {
	if tailFreeLength < 0 || tailFreeLength > trenchLength {
		return fmt.Errorf("invalid tail free length %d for trench length %d", tailFreeLength, trenchLength)
	}
	// Validate every candidate regardless of which branch the strategy chooses.
	for _, candidate := range candidates {
		if candidate.Start < 0 || candidate.Start > trenchLength ||
			candidate.FreeLength < length || candidate.FreeLength > trenchLength-candidate.Start {
			return fmt.Errorf("Layout returned invalid candidate %+v for length %d", candidate, length)
		}
	}

	if plan.Start < 0 {
		return fmt.Errorf("negative start coordinate %d", plan.Start)
	}
	if plan.DigUnits < 0 {
		return fmt.Errorf("negative digging amount %d", plan.DigUnits)
	}

	if plan.DigUnits == 0 {
		found := false
		for _, candidate := range candidates {
			if candidate.Start == plan.Start {
				found = true
			}
		}
		if !found {
			return fmt.Errorf("start %d is not among Layout candidates and no trench extension was requested", plan.Start)
		}
		if trenchLength < length || plan.Start > trenchLength-length {
			return fmt.Errorf("pipe end exceeds current trench length %d", trenchLength)
		}
		return nil
	}

	// Extension adds free space to the current free tail. The whole pipe must
	// start inside that tail (or its extension), not in an earlier gap separated
	// from the end by another pipe.
	tailStart := trenchLength - tailFreeLength
	if plan.Start < tailStart {
		return fmt.Errorf("when digging, start %d is before free tail start %d", plan.Start, tailStart)
	}
	maxUnits := (pipeline.MaxTrenchMeters - trenchLength) / pipeline.DigStepMeters
	if plan.DigUnits > maxUnits {
		return fmt.Errorf("digging %d units would exceed maximum trench length %d", plan.DigUnits, pipeline.MaxTrenchMeters)
	}
	newLength := trenchLength + plan.DigUnits*pipeline.DigStepMeters
	if plan.Start > newLength-length {
		return fmt.Errorf("pipe end %d exceeds trench length after digging %d", plan.Start+length, newLength)
	}
	return nil
}
