# Gas Pipeline — controller module (Go)

This folder contains the shared Go contracts, the interactive controller, unit tests, and a command-sequence generator. The concrete `Layout` and production `Strategy` implementations belong to the other developers and can be plugged in without changing `controller.Run` as long as they satisfy `internal/pipeline/contracts.go`.

## Project layout

```text
gas-pipeline/
├── go.mod
├── internal/
│   ├── pipeline/contracts.go       # shared contract: Layout, Strategy, Candidate, Placement
│   └── controller/
│       ├── controller.go           # protocol, PID map, validation, output + flush
│       └── controller_test.go      # fake Layout/Strategy; tests PID and protocol
├── cmd/gas-pipeline/main.go.example # wiring template; copy after modules are merged
└── cmd/testgen/main.go             # generates interactor-compatible stress tests
```

## Contracts to agree with the other developers

`Layout` must implement exactly these methods:

```go
FindCandidates(length int) []pipeline.Candidate
Place(start, length int) error
Remove(start, length int) error
Extend(unitsOf20Meters int) error
TrenchLength() int
```

`Strategy` must implement:

```go
ChoosePlacement(length int, candidates []pipeline.Candidate, trenchLength int) (pipeline.Placement, error)
```

A `Candidate` is the **start of a currently free interval**, plus that interval's total free length. The layout should return a bounded shortlist, not enumerate every integer coordinate or scan every free interval per query. If a fitting interval exists, return at least one candidate. The strategy must choose a candidate's `Start` when `DigUnits == 0`. If it requests digging (`DigUnits > 0`), it must place the pipe at the old trench end (`Start == trenchLength`). The controller then extends the trench by exactly `DigUnits * 20` and applies the placement.

All modules must use half-open pipe intervals `[start, start+length)`: adjacent pipes are allowed; overlapping pipes are not. `FindCandidates` and `Place` must agree on the same coordinate system.

## Controller behavior

- Every positive command gets the next PID: 1, 2, 3, ...; a PID is never reused after removal.
- The controller stores active `PID -> {start, length}` records.
- For `-PID`, it removes that exact interval from `Layout`, prints the remembered start, flushes, and deletes the active PID.
- For a positive length, it asks the strategy for a plan, validates it, updates `Layout`, records the PID, prints `N D`, and flushes.
- Command `0` returns immediately without printing anything.
- Protocol and module-contract errors are returned to the caller; the executable should log them to **stderr**, never stdout.

## Tests

```bash
go test ./...
go run ./cmd/testgen -mode boundary -n 1000 -seed 7 > /tmp/boundary.in
```

Generator modes:

- `dense`: many pipe placements, with varied lengths and no removals;
- `alternating`: repeated placements and removals to exercise released space;
- `boundary`: lengths around 1, 20, 21, 39, and 40, plus removals;
- `random`: seeded random lengths and valid removals.

The generated file follows the supplied interactor's test-file format: `weight n`, then exactly `n` scripted commands. It does **not** include command `0`: the interactor sends that terminal command itself.

Example using the supplied runner once the team has connected the real modules and built the executable:

```bash
go build -o ./solution ./cmd/gas-pipeline
python3 runner.py -i 1.in ./solution
python3 runner.py -d tests ./solution
```

## Wiring the real modules

After the `Layout` and `Strategy` packages are merged, `cmd/gas-pipeline/main.go` should construct their concrete implementations and call the controller:

```go
in := bufio.NewReader(os.Stdin)
out := bufio.NewWriter(os.Stdout)
err := controller.Run(in, out, layout.New(), strategy.New())
if err != nil {
    fmt.Fprintln(os.Stderr, err)
    os.Exit(1)
}
```

Replace `layout.New()` and `strategy.New()` with the actual constructor names agreed by the team. The controller package itself does not depend on either concrete package, so the concrete modules can be changed independently.
