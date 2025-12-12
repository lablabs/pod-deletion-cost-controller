package zone_test

import (
	"testing"

	"github.com/lablabs/pod-deletion-cost-controller/internal/zone"
	v2 "k8s.io/api/apps/v1"
	v1 "k8s.io/api/core/v1"
	controllerruntime "sigs.k8s.io/controller-runtime"
)

func TestGetSpreadByAnnotation(t *testing.T) {
	tests := []struct {
		name       string
		nodeLabels map[string]string
		depAnn     map[string]string
		want       string
	}{
		{
			name:       "no deployment annotations → use default zone label",
			nodeLabels: map[string]string{zone.TopologyZoneAnnotation: "zone-a"},
			depAnn:     nil,
			want:       "zone-a",
		},
		{
			name: "deployment has spread-by → use custom label",
			nodeLabels: map[string]string{
				zone.TopologyZoneAnnotation: "zone-a",
				"custom-key":                "custom-value",
			},
			depAnn: map[string]string{
				zone.SpreadByAnnotation: "custom-key",
			},
			want: "custom-value",
		},
		{
			name:       "deployment has spread-by but node missing that label → empty",
			nodeLabels: map[string]string{zone.TopologyZoneAnnotation: "zone-a"},
			depAnn: map[string]string{
				zone.SpreadByAnnotation: "not-existing",
			},
			want: "",
		},
		{
			name:       "deployment annotations present but without spread-by → use default zone",
			nodeLabels: map[string]string{zone.TopologyZoneAnnotation: "zone-a"},
			depAnn:     map[string]string{"foo": "bar"},
			want:       "zone-a",
		},
		{
			name:       "node missing default zone label → empty string",
			nodeLabels: map[string]string{},
			depAnn:     nil,
			want:       "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {

			node := &v1.Node{
				ObjectMeta: controllerruntime.ObjectMeta{
					Labels: tt.nodeLabels,
				},
			}

			dep := &v2.Deployment{
				ObjectMeta: controllerruntime.ObjectMeta{
					Annotations: tt.depAnn,
				},
			}

			got := zone.GetSpreadByAnnotation(node, dep)
			if got != tt.want {
				t.Fatalf("expected %q, got %q", tt.want, got)
			}
		})
	}
}
