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
func (l DeletionCostPool) AddValues(costs []int) {
	for _, cost := range costs {
		l[cost] = struct{}{}
	}
}

// AddValue add value to pool
func (l DeletionCostPool) AddValue(cost int) {
	l[cost] = struct{}{}
}

// FindNextFree find new available slot
func (l DeletionCostPool) FindNextFree() (int, error) {
	if len(l) == 0 {
		l[math.MaxInt32] = struct{}{}
		return math.MaxInt32, nil
	}
	for i := math.MaxInt32; i >= 1; i-- {
		if _, has := l[i]; !has {
			l[i] = struct{}{}
			return i, nil
		}
	}
	return 0, fmt.Errorf("no deletion cost slot found")
}
