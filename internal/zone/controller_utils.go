package zone

import (
	"fmt"
	"math"
)

// DeletionCostPool pool
type DeletionCostPool map[int]struct{}

// NewDeletionCostPool create new pool
func NewDeletionCostPool() DeletionCostPool {
	return make(DeletionCostPool)
}

// AddValues add arrays of value
func (p DeletionCostPool) AddValues(costs []int) {
	for _, cost := range costs {
		p[cost] = struct{}{}
	}
}

// AddValue add value to pool
func (p DeletionCostPool) AddValue(cost int) {
	p[cost] = struct{}{}
}

// FindNextFree find new available slot
func (p DeletionCostPool) FindNextFree() (int, error) {
	if len(p) == 0 {
		p[math.MaxInt32] = struct{}{}
		return math.MaxInt32, nil
	}
	for i := math.MaxInt32; i >= 1; i-- {
		if _, has := p[i]; !has {
			p[i] = struct{}{}
			return i, nil
		}
	}
	return 0, fmt.Errorf("no deletion cost slot found")
}
