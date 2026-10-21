package pipeline

import "testing"

func TestClassicStrategy(t *testing.T) {
	candidates := []Candidate{
		{Start: 50, FreeLength: 20},
		{Start: 10, FreeLength: 15},
		{Start: 80, FreeLength: 12},
	}

	tests := []struct {
		name string
		mode FitMode
		want Placement
	}{
		{
			name: "First Fit",
			mode: FirstFit,
			want: Placement{Start: 10, DigUnits: 0},
		},
		{
			name: "Best Fit",
			mode: BestFit,
			want: Placement{Start: 80, DigUnits: 0},
		},
		{
			name: "Worst Fit",
			mode: WorstFit,
			want: Placement{Start: 50, DigUnits: 0},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			strategy := ClassicStrategy{Mode: tt.mode}

			got, err := strategy.ChoosePlacement(
				10,
				candidates,
				100,
				0,
			)

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if got != tt.want {
				t.Errorf(
					"got %+v, want %+v",
					got,
					tt.want,
				)
			}
		})
	}
}

func TestPlacementWithDig(t *testing.T) {
	tests := []struct {
		name           string
		length         int
		trenchLength   int
		tailFreeLength int
		want           Placement
	}{
		{
			name:           "Empty trench",
			length:         12,
			trenchLength:   0,
			tailFreeLength: 0,
			want:           Placement{Start: 0, DigUnits: 1},
		},
		{
			name:           "Reuse free tail",
			length:         30,
			trenchLength:   60,
			tailFreeLength: 12,
			want:           Placement{Start: 48, DigUnits: 1},
		},
		{
			name:           "Dig two units",
			length:         30,
			trenchLength:   100,
			tailFreeLength: 8,
			want:           Placement{Start: 92, DigUnits: 2},
		},
		{
			name:           "No free tail",
			length:         40,
			trenchLength:   60,
			tailFreeLength: 0,
			want:           Placement{Start: 60, DigUnits: 2},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			strategy := ClassicStrategy{Mode: BestFit}

			got, err := strategy.ChoosePlacement(
				tt.length,
				nil,
				tt.trenchLength,
				tt.tailFreeLength,
			)

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if got != tt.want {
				t.Errorf("got %+v, want %+v", got, tt.want)
			}
		})
	}
}

func TestChoosePlacementErrors(t *testing.T) {
	tests := []struct {
		name       string
		mode       FitMode
		length     int
		candidates []Candidate
		trench     int
		tail       int
	}{
		{
			name:   "Pipe too short",
			mode:   BestFit,
			length: 0,
			trench: 20,
		},
		{
			name:   "Pipe too long",
			mode:   BestFit,
			length: 41,
			trench: 20,
		},
		{
			name:   "Unknown strategy",
			mode:   FitMode(100),
			length: 10,
			trench: 20,
		},
		{
			name:   "Negative trench",
			mode:   BestFit,
			length: 10,
			trench: -20,
		},
		{
			name:   "Trench exceeds limit",
			mode:   BestFit,
			length: 10,
			trench: MaxTrenchMeters + 20,
		},
		{
			name:   "Negative tail",
			mode:   BestFit,
			length: 10,
			trench: 20,
			tail:   -1,
		},
		{
			name:   "Tail exceeds trench",
			mode:   BestFit,
			length: 10,
			trench: 20,
			tail:   25,
		},
		{
			name:   "Negative candidate position",
			mode:   BestFit,
			length: 10,
			trench: 40,
			candidates: []Candidate{
				{Start: -5, FreeLength: 15},
			},
		},
		{
			name:   "Pipe exceeds trench boundary",
			mode:   BestFit,
			length: 10,
			trench: 40,
			candidates: []Candidate{
				{Start: 35, FreeLength: 15},
			},
		},
		{
			name:   "Dig exceeds maximum trench",
			mode:   BestFit,
			length: 21,
			trench: MaxTrenchMeters - 20,
		},
		{
			name:   "Missing tail candidate",
			mode:   BestFit,
			length: 10,
			trench: 40,
			tail:   15,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			strategy := ClassicStrategy{Mode: tt.mode}

			_, err := strategy.ChoosePlacement(
				tt.length,
				tt.candidates,
				tt.trench,
				tt.tail,
			)

			if err == nil {
				t.Error("expected error, got nil")
			}
		})
	}
}

