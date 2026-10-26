package structure

import (
	"errors"
	"math"

	"github.com/jackc/pgx/v5/pgtype"
)

const (
	positionStep = 1024.0
	// Below this gap a midpoint is no longer a trustworthy sort key, so the whole
	// sibling set is renumbered instead.
	minPositionGap = 1e-6
)

type sibling struct {
	ID       pgtype.UUID
	Position float64
}

// placement is the result of moving one sibling to a zero-based index among the
// other live siblings. Renumber, when set, lists every live sibling in its final
// order and replaces all positions with multiples of positionStep.
type placement struct {
	Unchanged bool
	Position  float64
	Renumber  []pgtype.UUID
}

// place is deterministic: siblings arrive ordered by (position, id), the target
// index counts the siblings excluding the moving item and must be in range.
func place(siblings []sibling, moving pgtype.UUID, index int) (placement, error) {
	current := -1
	others := make([]sibling, 0, len(siblings))
	for _, item := range siblings {
		if item.ID == moving {
			current = len(others)
			continue
		}
		others = append(others, item)
	}
	if current < 0 {
		return placement{}, errors.New("moving item is not among its live siblings")
	}
	if index < 0 || index > len(others) {
		return placement{}, validation("Index is outside the sibling range")
	}
	if index == current {
		return placement{Unchanged: true}, nil
	}
	switch {
	case len(others) == 0:
		return placement{Position: positionStep}, nil
	case index == 0:
		return placement{Position: others[0].Position - positionStep}, nil
	case index == len(others):
		return placement{Position: others[len(others)-1].Position + positionStep}, nil
	}
	before, after := others[index-1].Position, others[index].Position
	middle := before + (after-before)/2
	if after-before >= minPositionGap && before < middle && middle < after && !math.IsInf(middle, 0) {
		return placement{Position: middle}, nil
	}
	order := make([]pgtype.UUID, 0, len(siblings))
	for position, item := range others {
		if position == index {
			order = append(order, moving)
		}
		order = append(order, item.ID)
	}
	return placement{Renumber: order}, nil
}
