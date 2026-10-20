package pipeline

import "fmt"

type FitMode int

const (
	FirstFit FitMode = 0
	BestFit  FitMode = 1
	WorstFit FitMode = 2
)

type ClassicStrategy struct {
	Mode FitMode
}

func (s ClassicStrategy) ChoosePlacement(
	length int,
	candidates []Candidate,
	trenchLength int,
	tailFreeLength int,
) (Placement, error) {
	if err := ValidatePipeLength(length); err != nil {
		return Placement{}, err
	}

	switch s.Mode {
	case FirstFit, BestFit, WorstFit:
	default:
		return Placement{}, fmt.Errorf("unknown fit mode: %d", s.Mode)
	}

	if trenchLength < 0 || trenchLength > MaxTrenchMeters {
		return Placement{}, fmt.Errorf("invalid trench length")
	}

	if tailFreeLength < 0 || tailFreeLength > trenchLength {
		return Placement{}, fmt.Errorf("invalid tail length")
	}

	best, found := s.findCandidate(length, candidates)

	if found {
		if best.Start < 0 || best.Start > trenchLength-length {
			return Placement{}, fmt.Errorf("invalid candidate position")
		}

		return Placement{
			Start:    best.Start,
			DigUnits: 0,
		}, nil
	}

	// Места нет — расширяем траншею
	return placementWithDig(length, trenchLength, tailFreeLength)
}

func (s ClassicStrategy) findCandidate(length int, candidates []Candidate) (Candidate, bool) {

	var best Candidate
	found := false

	for _, candidate := range candidates {
		if candidate.FreeLength < length {
			continue
		}

		if !found {
			best = candidate
			found = true
			continue
		}

		switch s.Mode {
		case FirstFit:
			if candidate.Start < best.Start {
				best = candidate
			}

		case BestFit:
			if candidate.FreeLength < best.FreeLength ||
				(candidate.FreeLength == best.FreeLength &&
					candidate.Start < best.Start) {
				best = candidate
			}

		case WorstFit:
			if candidate.FreeLength > best.FreeLength ||
				(candidate.FreeLength == best.FreeLength &&
					candidate.Start < best.Start) {
				best = candidate
			}
		}
	}

	return best, found
}

func placementWithDig(length int, trenchLength int, tailFreeLength int) (Placement, error) {
	if err := ValidatePipeLength(length); err != nil {
		return Placement{}, err
	}

	if trenchLength < 0 || trenchLength > MaxTrenchMeters {
		return Placement{}, fmt.Errorf("invalid trench length")
	}

	if tailFreeLength < 0 || tailFreeLength > trenchLength {
		return Placement{}, fmt.Errorf("invalid tail length")
	}

	if tailFreeLength >= length {
		return Placement{}, fmt.Errorf("pipe fits in existing tail")
	}

	start := trenchLength - tailFreeLength
	missing := length - tailFreeLength

	digUnits := (missing + DigStepMeters - 1) / DigStepMeters

	newLength := trenchLength + digUnits*DigStepMeters

	if newLength > MaxTrenchMeters {
		return Placement{}, fmt.Errorf("trench length limit exceeded")
	}

	return Placement{
		Start:    start,
		DigUnits: digUnits,
	}, nil
}