func TestTieBreaking(t *testing.T) {
	candidates := []Candidate{
		{Start: 50, FreeLength: 20},
		{Start: 10, FreeLength: 20},
		{Start: 80, FreeLength: 20},
	}

	modes := []FitMode{
		FirstFit,
		BestFit,
		WorstFit,
	}

	for _, mode := range modes {
		strategy := ClassicStrategy{Mode: mode}

		got, err := strategy.ChoosePlacement(
			10, candidates, 100, 0,
		)

		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if got.Start != 10 || got.DigUnits != 0 {
			t.Errorf(
				"mode %d: got %+v, want Start=10 DigUnits=0",
				mode, got,
			)
		}
	}
}

func TestCandidateFiltering(t *testing.T) {
	candidates := []Candidate{
		{Start: 0, FreeLength: 5},
		{Start: 20, FreeLength: 8},
		{Start: 40, FreeLength: 10},
		{Start: 60, FreeLength: 20},
	}

	strategy := ClassicStrategy{Mode: BestFit}

	got, err := strategy.ChoosePlacement(
		10, candidates, 100, 0,
	)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	want := Placement{
		Start:    40,
		DigUnits: 0,
	}

	if got != want {
		t.Errorf("got %+v, want %+v", got, want)
	}
}

func TestDigBoundaries(t *testing.T) {
	tests := []struct {
		name   string
		length int
		trench int
		tail   int
		want   Placement
	}{
		{
			name:   "Minimum pipe",
			length: 1,
			trench: 0,
			tail:   0,
			want:   Placement{Start: 0, DigUnits: 1},
		},
		{
			name:   "Exactly 20 meters missing",
			length: 20,
			trench: 60,
			tail:   0,
			want:   Placement{Start: 60, DigUnits: 1},
		},
		{
			name:   "21 meters missing",
			length: 21,
			trench: 60,
			tail:   0,
			want:   Placement{Start: 60, DigUnits: 2},
		},
		{
			name:   "Maximum pipe length",
			length: 40,
			trench: 60,
			tail:   0,
			want:   Placement{Start: 60, DigUnits: 2},
		},
		{
			name:   "Reuse 20 meter tail",
			length: 40,
			trench: 60,
			tail:   20,
			want:   Placement{Start: 40, DigUnits: 1},
		},
		{
			name:   "Dig exactly to limit",
			length: 20,
			trench: MaxTrenchMeters - 20,
			tail:   0,
			want: Placement{
				Start:    MaxTrenchMeters - 20,
				DigUnits: 1,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			strategy := ClassicStrategy{Mode: BestFit}

			got, err := strategy.ChoosePlacement(
				tt.length, nil, tt.trench, tt.tail,
			)

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if got != tt.want {
				t.Errorf("got %+v, want %+v", got, tt.want)
			}
		})
	}
}

func TestPlacementWithDigErrors(t *testing.T) {
	tests := []struct {
		name   string
		length int
		trench int
		tail   int
	}{
		{
			name:   "Invalid pipe length",
			length: 0,
			trench: 20,
			tail:   0,
		},
		{
			name:   "Invalid trench length",
			length: 10,
			trench: -1,
			tail:   0,
		},
		{
			name:   "Invalid tail length",
			length: 10,
			trench: 20,
			tail:   -1,
		},
		{
			name:   "Pipe already fits in tail",
			length: 10,
			trench: 20,
			tail:   15,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := placementWithDig(
				tt.length,
				tt.trench,
				tt.tail,
			)

			if err == nil {
				t.Error("expected error, got nil")
			}
		})
	}
}
