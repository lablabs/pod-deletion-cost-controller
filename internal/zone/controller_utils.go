package zone

import (
	"fmt"
	"math"
	"strconv"

	"github.com/lablabs/pod-deletion-cost-controller/internal/cost"
	corev1 "k8s.io/api/core/v1"
)

type DeletionCostList map[int]struct{}

func NewDeletionCostList(pods []corev1.Pod) DeletionCostList {
	out := make(DeletionCostList)
	for _, pod := range pods {
		v, has := pod.Annotations[cost.PodDeletionCostAnnotation]
		if has {
			dc, err := strconv.Atoi(v)
			if err != nil {
				continue
			}
			out[dc] = struct{}{}
		}
	}
	return out
}

func (l DeletionCostList) FindNext() (int, error) {
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

func ApplyPodDeletionCost(pod *corev1.Pod, c int) {
	if pod.Annotations == nil {
		pod.Annotations = make(map[string]string)
	}
	pod.Annotations[cost.PodDeletionCostAnnotation] = strconv.Itoa(c)
}
