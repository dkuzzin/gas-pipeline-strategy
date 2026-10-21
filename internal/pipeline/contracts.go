package pipeline

import "fmt"

const (
	DigStepMeters   = 20
	MaxTrenchMeters = 100_000_000
	MinPipeMeters   = 1
	MaxPipeMeters   = 40
)

type Candidate struct {
	Start      int
	FreeLength int
}

// Размещение (layout) реализуется модулем layout.
//
// Метод FindCandidates должен возвращать ограниченный список возможных вариантов,
// а не все возможные целочисленные координаты.
// Если существует подходящий свободный интервал, метод обязан вернуть как минимум один вариант.
// Допускается возвращать варианты, упорядоченные в соответствии с эвристикой, специфичной для конкретного алгоритма размещения.
//
// Координаты задаются с использованием полуоткрытых интервалов: сегмент, начинающийся в точке Start и имеющий длину L,
// занимает диапазон [Start, Start+L). Сегменты могут соприкасаться, но не должны перекрываться.
type Layout interface {
	FindCandidates(length int) []Candidate
	Place(start, length int) error
	Remove(start, length int) error
	Extend(unitsOf20Meters int) error
	TrenchLength() int
	TailFreeLength() int
}

// Размещение (Placement) — это решение стратегии относительно следующего участка трубопровода.
//
// Если DigUnits == 0, то параметр Start должен совпадать с одним из предложенных вариантов начала участка.
// Если DigUnits > 0, контроллер выполняет прокладку траншеи, начиная с конца уже существующей траншеи;
// в этом случае параметр Start должен быть равен длине предыдущего участка траншеи.
// 
//Значение DigUnits указывается в 20-метровых единицах.
type Placement struct {
	Start    int
	DigUnits int
}

// Стратегия реализуется модулем стратегии.
// контроллер применяет выбранный вариант размещения после его проверки.
type Strategy interface {
	ChoosePlacement(length int, candidates []Candidate, trenchLength int, TailFreeLenght int) (Placement, error)
}

// ValidatePipeLength используется модулями, которые должны отклонять некорректные вызовы.
func ValidatePipeLength(length int) error {
	if length < MinPipeMeters || length > MaxPipeMeters {
		return fmt.Errorf("pipe length %d is outside [%d, %d]", length, MinPipeMeters, MaxPipeMeters)
	}
	return nil
}
