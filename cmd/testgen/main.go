// testgen генерирует файлы в формате, который ожидает используемый интерактор:
// первая строка содержит «вес» (weight) и «количество команд» (number_of_commands), за которыми следуют сами команды.
// Последняя команда протокола — 0 — опущена; интерактор отправляет её самостоятельно.
package main

import (
	"flag"
	"fmt"
	"math/rand"
	"os"
)

func main() {
	mode := flag.String("mode", "random", "scenario: dense, alternating, boundary, random")
	n := flag.Int("n", 1000, "number of scripted commands (1..199999)")
	seed := flag.Int64("seed", 1, "random seed")
	weight := flag.Float64("weight", 1.0, "test weight written to the first line")
	flag.Parse()

	if *n < 1 || *n > 199_999 {
		fatalf("-n must be in [1, 199999], got %d", *n)
	}
	if *weight <= 0 {
		fatalf("-weight must be positive")
	}

	rng := rand.New(rand.NewSource(*seed))
	commands, err := generate(*mode, *n, rng)
	if err != nil {
		fatalf("%v", err)
	}

	writer := os.Stdout
	if _, err := fmt.Fprintf(writer, "%.10g %d\n", *weight, len(commands)); err != nil {
		fatalf("write header: %v", err)
	}
	for _, command := range commands {
		if _, err := fmt.Fprintln(writer, command); err != nil {
			fatalf("write command: %v", err)
		}
	}
}

func generate(mode string, n int, rng *rand.Rand) ([]int, error) {
	commands := make([]int, 0, n)
	activePIDs := make([]int, 0, n)
	nextPID := 1
	boundaryLengths := []int{40, 1, 39, 2, 20, 21, 19, 40, 1, 20, 40, 2, 38, 3}

	for i := 0; i < n; i++ {
		remove := false
		switch mode {

		case "dense":

		case "alternating":
			remove = len(activePIDs) > 0 && i%4 == 3
		case "boundary":
			remove = len(activePIDs) > 0 && i%5 == 4
		case "random":
			remove = len(activePIDs) > 0 && rng.Intn(100) < 35
		default:
			return nil, fmt.Errorf("unknown mode %q; use dense, alternating, boundary or random", mode)
		}

		if remove {
			index := len(activePIDs) - 1
			if mode == "random" {
				index = rng.Intn(len(activePIDs))
			}
			pid := activePIDs[index]
			commands = append(commands, -pid)
			activePIDs[index] = activePIDs[len(activePIDs)-1]
			activePIDs = activePIDs[:len(activePIDs)-1]
			continue
		}

		length := 1
		switch mode {
		case "dense":

			length = (i % 40) + 1
		case "alternating":

			length = []int{20, 1, 19, 2, 39, 40, 3, 21}[i%8]
		case "boundary":

			length = boundaryLengths[i%len(boundaryLengths)]
		case "random":
			
			length = rng.Intn(40) + 1
		}
		commands = append(commands, length)
		activePIDs = append(activePIDs, nextPID)
		nextPID++
	}

	if len(commands) == 0 || commands[0] <= 0 {
		return nil, fmt.Errorf("generated test must start with a pipe placement")
	}
	return commands, nil
}

func fatalf(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "testgen: "+format+"\n", args...)
	os.Exit(2)
}
