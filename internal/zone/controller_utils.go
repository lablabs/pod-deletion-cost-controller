package zone

import (
	"fmt"
	"math"
)

type DeletionCostPool map[int]struct{}

func NewDeletionCostPool() DeletionCostPool {
	return make(DeletionCostPool)
}

func (l DeletionCostPool) AddValues(costs []int) {
	for _, cost := range costs {
		l[cost] = struct{}{}
	}
}

func (l DeletionCostPool) AddValue(cost int) {
	l[cost] = struct{}{}
}

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
